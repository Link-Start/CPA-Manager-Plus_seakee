package managedruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
)

type fakeRuntimeReleaseInstaller struct {
	manifest ReleaseManifest
	results  map[string]InstalledComponent
	errors   map[string]error
	installs int
}

func TestCheckUpdatesRejectsManifestMissingARequiredComponent(t *testing.T) {
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(
			model.DeploymentModeIntegrated,
			"/panel",
			model.PanelBasePathSourceRuntime,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	controller := &RuntimeController{
		state: state,
		installer: &fakeRuntimeReleaseInstaller{manifest: ReleaseManifest{
			Components: map[string]ReleaseComponent{
				"cpamp": {Version: "v2.0.0"},
			},
		}},
	}

	_, err = controller.CheckUpdates(t.Context())
	if err == nil || !strings.Contains(err.Error(), `component "cpa"`) {
		t.Fatalf("CheckUpdates() error = %v", err)
	}
}

func TestCheckUpdatesReportsCPACompatibilityWithCurrentAndAvailableCPAMP(t *testing.T) {
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, version := range map[string]string{"cpamp": "v1.11.9", "cpa": "v7.1.18"} {
		if err := state.UpdateComponent(name, func(component ComponentState) ComponentState {
			component.Version = version
			return component
		}); err != nil {
			t.Fatal(err)
		}
	}
	controller := &RuntimeController{
		state: state,
		installer: &fakeRuntimeReleaseInstaller{manifest: ReleaseManifest{
			Components: map[string]ReleaseComponent{
				"cpamp": {Version: "v1.12.0"},
				"cpa": {
					Version:         "v7.2.0",
					MinCPAMPVersion: "v1.12.0",
				},
			},
		}},
	}

	result, err := controller.CheckUpdates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cpa := result.Components["cpa"]
	if cpa.StandaloneUpdateAllowed {
		t.Fatalf("CPA standalone update allowed = true, want false: %+v", cpa)
	}
	if !cpa.CombinedUpdateAllowed {
		t.Fatalf("CPA combined update allowed = false, want true: %+v", cpa)
	}
}

func TestStartUpdateRejectsCPAWhenCurrentCPAMPIsTooOld(t *testing.T) {
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Version = "v1.11.9"
		return component
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpa", func(component ComponentState) ComponentState {
		component.Version = "v7.1.18"
		return component
	}); err != nil {
		t.Fatal(err)
	}
	controller := &RuntimeController{
		state: state,
		installer: &fakeRuntimeReleaseInstaller{manifest: ReleaseManifest{
			Components: map[string]ReleaseComponent{
				"cpa": {
					Version:         "v7.2.0",
					MinCPAMPVersion: "v1.12.0",
				},
			},
		}},
	}

	_, err = controller.StartUpdate(t.Context(), "cpa")
	if err == nil || !strings.Contains(err.Error(), "requires CPAMP v1.12.0 or newer") {
		t.Fatalf("StartUpdate() error = %v", err)
	}
	if snapshot := state.Snapshot(); len(snapshot.Operations) != 0 {
		t.Fatalf("incompatible update persisted operations: %+v", snapshot.Operations)
	}
}

func (f *fakeRuntimeReleaseInstaller) FetchManifest(context.Context) (ReleaseManifest, error) {
	return f.manifest, nil
}

func (f *fakeRuntimeReleaseInstaller) InstallLatest(ctx context.Context, name string) (InstalledComponent, error) {
	component, ok := f.manifest.Components[name]
	if !ok {
		return InstalledComponent{}, errors.New("missing component")
	}
	return f.InstallComponent(ctx, name, component)
}

func (f *fakeRuntimeReleaseInstaller) InstallComponent(_ context.Context, name string, _ ReleaseComponent) (InstalledComponent, error) {
	f.installs++
	if err := f.errors[name]; err != nil {
		return InstalledComponent{}, err
	}
	return f.results[name], nil
}

func TestRuntimeUpdateDoesNotInstallWhenOperationStateCannotPersist(t *testing.T) {
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	operation := UpdateOperation{
		ID:         "update_persistence_failure",
		Kind:       "cpamp",
		Status:     "queued",
		Components: []string{"cpamp"},
	}
	if err := state.SaveOperation(operation); err != nil {
		t.Fatal(err)
	}
	invalidTarget := filepath.Join(t.TempDir(), "state-directory")
	if err := os.Mkdir(invalidTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	state.path = invalidTarget
	installer := &fakeRuntimeReleaseInstaller{
		results: map[string]InstalledComponent{
			"cpamp": {Name: "cpamp", Version: "v2.0.0", BinaryPath: "/runtime/cpamp-v2"},
		},
		errors: map[string]error{},
	}
	controller := &RuntimeController{
		ctx:            context.Background(),
		state:          state,
		installer:      installer,
		activeUpdateID: operation.ID,
	}
	controller.runUpdateOperation(operation.ID, operation.Components, map[string]ReleaseComponent{
		"cpamp": {Version: "v2.0.0"},
	})
	if installer.installs != 0 {
		t.Fatalf("installer calls = %d, want 0", installer.installs)
	}
	stored, ok := state.Operation(operation.ID)
	if !ok || stored.Status != "queued" {
		t.Fatalf("operation after persistence failure = %+v, ok=%v", stored, ok)
	}
	controller.updateMu.Lock()
	activeID := controller.activeUpdateID
	controller.updateMu.Unlock()
	if activeID != "" {
		t.Fatalf("active update ID = %q after persistence failure", activeID)
	}
}

func TestStartUpdateRefreshesComponentSnapshotAfterWaitingForLock(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(health.Close)
	root := t.TempDir()
	previousBinary := filepath.Join(root, "cpamp-v1")
	targetBinary := filepath.Join(root, "cpamp-v2")
	for _, binary := range []string{previousBinary, targetBinary} {
		if err := os.WriteFile(binary, []byte("binary"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	state, err := OpenStateStore(
		filepath.Join(root, "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Version = "v1.0.0"
		component.BinaryPath = previousBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}
	installer := &fakeRuntimeReleaseInstaller{
		manifest: ReleaseManifest{Components: map[string]ReleaseComponent{
			"cpamp": {Version: "v2.0.0"},
		}},
		results: map[string]InstalledComponent{
			"cpamp": {Name: "cpamp", Version: "v2.0.0", BinaryPath: targetBinary},
		},
		errors: map[string]error{},
	}
	controller := &RuntimeController{
		ctx: context.Background(),
		cfg: Config{
			ManagerURL:   health.URL,
			ComponentDir: filepath.Join(root, "components"),
		},
		state:      state,
		supervisor: &fakeRuntimeProcessSupervisor{state: state, nextPID: 100},
		installer:  installer,
		handoff:    make(chan runtimeHandoff, 1),
	}

	controller.updateMu.Lock()
	result := make(chan error, 1)
	go func() {
		_, startErr := controller.StartUpdate(context.Background(), "cpamp")
		result <- startErr
	}()
	time.Sleep(25 * time.Millisecond)
	if err := state.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Version = "v2.0.0"
		component.BinaryPath = targetBinary
		return component
	}); err != nil {
		controller.updateMu.Unlock()
		t.Fatal(err)
	}
	controller.updateMu.Unlock()

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "already current") {
			t.Fatalf("StartUpdate() after concurrent completion error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartUpdate() did not resume after releasing the update lock")
	}
	if installer.installs != 0 {
		t.Fatalf("installer calls after concurrent completion = %d, want 0", installer.installs)
	}
}

func TestStartUpdateRejectsSlimTransitionCreatedWhileWaitingForRuntimeMutationLock(t *testing.T) {
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	controller := &RuntimeController{state: state}

	controller.runtimeMutationMu.Lock()
	result := make(chan error, 1)
	go func() {
		_, startErr := controller.StartUpdate(context.Background(), "cpa")
		result <- startErr
	}()
	time.Sleep(25 * time.Millisecond)
	if _, err := state.BeginSlimTransition(
		slimTransitionProvision,
		"http://127.0.0.1:8317",
		"",
		false,
		nil,
	); err != nil {
		controller.runtimeMutationMu.Unlock()
		t.Fatal(err)
	}
	controller.runtimeMutationMu.Unlock()

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "Slim runtime transition") {
			t.Fatalf("StartUpdate() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartUpdate() did not resume after releasing the runtime mutation lock")
	}
	if snapshot := state.Snapshot(); len(snapshot.Operations) != 0 {
		t.Fatalf("StartUpdate() persisted an operation during a Slim transition: %+v", snapshot.Operations)
	}
}

func TestSlimTransitionRejectsUpdateCreatedWhileWaitingForRuntimeMutationLock(t *testing.T) {
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeSlim, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	controller := &RuntimeController{state: state}

	controller.runtimeMutationMu.Lock()
	result := make(chan error, 1)
	go func() {
		_, transitionErr := controller.UseExistingCPA("http://127.0.0.1:8317")
		result <- transitionErr
	}()
	time.Sleep(25 * time.Millisecond)
	if err := state.SaveOperation(UpdateOperation{
		ID:         "update_in_progress",
		Kind:       "cpa",
		Status:     runtimeUpdateStatusQueued,
		Components: []string{"cpa"},
	}); err != nil {
		controller.runtimeMutationMu.Unlock()
		t.Fatal(err)
	}
	controller.runtimeMutationMu.Unlock()

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "update_in_progress") {
			t.Fatalf("UseExistingCPA() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("UseExistingCPA() did not resume after releasing the runtime mutation lock")
	}
	if transition := state.Snapshot().SlimTransition; transition != nil {
		t.Fatalf("UseExistingCPA() began a Slim transition during an update: %+v", transition)
	}
}

type fakeRuntimeProcessSupervisor struct {
	state   *StateStore
	nextPID int
}

func (*fakeRuntimeProcessSupervisor) Run(context.Context, string, ProcessSpecProvider) {}

func (f *fakeRuntimeProcessSupervisor) Restart(name string) error {
	f.nextPID++
	return f.state.UpdateComponent(name, func(component ComponentState) ComponentState {
		component.Status = "running"
		component.PID = f.nextPID
		return component
	})
}

func TestReleaseVersionComparison(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		newer   bool
	}{
		{latest: "v1.12.0", current: "v1.11.9", newer: true},
		{latest: "v1.11.9", current: "1.11.9", newer: false},
		{latest: "v1.12.0-rc.1", current: "v1.12.0", newer: false},
		{latest: "v1.12.0", current: "v1.12.0-rc.1", newer: true},
		{latest: "v1.12.0-rc.10", current: "v1.12.0-rc.2", newer: true},
		{latest: "v1.12.0-rc.2", current: "v1.12.0-rc.10", newer: false},
		{latest: "v1.12.0-rc.1", current: "v1.12.0-rc.beta", newer: false},
		{latest: "v1.12.0-rc.1.1", current: "v1.12.0-rc.1", newer: true},
		{latest: "v7.2.0", current: "dev", newer: true},
	}
	for _, test := range tests {
		if got := releaseVersionIsNewer(test.latest, test.current); got != test.newer {
			t.Fatalf("releaseVersionIsNewer(%q, %q) = %v", test.latest, test.current, got)
		}
	}
}

func TestParseReleaseVersionRequiresThreePartCore(t *testing.T) {
	for _, version := range []string{"v1", "v1.2", "v1.2.3.4"} {
		t.Run(version, func(t *testing.T) {
			if _, _, ok := parseReleaseVersion(version); ok {
				t.Fatalf("parseReleaseVersion(%q) accepted a non-standard core version", version)
			}
		})
	}
}

func TestRuntimeOperationRequiresRuntimeKeyOrOperationToken(t *testing.T) {
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", "runtime"),
	)
	if err != nil {
		t.Fatal(err)
	}
	token := "op_test_token"
	operation := UpdateOperation{
		ID:                 "update_test",
		Kind:               "all",
		Status:             "running",
		Components:         []string{"cpamp", "cpa"},
		CurrentBinaries:    map[string]string{"cpamp": "/private/runtime/cpamp"},
		OperationTokenHash: hashOperationToken(token),
	}
	if err := state.SaveOperation(operation); err != nil {
		t.Fatal(err)
	}
	controller := &RuntimeController{ctx: context.Background(), state: state}
	handler := NewControlHandler(controller, "runtime-key")

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(
		unauthorized,
		httptest.NewRequest(http.MethodGet, "/v0/runtime/operations/update_test", nil),
	)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	queryToken := httptest.NewRecorder()
	handler.ServeHTTP(
		queryToken,
		httptest.NewRequest(
			http.MethodGet,
			"/v0/runtime/operations/update_test?token="+token,
			nil,
		),
	)
	if queryToken.Code != http.StatusUnauthorized {
		t.Fatalf("query token status = %d, want %d", queryToken.Code, http.StatusUnauthorized)
	}

	authorizedRequest := httptest.NewRequest(
		http.MethodGet,
		"/v0/runtime/operations/update_test",
		nil,
	)
	authorizedRequest.Header.Set("X-CPAMP-Operation-Token", token)
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, body = %s", authorized.Code, authorized.Body.String())
	}
	var result UpdateOperation
	if err := json.NewDecoder(authorized.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.ID != operation.ID || result.OperationTokenHash != "" || result.CurrentBinaries != nil {
		t.Fatalf("operation response = %+v", result)
	}
}

func TestStateStoreMigratesSchemaOneAndPersistsOperations(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	legacy := `{
  "schemaVersion": 1,
  "deployment": {
    "schemaVersion": 1,
    "mode": "integrated",
    "panelBasePath": "/panel",
    "runtimeManaged": true,
    "cpampUpdatesManaged": true,
    "cpaUpdatesManaged": true
  },
  "components": {}
}`
	if err := writeAtomicRuntimeFile(statePath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", "runtime"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if store.Snapshot().SchemaVersion != runtimeStateSchemaVersion {
		t.Fatalf("schema version = %d", store.Snapshot().SchemaVersion)
	}
	if err := store.SaveOperation(UpdateOperation{
		ID:         "update_saved",
		Kind:       "cpa",
		Status:     "succeeded",
		Components: []string{"cpa"},
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", "runtime"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if operation, ok := reopened.Operation("update_saved"); !ok || operation.Status != "succeeded" {
		t.Fatalf("reopened operation = %+v, %v", operation, ok)
	}
}

func TestStateStoreOperationTimestampsAdvanceMonotonically(t *testing.T) {
	store, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOperation(UpdateOperation{
		ID:         "update_monotonic_timestamp",
		Kind:       "cpamp",
		Status:     "queued",
		Components: []string{"cpamp"},
	}); err != nil {
		t.Fatal(err)
	}

	futureUpdatedAt := time.Now().Add(time.Hour).UnixMilli()
	store.mu.Lock()
	operation := store.state.Operations["update_monotonic_timestamp"]
	operation.UpdatedAtMS = futureUpdatedAt
	store.state.Operations[operation.ID] = operation
	store.mu.Unlock()

	updated, err := store.UpdateOperation(operation.ID, func(current UpdateOperation) UpdateOperation {
		current.Status = "running"
		return current
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.UpdatedAtMS != futureUpdatedAt+1 {
		t.Fatalf("UpdatedAtMS = %d, want %d", updated.UpdatedAtMS, futureUpdatedAt+1)
	}
}

func TestStateStoreMarksInterruptedUpdateFailedOnStartup(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOperation(UpdateOperation{
		ID:         "update_interrupted",
		Kind:       "all",
		Status:     "rolling_back",
		Progress:   90,
		Components: []string{"cpamp", "cpa"},
	}); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := reopened.Operation("update_interrupted")
	if !ok || operation.Status != "failed" || operation.Progress != 100 || operation.CompletedAtMS == 0 || operation.Error == "" {
		t.Fatalf("recovered operation = %+v, ok=%v", operation, ok)
	}
}

func TestStateStorePreservesPendingHandoffUntilReplacementConfirms(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	store, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOperation(UpdateOperation{
		ID:         "update_pending_handoff",
		Kind:       "cpamp",
		Status:     runtimeUpdateStatusHandoffPending,
		Progress:   95,
		Components: []string{"cpamp"},
	}); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := reopened.Operation("update_pending_handoff")
	if !ok || operation.Status != runtimeUpdateStatusHandoffPending || operation.CompletedAtMS != 0 {
		t.Fatalf("pending handoff after restart = %+v, ok=%v", operation, ok)
	}
	completed, err := reopened.CompletePendingHandoff(operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != runtimeUpdateStatusSucceeded || completed.Progress != 100 || completed.CompletedAtMS == 0 {
		t.Fatalf("confirmed handoff = %+v", completed)
	}
}

func TestStateStoreRollsBackPendingHandoffFailure(t *testing.T) {
	root := t.TempDir()
	previousBinary := filepath.Join(root, "cpamp-v1")
	targetBinary := filepath.Join(root, "cpamp-v2")
	for _, binary := range []string{previousBinary, targetBinary} {
		if err := os.WriteFile(binary, []byte("binary"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store, err := OpenStateStore(
		filepath.Join(root, "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Status = "stopped"
		component.Version = "v2.0.0"
		component.BinaryPath = targetBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOperation(UpdateOperation{
		ID:              "update_handoff_failed",
		Kind:            "cpamp",
		Status:          runtimeUpdateStatusHandoffPending,
		Progress:        95,
		Components:      []string{"cpamp"},
		CurrentVersions: map[string]string{"cpamp": "v1.0.0"},
		CurrentBinaries: map[string]string{"cpamp": previousBinary},
		TargetVersions:  map[string]string{"cpamp": "v2.0.0"},
		Results: map[string]ComponentUpdateResult{
			"cpamp": {Name: "cpamp", FromVersion: "v1.0.0", ToVersion: "v2.0.0", Status: "succeeded"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	operation, err := store.FailPendingHandoff(
		"update_handoff_failed",
		errors.New("exec format error"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != runtimeUpdateStatusRolledBack || !operation.RollbackSuccessful ||
		!strings.Contains(operation.Error, "exec format error") {
		t.Fatalf("failed handoff operation = %+v", operation)
	}
	component := store.Snapshot().Components["cpamp"]
	if component.Version != "v1.0.0" || component.BinaryPath != previousBinary || component.Status != "recovered" {
		t.Fatalf("component after failed handoff = %+v", component)
	}
}

func TestStateStoreMigratesInstallerManagedRuntimeCapability(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	legacy := State{
		SchemaVersion: runtimeStateSchemaVersion,
		Deployment: model.DeploymentState{
			SchemaVersion:       1,
			Mode:                model.DeploymentModeInstallerManaged,
			PanelBasePath:       "/panel",
			PanelBasePathSource: model.PanelBasePathSourceRuntime,
			RuntimeManaged:      false,
		},
		Components: map[string]ComponentState{},
		Operations: map[string]UpdateOperation{},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(
			model.DeploymentModeInstallerManaged,
			"/panel",
			model.PanelBasePathSourceRuntime,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	deployment := state.Snapshot().Deployment
	if !deployment.RuntimeManaged || deployment.CPAMPUpdatesManaged || deployment.CPAUpdatesManaged {
		t.Fatalf("migrated installer-managed deployment = %+v", deployment)
	}
}

func TestStateStoreRestoresInterruptedUpdateComponentsOnStartup(t *testing.T) {
	root := t.TempDir()
	previousBinary := filepath.Join(root, "cpamp-v1")
	targetBinary := filepath.Join(root, "cpamp-v2")
	if err := os.WriteFile(previousBinary, []byte("previous"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetBinary, []byte("target"), 0o700); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, "state.json")
	store, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Status = "running"
		component.PID = 42
		component.Version = "v2.0.0"
		component.BinaryPath = targetBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOperation(UpdateOperation{
		ID:              "update_interrupted_recoverable",
		Kind:            "cpamp",
		Status:          "running",
		Progress:        75,
		Components:      []string{"cpamp"},
		CurrentVersions: map[string]string{"cpamp": "v1.0.0"},
		CurrentBinaries: map[string]string{"cpamp": previousBinary},
		TargetVersions:  map[string]string{"cpamp": "v2.0.0"},
		Results: map[string]ComponentUpdateResult{
			"cpamp": {Name: "cpamp", FromVersion: "v1.0.0", ToVersion: "v2.0.0", Status: "succeeded"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	component := reopened.Snapshot().Components["cpamp"]
	if component.Version != "v1.0.0" || component.BinaryPath != previousBinary || component.Status != "recovered" || component.PID != 0 {
		t.Fatalf("recovered component = %+v", component)
	}
	operation, ok := reopened.Operation("update_interrupted_recoverable")
	if !ok || operation.Status != "rolled_back" || !operation.RollbackAttempted || !operation.RollbackSuccessful {
		t.Fatalf("recovered operation = %+v, ok=%v", operation, ok)
	}
	if result := operation.Results["cpamp"]; result.Status != "rolled_back" || result.RollbackVersion != "v1.0.0" {
		t.Fatalf("recovered component result = %+v", result)
	}
}

func TestStateStoreReportsInterruptedRollbackWhenPreviousBinaryIsMissing(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	store, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	targetBinary := filepath.Join(root, "cpamp-v2")
	if err := os.WriteFile(targetBinary, []byte("target"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Version = "v2.0.0"
		component.BinaryPath = targetBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}
	missingBinary := filepath.Join(root, "missing-cpamp-v1")
	if err := store.SaveOperation(UpdateOperation{
		ID:              "update_interrupted_missing_binary",
		Kind:            "cpamp",
		Status:          "running",
		Components:      []string{"cpamp"},
		CurrentVersions: map[string]string{"cpamp": "v1.0.0"},
		CurrentBinaries: map[string]string{"cpamp": missingBinary},
		TargetVersions:  map[string]string{"cpamp": "v2.0.0"},
	}); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenStateStore(
		statePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := reopened.Operation("update_interrupted_missing_binary")
	if !ok || operation.Status != "failed" || !operation.RollbackAttempted || operation.RollbackSuccessful ||
		!strings.Contains(operation.Error, "rollback failed") {
		t.Fatalf("failed recovery operation = %+v, ok=%v", operation, ok)
	}
	if result := operation.Results["cpamp"]; result.Status != "rollback_failed" || result.Error == "" {
		t.Fatalf("failed recovery result = %+v", result)
	}
	component := reopened.Snapshot().Components["cpamp"]
	if component.Version != "v2.0.0" || component.BinaryPath != targetBinary {
		t.Fatalf("component changed despite unavailable rollback binary = %+v", component)
	}
}

func TestStateStoreRestoresMemoryWhenPersistenceFails(t *testing.T) {
	store, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeSlim, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot()
	invalidTarget := filepath.Join(t.TempDir(), "state-directory")
	if err := os.Mkdir(invalidTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	store.path = invalidTarget
	if _, err := store.UpdateCPAUpstream("http://new-cpa:8317"); err == nil {
		t.Fatal("UpdateCPAUpstream() error = nil")
	}
	after := store.Snapshot()
	if after.CPAUpstreamURL != before.CPAUpstreamURL || after.UpdatedAtMS != before.UpdatedAtMS {
		t.Fatalf("state changed after failed persistence: before=%+v after=%+v", before, after)
	}
}

func TestStateStoreReenablesRetiredPanelPath(t *testing.T) {
	store, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/first", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePanelBasePath("/second"); err != nil {
		t.Fatal(err)
	}
	deployment, err := store.UpdatePanelBasePath("/first")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(deployment.RetiredPanelPaths, "/first") || !slices.Contains(deployment.RetiredPanelPaths, "/second") {
		t.Fatalf("retired panel paths = %v", deployment.RetiredPanelPaths)
	}
}

func TestRuntimeUpdateRollsBackAppliedComponentsWhenALaterComponentFails(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(health.Close)
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Name = "cpamp"
		component.Status = "running"
		component.PID = 10
		component.Version = "v1.0.0"
		component.BinaryPath = "/runtime/cpamp-v1"
		return component
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpa", func(component ComponentState) ComponentState {
		component.Name = "cpa"
		component.Status = "running"
		component.PID = 20
		component.Version = "v7.1.0"
		component.BinaryPath = "/runtime/cpa-v7.1.0"
		return component
	}); err != nil {
		t.Fatal(err)
	}
	installer := &fakeRuntimeReleaseInstaller{
		manifest: ReleaseManifest{Components: map[string]ReleaseComponent{
			"cpamp": {Version: "v1.1.0"},
			"cpa":   {Version: "v7.2.0"},
		}},
		results: map[string]InstalledComponent{
			"cpamp": {Name: "cpamp", Version: "v1.1.0", BinaryPath: "/runtime/cpamp-v1.1.0"},
		},
		errors: map[string]error{"cpa": errors.New("CPA checksum mismatch")},
	}
	componentDir := t.TempDir()
	controller := &RuntimeController{
		ctx: context.Background(),
		cfg: Config{
			ManagerURL:   health.URL,
			CPAURL:       health.URL,
			ComponentDir: componentDir,
		},
		state:      state,
		supervisor: &fakeRuntimeProcessSupervisor{state: state, nextPID: 100},
		installer:  installer,
	}
	started, err := controller.StartUpdate(context.Background(), "all")
	if err != nil {
		t.Fatal(err)
	}
	if !controller.AuthorizeOperation(started.Operation.ID, started.OperationToken) {
		t.Fatal("operation token was not accepted")
	}

	deadline := time.Now().Add(2 * time.Second)
	var operation UpdateOperation
	for time.Now().Before(deadline) {
		operation, _ = state.Operation(started.Operation.ID)
		if operation.Status == "rolled_back" || operation.Status == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if operation.Status != "rolled_back" || !operation.RollbackAttempted || !operation.RollbackSuccessful {
		t.Fatalf("operation = %+v", operation)
	}
	if result := operation.Results["cpamp"]; result.Status != "rolled_back" || result.RollbackVersion != "v1.0.0" {
		t.Fatalf("CPAMP result = %+v", result)
	}
	if result := operation.Results["cpa"]; result.Status != "failed" || result.Error == "" {
		t.Fatalf("CPA result = %+v", result)
	}
	component := state.Snapshot().Components["cpamp"]
	if component.Version != "v1.0.0" || component.BinaryPath != "/runtime/cpamp-v1" || component.Status != "running" {
		t.Fatalf("CPAMP component after rollback = %+v", component)
	}
	pointerData, err := os.ReadFile(filepath.Join(componentDir, "cpamp", "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pointer map[string]string
	if err := json.Unmarshal(pointerData, &pointer); err != nil {
		t.Fatal(err)
	}
	if pointer["version"] != "v1.0.0" || pointer["binaryPath"] != "/runtime/cpamp-v1" {
		t.Fatalf("CPAMP current pointer after rollback = %+v", pointer)
	}
}

func TestRuntimeUpdateWaitsForReplacementHandoffConfirmation(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(health.Close)
	root := t.TempDir()
	previousBinary := filepath.Join(root, "cpamp-v1")
	targetBinary := filepath.Join(root, "cpamp-v2")
	for _, binary := range []string{previousBinary, targetBinary} {
		if err := os.WriteFile(binary, []byte("binary"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	state, err := OpenStateStore(
		filepath.Join(root, "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Status = "running"
		component.PID = 10
		component.Version = "v1.0.0"
		component.BinaryPath = previousBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}
	installer := &fakeRuntimeReleaseInstaller{
		manifest: ReleaseManifest{Components: map[string]ReleaseComponent{
			"cpamp": {Version: "v2.0.0"},
		}},
		results: map[string]InstalledComponent{
			"cpamp": {Name: "cpamp", Version: "v2.0.0", BinaryPath: targetBinary},
		},
		errors: map[string]error{},
	}
	handoff := make(chan runtimeHandoff, 1)
	controller := &RuntimeController{
		ctx: context.Background(),
		cfg: Config{
			ManagerURL:   health.URL,
			ComponentDir: filepath.Join(root, "components"),
		},
		state:      state,
		supervisor: &fakeRuntimeProcessSupervisor{state: state, nextPID: 100},
		installer:  installer,
		handoff:    handoff,
	}
	started, err := controller.StartUpdate(context.Background(), "cpamp")
	if err != nil {
		t.Fatal(err)
	}

	var request runtimeHandoff
	select {
	case request = <-handoff:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime handoff was not requested")
	}
	if request.OperationID != started.Operation.ID || request.Binary != targetBinary {
		t.Fatalf("runtime handoff request = %+v", request)
	}
	operation, ok := state.Operation(started.Operation.ID)
	if !ok || operation.Status != runtimeUpdateStatusHandoffPending || operation.Progress != 95 ||
		operation.CompletedAtMS != 0 {
		t.Fatalf("operation before replacement confirmation = %+v, ok=%v", operation, ok)
	}
	if _, err := controller.StartUpdate(context.Background(), "cpamp"); err == nil ||
		!strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("second update while handoff is pending error = %v", err)
	}
	confirmed, err := state.CompletePendingHandoff(operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != runtimeUpdateStatusSucceeded || confirmed.CompletedAtMS == 0 {
		t.Fatalf("operation after replacement confirmation = %+v", confirmed)
	}
}

func TestCPAOnlyRuntimeUpdateCompletesWithoutRuntimeHandoff(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(health.Close)
	root := t.TempDir()
	previousBinary := filepath.Join(root, "cpa-v1")
	targetBinary := filepath.Join(root, "cpa-v2")
	for _, binary := range []string{previousBinary, targetBinary} {
		if err := os.WriteFile(binary, []byte("binary"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	state, err := OpenStateStore(
		filepath.Join(root, "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpa", func(component ComponentState) ComponentState {
		component.Status = "running"
		component.PID = 20
		component.Version = "v7.1.0"
		component.BinaryPath = previousBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}
	controller := &RuntimeController{
		ctx: context.Background(),
		cfg: Config{
			CPAURL:       health.URL,
			ComponentDir: filepath.Join(root, "components"),
		},
		state:      state,
		supervisor: &fakeRuntimeProcessSupervisor{state: state, nextPID: 200},
		installer: &fakeRuntimeReleaseInstaller{
			manifest: ReleaseManifest{Components: map[string]ReleaseComponent{
				"cpa": {Version: "v7.2.0"},
			}},
			results: map[string]InstalledComponent{
				"cpa": {Name: "cpa", Version: "v7.2.0", BinaryPath: targetBinary},
			},
			errors: map[string]error{},
		},
	}
	started, err := controller.StartUpdate(context.Background(), "cpa")
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var operation UpdateOperation
	for time.Now().Before(deadline) {
		operation, _ = state.Operation(started.Operation.ID)
		if operation.Status == runtimeUpdateStatusSucceeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if operation.Status != runtimeUpdateStatusSucceeded || operation.CompletedAtMS == 0 {
		t.Fatalf("CPA-only update operation = %+v", operation)
	}
}
