package setup

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/app"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/collector"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/managedruntime"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
	collectorsvc "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/collector"
	managerconfigsvc "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/managerconfig"
	runtimecontrolsvc "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/runtimecontrol"
	setupsvc "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/setup"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/store"
)

func TestSetupRollsBackSlimRuntimeWhenCPAValidationFails(t *testing.T) {
	invalidCPA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid management key", http.StatusUnauthorized)
	}))
	t.Cleanup(invalidCPA.Close)

	deployment := model.DefaultDeploymentState(
		model.DeploymentModeSlim,
		"/panel",
		model.PanelBasePathSourceRuntime,
	)
	var callsMu sync.Mutex
	var runtimeCalls []string
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callsMu.Lock()
		runtimeCalls = append(runtimeCalls, r.Method+" "+r.URL.Path)
		callsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v0/runtime/slim/existing":
			_ = json.NewEncoder(w).Encode(managedruntime.SlimProvisionResult{
				OK:             true,
				CPAUpstreamURL: invalidCPA.URL,
				Deployment:     deployment,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/revert":
			_ = json.NewEncoder(w).Encode(map[string]any{"deployment": deployment})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(runtimeServer.Close)

	dataDir := t.TempDir()
	st, err := store.Open(filepath.Join(dataDir, "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveDeploymentState(t.Context(), deployment); err != nil {
		t.Fatal(err)
	}
	adminKey := "cpamp_TestAdminKey!234567890"
	credential, err := security.NewAdminCredential(adminKey, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveAdminCredential(t.Context(), credential); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		DataDir:           dataDir,
		DBPath:            filepath.Join(dataDir, "usage.sqlite"),
		DeploymentMode:    string(model.DeploymentModeSlim),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
		PanelBasePath:     "/panel",
		Queue:             "usage",
		PopSide:           "left",
		BatchSize:         100,
		PollInterval:      500 * time.Millisecond,
		QueryLimit:        50000,
	}
	collectorManager := collector.NewManager(cfg, st)
	collectorService := collectorsvc.New(collectorManager)
	managerConfigService := managerconfigsvc.New(cfg, st, collectorService)
	handler := &Handler{App: &app.Context{
		SetupService: setupsvc.New(
			cfg,
			st,
			collectorService,
			managerConfigService,
			time.Now().UnixMilli(),
			"test-service",
		),
		RuntimeControlService: runtimecontrolsvc.New(cfg, st),
	}}
	payload, err := json.Marshal(setupsvc.Request{
		CPAUpstreamURL:           invalidCPA.URL,
		CPAManagementKey:         "wrong-key",
		RequestMonitoringEnabled: boolPointer(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/setup", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+adminKey)
	response := httptest.NewRecorder()
	handler.Setup(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	callsMu.Lock()
	gotCalls := slices.Clone(runtimeCalls)
	callsMu.Unlock()
	wantCalls := []string{
		http.MethodPut + " /v0/runtime/slim/existing",
		http.MethodPost + " /v0/runtime/slim/revert",
	}
	if !slices.Equal(gotCalls, wantCalls) {
		t.Fatalf("runtime calls = %v, want %v", gotCalls, wantCalls)
	}
	storedDeployment, ok, err := st.LoadDeploymentState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !ok || storedDeployment.Mode != model.DeploymentModeSlim {
		t.Fatalf("stored deployment after rollback = %+v, ok=%v", storedDeployment, ok)
	}
}

func TestAdminKeyReconcilesPersistedSlimTransitionBeforeCompletingSetup(t *testing.T) {
	const (
		bootstrapToken = "bootstrap-token"
		managementKey  = "management-key"
		adminKey       = "cpamp_RecoveredAdmin!123456789"
	)
	cpaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v0/management/config" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+managementKey {
			http.Error(w, "invalid management key", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(cpaServer.Close)

	slim := model.DefaultDeploymentState(
		model.DeploymentModeSlim,
		"/panel",
		model.PanelBasePathSourceRuntime,
	)
	integrated := model.DefaultDeploymentState(
		model.DeploymentModeIntegrated,
		"/panel",
		model.PanelBasePathSourceRuntime,
	)
	pending := managedruntime.State{
		Deployment:     integrated,
		CPAUpstreamURL: cpaServer.URL,
		SlimTransition: &managedruntime.SlimTransition{
			Kind:                   "provision",
			PreviousDeployment:     slim,
			PreviousCPAUpstreamURL: "http://old-cpa:8317",
			TargetCPAUpstreamURL:   cpaServer.URL,
		},
	}
	var commitCalls atomic.Int32
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/runtime/status":
			_ = json.NewEncoder(w).Encode(pending)
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/commit":
			commitCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(runtimeServer.Close)

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "usage.sqlite")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveDeploymentState(t.Context(), slim); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSetup(t.Context(), store.Setup{
		CPAUpstreamURL: cpaServer.URL,
		ManagementKey:  managementKey,
	}); err != nil {
		t.Fatal(err)
	}
	bootstrapCredential, err := security.NewBootstrapCredential(bootstrapToken, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveBootstrapCredential(t.Context(), bootstrapCredential); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		DataDir:           dataDir,
		DBPath:            dbPath,
		DeploymentMode:    string(model.DeploymentModeSlim),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
		PanelBasePath:     "/panel",
		Queue:             "usage",
		PopSide:           "left",
		BatchSize:         100,
		PollInterval:      500 * time.Millisecond,
		QueryLimit:        50000,
	}
	collectorManager := collector.NewManager(cfg, st)
	collectorService := collectorsvc.New(collectorManager)
	managerConfigService := managerconfigsvc.New(cfg, st, collectorService)
	handler := &Handler{App: &app.Context{
		SetupService: setupsvc.New(
			cfg,
			st,
			collectorService,
			managerConfigService,
			time.Now().UnixMilli(),
			"test-service",
		),
		RuntimeControlService: runtimecontrolsvc.New(cfg, st),
	}}
	payload, err := json.Marshal(setupsvc.AdminKeyRequest{AdminKey: adminKey})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/setup/admin-key", bytes.NewReader(payload))
	request.Header.Set("X-CPAMP-Bootstrap-Token", bootstrapToken)
	response := httptest.NewRecorder()
	handler.AdminKey(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if commitCalls.Load() != 1 {
		t.Fatalf("runtime commit calls = %d, want 1", commitCalls.Load())
	}
	credential, ok, err := st.LoadAdminCredential(t.Context())
	if err != nil || !ok || !security.VerifyAdminKey(credential, adminKey) {
		t.Fatalf("admin credential after recovery: ok=%v err=%v", ok, err)
	}
	deployment, ok, err := st.LoadDeploymentState(t.Context())
	if err != nil || !ok || deployment.Mode != model.DeploymentModeIntegrated {
		t.Fatalf("deployment after recovery = %+v, ok=%v, err=%v", deployment, ok, err)
	}
}

func TestSetupWorkflowSerializesAdminInitializationWithSlimConnectionCommit(t *testing.T) {
	const (
		bootstrapToken = "bootstrap-token"
		managementKey  = "management-key"
		adminKey       = "cpamp_SerializedAdmin!123456789"
	)
	validationStarted := make(chan struct{})
	allowValidation := make(chan struct{})
	var allowValidationOnce sync.Once
	releaseValidation := func() {
		allowValidationOnce.Do(func() { close(allowValidation) })
	}
	var validationOnce sync.Once
	cpaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v0/management/config" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+managementKey {
			http.Error(w, "invalid management key", http.StatusUnauthorized)
			return
		}
		validationOnce.Do(func() { close(validationStarted) })
		<-allowValidation
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() {
		releaseValidation()
		cpaServer.Close()
	})

	slim := model.DefaultDeploymentState(
		model.DeploymentModeSlim,
		"/panel",
		model.PanelBasePathSourceRuntime,
	)
	var runtimeMu sync.Mutex
	pendingTransition := false
	var statusCalls atomic.Int32
	var revertCalls atomic.Int32
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v0/runtime/slim/existing":
			runtimeMu.Lock()
			pendingTransition = true
			runtimeMu.Unlock()
			_ = json.NewEncoder(w).Encode(managedruntime.SlimProvisionResult{
				OK:             true,
				CPAUpstreamURL: cpaServer.URL,
				Deployment:     slim,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/commit":
			runtimeMu.Lock()
			pendingTransition = false
			runtimeMu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/revert":
			revertCalls.Add(1)
			runtimeMu.Lock()
			pendingTransition = false
			runtimeMu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"deployment": slim})
		case r.Method == http.MethodGet && r.URL.Path == "/v0/runtime/status":
			statusCalls.Add(1)
			runtimeMu.Lock()
			pending := pendingTransition
			runtimeMu.Unlock()
			state := managedruntime.State{Deployment: slim, CPAUpstreamURL: cpaServer.URL}
			if pending {
				state.SlimTransition = &managedruntime.SlimTransition{
					Kind:                 "existing",
					PreviousDeployment:   slim,
					TargetCPAUpstreamURL: cpaServer.URL,
				}
			}
			_ = json.NewEncoder(w).Encode(state)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(runtimeServer.Close)

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "usage.sqlite")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveDeploymentState(t.Context(), slim); err != nil {
		t.Fatal(err)
	}
	bootstrapCredential, err := security.NewBootstrapCredential(bootstrapToken, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveBootstrapCredential(t.Context(), bootstrapCredential); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		DataDir:           dataDir,
		DBPath:            dbPath,
		DeploymentMode:    string(model.DeploymentModeSlim),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
		PanelBasePath:     "/panel",
		Queue:             "usage",
		PopSide:           "left",
		BatchSize:         100,
		PollInterval:      500 * time.Millisecond,
		QueryLimit:        50000,
	}
	collectorManager := collector.NewManager(cfg, st)
	collectorService := collectorsvc.New(collectorManager)
	managerConfigService := managerconfigsvc.New(cfg, st, collectorService)
	handler := &Handler{App: &app.Context{
		SetupService: setupsvc.New(
			cfg,
			st,
			collectorService,
			managerConfigService,
			time.Now().UnixMilli(),
			"test-service",
		),
		RuntimeControlService: runtimecontrolsvc.New(cfg, st),
	}}

	setupPayload, err := json.Marshal(setupsvc.Request{
		CPAUpstreamURL:           cpaServer.URL,
		CPAManagementKey:         managementKey,
		RequestMonitoringEnabled: boolPointer(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	setupRequest := httptest.NewRequest(http.MethodPost, "/setup", bytes.NewReader(setupPayload))
	setupRequest.Header.Set("X-CPAMP-Bootstrap-Token", bootstrapToken)
	setupResponse := httptest.NewRecorder()
	setupDone := make(chan struct{})
	go func() {
		handler.Setup(setupResponse, setupRequest)
		close(setupDone)
	}()

	select {
	case <-validationStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("Slim setup did not reach CPA validation")
	}

	adminPayload, err := json.Marshal(setupsvc.AdminKeyRequest{AdminKey: adminKey})
	if err != nil {
		t.Fatal(err)
	}
	adminRequest := httptest.NewRequest(http.MethodPost, "/setup/admin-key", bytes.NewReader(adminPayload))
	adminRequest.Header.Set("X-CPAMP-Bootstrap-Token", bootstrapToken)
	adminResponse := httptest.NewRecorder()
	adminDone := make(chan struct{})
	go func() {
		handler.AdminKey(adminResponse, adminRequest)
		close(adminDone)
	}()

	select {
	case <-adminDone:
		t.Fatal("admin initialization completed before the Slim setup workflow committed")
	case <-time.After(100 * time.Millisecond):
	}
	if statusCalls.Load() != 0 || revertCalls.Load() != 0 {
		t.Fatalf(
			"admin recovery overlapped Slim setup: status calls=%d revert calls=%d",
			statusCalls.Load(),
			revertCalls.Load(),
		)
	}

	releaseValidation()
	for name, done := range map[string]<-chan struct{}{
		"setup": setupDone,
		"admin": adminDone,
	} {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s workflow did not complete", name)
		}
	}
	if setupResponse.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body = %s", setupResponse.Code, setupResponse.Body.String())
	}
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin status = %d, body = %s", adminResponse.Code, adminResponse.Body.String())
	}
	if revertCalls.Load() != 0 {
		t.Fatalf("Slim transition was reverted after serialized setup: %d", revertCalls.Load())
	}
}

func TestSetupWorkflowReauthorizesQueuedMutationAfterBootstrapTokenConsumption(t *testing.T) {
	const (
		bootstrapToken = "bootstrap-token"
		managementKey  = "management-key"
		adminKey       = "cpamp_ConsumedBootstrap!123456789"
	)
	cpaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v0/management/config" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+managementKey {
			http.Error(w, "invalid management key", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(cpaServer.Close)

	slim := model.DefaultDeploymentState(
		model.DeploymentModeSlim,
		"/panel",
		model.PanelBasePathSourceRuntime,
	)
	statusStarted := make(chan struct{})
	allowStatus := make(chan struct{})
	var statusStartedOnce sync.Once
	var allowStatusOnce sync.Once
	releaseStatus := func() {
		allowStatusOnce.Do(func() { close(allowStatus) })
	}
	defer releaseStatus()
	var slimMutationCalls atomic.Int32
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/runtime/status":
			statusStartedOnce.Do(func() { close(statusStarted) })
			<-allowStatus
			_ = json.NewEncoder(w).Encode(managedruntime.State{Deployment: slim})
		case r.Method == http.MethodPut && r.URL.Path == "/v0/runtime/slim/existing":
			slimMutationCalls.Add(1)
			_ = json.NewEncoder(w).Encode(managedruntime.SlimProvisionResult{
				OK:             true,
				CPAUpstreamURL: cpaServer.URL,
				Deployment:     slim,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/commit":
			slimMutationCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(runtimeServer.Close)

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "usage.sqlite")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveDeploymentState(t.Context(), slim); err != nil {
		t.Fatal(err)
	}
	bootstrapCredential, err := security.NewBootstrapCredential(bootstrapToken, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveBootstrapCredential(t.Context(), bootstrapCredential); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		DataDir:           dataDir,
		DBPath:            dbPath,
		DeploymentMode:    string(model.DeploymentModeSlim),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
		PanelBasePath:     "/panel",
		Queue:             "usage",
		PopSide:           "left",
		BatchSize:         100,
		PollInterval:      500 * time.Millisecond,
		QueryLimit:        50000,
	}
	collectorManager := collector.NewManager(cfg, st)
	collectorService := collectorsvc.New(collectorManager)
	managerConfigService := managerconfigsvc.New(cfg, st, collectorService)
	handler := &Handler{App: &app.Context{
		SetupService: setupsvc.New(
			cfg,
			st,
			collectorService,
			managerConfigService,
			time.Now().UnixMilli(),
			"test-service",
		),
		RuntimeControlService: runtimecontrolsvc.New(cfg, st),
	}}

	adminPayload, err := json.Marshal(setupsvc.AdminKeyRequest{AdminKey: adminKey})
	if err != nil {
		t.Fatal(err)
	}
	adminRequest := httptest.NewRequest(http.MethodPost, "/setup/admin-key", bytes.NewReader(adminPayload))
	adminRequest.Header.Set("X-CPAMP-Bootstrap-Token", bootstrapToken)
	adminResponse := httptest.NewRecorder()
	adminDone := make(chan struct{})
	go func() {
		handler.AdminKey(adminResponse, adminRequest)
		close(adminDone)
	}()

	select {
	case <-statusStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("admin initialization did not reach Slim reconciliation")
	}

	setupPayload, err := json.Marshal(setupsvc.Request{
		CPAUpstreamURL:           cpaServer.URL,
		CPAManagementKey:         managementKey,
		RequestMonitoringEnabled: boolPointer(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	setupRequest := httptest.NewRequest(http.MethodPost, "/setup", bytes.NewReader(setupPayload))
	setupRequest.Header.Set("X-CPAMP-Bootstrap-Token", bootstrapToken)
	setupResponse := httptest.NewRecorder()
	setupDone := make(chan struct{})
	go func() {
		handler.Setup(setupResponse, setupRequest)
		close(setupDone)
	}()

	select {
	case <-setupDone:
		t.Fatal("queued setup mutation completed before admin initialization released the workflow lock")
	case <-time.After(100 * time.Millisecond):
	}

	releaseStatus()
	for name, done := range map[string]<-chan struct{}{
		"admin": adminDone,
		"setup": setupDone,
	} {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s workflow did not complete", name)
		}
	}
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin status = %d, body = %s", adminResponse.Code, adminResponse.Body.String())
	}
	if setupResponse.Code != http.StatusUnauthorized {
		t.Fatalf("queued setup status = %d, body = %s", setupResponse.Code, setupResponse.Body.String())
	}
	if slimMutationCalls.Load() != 0 {
		t.Fatalf("queued setup mutated Slim runtime after bootstrap consumption: %d", slimMutationCalls.Load())
	}
}

func boolPointer(value bool) *bool {
	return &value
}
