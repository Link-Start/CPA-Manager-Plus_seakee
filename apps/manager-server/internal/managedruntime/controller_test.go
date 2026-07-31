package managedruntime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/gateway"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
)

type blockingRuntimeProcessSupervisor struct {
	started chan struct{}
	stopped chan struct{}
	once    sync.Once
}

func newBlockingRuntimeProcessSupervisor() *blockingRuntimeProcessSupervisor {
	return &blockingRuntimeProcessSupervisor{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (s *blockingRuntimeProcessSupervisor) Run(ctx context.Context, _ string, _ ProcessSpecProvider) {
	s.once.Do(func() { close(s.started) })
	<-ctx.Done()
	close(s.stopped)
}

func (*blockingRuntimeProcessSupervisor) Restart(string) error {
	return nil
}

func TestRuntimeControllerProcessSpecsPreserveConfiguredPathsAndVersions(t *testing.T) {
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	router, err := gateway.New(
		"http://127.0.0.1:18318",
		"http://127.0.0.1:8317",
		"http://127.0.0.1:18319",
		"/panel",
		nil,
	)
	if err != nil {
		t.Fatalf("create gateway router: %v", err)
	}
	customDBPath := filepath.Join(t.TempDir(), "custom", "manager.sqlite")
	controller := &RuntimeController{
		ctx: context.Background(),
		cfg: Config{
			DataDir:       t.TempDir(),
			UsageDBPath:   customDBPath,
			ManagerAddr:   "127.0.0.1:18318",
			ManagerBinary: "/runtime/cpa-manager-plus",
			CPABinary:     "/runtime/cli-proxy-api",
			CPAConfigPath: "/runtime/config.yaml",
			CPAVersion:    "v7.2.92",
			ControlURL:    "http://127.0.0.1:18319",
		},
		state:   state,
		router:  router,
		secrets: Secrets{RuntimeKey: "runtime-key", CPAManagementKey: "cpa-key"},
	}

	if got := controller.managerSpec().Env["USAGE_DB_PATH"]; got != customDBPath {
		t.Fatalf("manager USAGE_DB_PATH = %q, want %q", got, customDBPath)
	}
	if got := controller.managerSpec().Env["CPA_MANAGER_PANEL_BASE_PATH_SOURCE"]; got != model.PanelBasePathSourceRuntime {
		t.Fatalf("manager panel Base Path source = %q", got)
	}
	if got := controller.cpaSpec().Version; got != "v7.2.92" {
		t.Fatalf("initial CPA version = %q, want v7.2.92", got)
	}
}

func TestProvisionSlimFailureStopsCPAAndRestoresRuntimeSnapshot(t *testing.T) {
	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "manager:"+r.URL.Path)
	}))
	t.Cleanup(manager.Close)
	initialCPA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "initial:"+r.URL.Path)
	}))
	t.Cleanup(initialCPA.Close)
	unhealthyCPA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	t.Cleanup(unhealthyCPA.Close)
	runtimeTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "runtime:"+r.URL.Path)
	}))
	t.Cleanup(runtimeTarget.Close)

	dataDir := t.TempDir()
	state, err := OpenStateStore(
		filepath.Join(dataDir, "runtime", "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeSlim, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateCPAUpstream(initialCPA.URL); err != nil {
		t.Fatal(err)
	}
	router, err := gateway.New(manager.URL, initialCPA.URL, runtimeTarget.URL, "/panel", nil)
	if err != nil {
		t.Fatal(err)
	}
	supervisor := newBlockingRuntimeProcessSupervisor()
	oldSecrets := Secrets{RuntimeKey: "runtime-key", CPAManagementKey: "old-cpa-key"}
	cfg := Config{
		DataDir:              dataDir,
		DeploymentMode:       model.DeploymentModeSlim,
		CPAAddr:              unhealthyCPA.Listener.Addr().String(),
		CPAURL:               unhealthyCPA.URL,
		CPAConfigPath:        filepath.Join(dataDir, "cpa", "config.yaml"),
		CPAAuthDir:           filepath.Join(dataDir, "cpa", "auths"),
		CPAManagementKeyPath: filepath.Join(dataDir, "runtime", "cpa-management.key"),
		RuntimeKeyPath:       filepath.Join(dataDir, "runtime", "control.key"),
	}
	controller := &RuntimeController{
		ctx:        context.Background(),
		cfg:        cfg,
		state:      state,
		router:     router,
		secrets:    oldSecrets,
		supervisor: supervisor,
		installer: &fakeRuntimeReleaseInstaller{
			manifest: ReleaseManifest{Components: map[string]ReleaseComponent{"cpa": {Version: "v7.2.92"}}},
			results: map[string]InstalledComponent{
				"cpa": {Name: "cpa", Version: "v7.2.92", BinaryPath: filepath.Join(dataDir, "downloads", "cli-proxy-api")},
			},
			errors: map[string]error{},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := controller.ProvisionSlim(ctx); err == nil {
		t.Fatal("expected unhealthy CPA provisioning to fail")
	}
	select {
	case <-supervisor.started:
	default:
		t.Fatal("CPA supervisor did not start")
	}
	select {
	case <-supervisor.stopped:
	case <-time.After(time.Second):
		t.Fatal("CPA supervisor did not stop during rollback")
	}
	snapshot := state.Snapshot()
	if snapshot.Deployment.Mode != model.DeploymentModeSlim {
		t.Fatalf("deployment mode after rollback = %q", snapshot.Deployment.Mode)
	}
	if snapshot.CPAUpstreamURL != initialCPA.URL {
		t.Fatalf("CPA upstream after rollback = %q, want %q", snapshot.CPAUpstreamURL, initialCPA.URL)
	}
	if _, ok := snapshot.Components["cpa"]; ok {
		t.Fatalf("CPA component was not removed during rollback: %+v", snapshot.Components["cpa"])
	}
	controller.mu.Lock()
	gotCfg := controller.cfg
	gotSecrets := controller.secrets
	cpaStarted := controller.cpaStarted
	controller.mu.Unlock()
	pending := state.Snapshot().SlimTransition
	if gotCfg.DeploymentMode != cfg.DeploymentMode || gotCfg.CPABinary != cfg.CPABinary {
		t.Fatalf("runtime config after rollback = %+v, want deployment %q and binary %q", gotCfg, cfg.DeploymentMode, cfg.CPABinary)
	}
	if gotSecrets != oldSecrets {
		t.Fatalf("runtime secrets after rollback = %+v, want %+v", gotSecrets, oldSecrets)
	}
	if pending != nil || cpaStarted {
		t.Fatalf("rollback left pending transition=%v cpaStarted=%v", pending != nil, cpaStarted)
	}
	routed := httptest.NewRecorder()
	router.ServeHTTP(routed, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if routed.Code != http.StatusOK || routed.Body.String() != "initial:/v1/chat/completions" {
		t.Fatalf("routed response after rollback = %d %q", routed.Code, routed.Body.String())
	}
}

func TestSlimProvisionRetryAndRevertSurviveRuntimeRestart(t *testing.T) {
	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "manager:"+r.URL.Path)
	}))
	t.Cleanup(manager.Close)
	previousCPA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "previous:"+r.URL.Path)
	}))
	t.Cleanup(previousCPA.Close)
	provisionedCPA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "provisioned:"+r.URL.Path)
	}))
	t.Cleanup(provisionedCPA.Close)
	runtimeTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "runtime:"+r.URL.Path)
	}))
	t.Cleanup(runtimeTarget.Close)

	root := t.TempDir()
	statePath := filepath.Join(root, "runtime", "state.json")
	state, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeSlim, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateCPAUpstream(previousCPA.URL); err != nil {
		t.Fatal(err)
	}
	installed := InstalledComponent{
		Name:       "cpa",
		Version:    "v7.2.92",
		BinaryPath: filepath.Join(root, "components", "cpa", "v7.2.92", "cli-proxy-api"),
	}
	if _, err := state.BeginSlimTransition(
		slimTransitionProvision,
		provisionedCPA.URL,
		"",
		false,
		&installed,
	); err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpa", func(component ComponentState) ComponentState {
		component.Version = installed.Version
		component.BinaryPath = installed.BinaryPath
		component.Status = "installed"
		return component
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateCPAUpstream(provisionedCPA.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateDeployment(func(deployment model.DeploymentState) model.DeploymentState {
		deployment.Mode = model.DeploymentModeIntegrated
		deployment.RuntimeManaged = true
		deployment.CPAMPUpdatesManaged = true
		deployment.CPAUpdatesManaged = true
		return deployment
	}); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeSlim, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().SlimTransition == nil {
		t.Fatal("pending Slim transition was not persisted")
	}
	router, err := gateway.New(manager.URL, provisionedCPA.URL, runtimeTarget.URL, "/panel", nil)
	if err != nil {
		t.Fatal(err)
	}
	installer := &fakeRuntimeReleaseInstaller{
		manifest: ReleaseManifest{Components: map[string]ReleaseComponent{"cpa": {Version: "unexpected"}}},
		results:  map[string]InstalledComponent{},
		errors:   map[string]error{},
	}
	controller := &RuntimeController{
		ctx: context.Background(),
		cfg: Config{
			DataDir:              root,
			DeploymentMode:       model.DeploymentModeIntegrated,
			ManagerAddr:          "127.0.0.1:18318",
			ManagerBinary:        "/runtime/cpa-manager-plus",
			CPABinary:            installed.BinaryPath,
			CPAConfigPath:        filepath.Join(root, "cpa", "config.yaml"),
			CPAAuthDir:           filepath.Join(root, "cpa", "auths"),
			CPAManagementKeyPath: filepath.Join(root, "runtime", "cpa-management.key"),
			RuntimeKeyPath:       filepath.Join(root, "runtime", "control.key"),
		},
		state:     reopened,
		router:    router,
		secrets:   Secrets{RuntimeKey: "runtime-key", CPAManagementKey: "provisioned-key"},
		installer: installer,
	}

	result, err := controller.ProvisionSlim(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if installer.installs != 0 {
		t.Fatalf("retry reinstalled CPA %d times", installer.installs)
	}
	if result.Deployment.Mode != model.DeploymentModeIntegrated ||
		result.CPAUpstreamURL != provisionedCPA.URL ||
		result.CPAManagementKey != "provisioned-key" ||
		result.Installed == nil || result.Installed.Version != installed.Version {
		t.Fatalf("resumed Slim provision = %+v", result)
	}
	if got := controller.managerSpec().Env["CPA_MANAGER_DEPLOYMENT_MODE"]; got != string(model.DeploymentModeSlim) {
		t.Fatalf("manager deployment mode during pending transition = %q", got)
	}

	deployment, err := controller.RevertSlimProvision(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Mode != model.DeploymentModeSlim {
		t.Fatalf("reverted deployment = %+v", deployment)
	}
	if _, err := controller.RevertSlimProvision(t.Context()); err != nil {
		t.Fatalf("idempotent revert failed: %v", err)
	}
	persisted, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeSlim, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := persisted.Snapshot()
	if snapshot.SlimTransition != nil || snapshot.Deployment.Mode != model.DeploymentModeSlim ||
		snapshot.CPAUpstreamURL != previousCPA.URL {
		t.Fatalf("persisted state after revert = %+v", snapshot)
	}
	routed := httptest.NewRecorder()
	router.ServeHTTP(routed, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if routed.Code != http.StatusOK || routed.Body.String() != "previous:/v1/chat/completions" {
		t.Fatalf("routed response after restart rollback = %d %q", routed.Code, routed.Body.String())
	}
}

func TestStartCPARejectsMissingSupervisor(t *testing.T) {
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	controller := &RuntimeController{ctx: context.Background(), state: state, cfg: Config{CPABinary: "/runtime/cpa"}}
	if err := controller.StartCPA(); err == nil {
		t.Fatal("expected missing supervisor error")
	}
}
