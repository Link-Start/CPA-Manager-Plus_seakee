package managedruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
)

type fakeRuntimeProcessWaiter struct {
	called bool
	err    error
}

func (f *fakeRuntimeProcessWaiter) WaitProcesses(context.Context) error {
	f.called = true
	return f.err
}

func TestStopRuntimeWaitsForListenersAndProcesses(t *testing.T) {
	runCtx, cancel := context.WithCancel(context.Background())
	listeners := make(chan error, 2)
	listeners <- nil
	listeners <- errors.New("control listener failed")
	waiter := &fakeRuntimeProcessWaiter{err: errors.New("manager process did not stop")}

	err := stopRuntime(runCtx, cancel, waiter, listeners, 2)
	if runCtx.Err() == nil {
		t.Fatal("runtime context was not cancelled")
	}
	if !waiter.called {
		t.Fatal("runtime processes were not awaited")
	}
	if err == nil || !strings.Contains(err.Error(), "control listener failed") ||
		!strings.Contains(err.Error(), "manager process did not stop") {
		t.Fatalf("shutdown error = %v", err)
	}
}

func TestCurrentRuntimeBinaryRecoversInterruptedUpdateBeforeDelegation(t *testing.T) {
	t.Setenv("CPA_MANAGER_RUNTIME_DELEGATED", "")
	root := t.TempDir()
	previousBinary := filepath.Join(root, "cpamp-v1")
	targetBinary := filepath.Join(root, "cpamp-v2")
	for path, content := range map[string]string{
		previousBinary: "previous",
		targetBinary:   "target",
	} {
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Config{
		StatePath:      filepath.Join(root, "state.json"),
		ComponentDir:   filepath.Join(root, "components"),
		ManagerBinary:  previousBinary,
		DeploymentMode: model.DeploymentModeIntegrated,
		PanelBasePath:  "/panel",
	}
	store, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Version = "v2.0.0"
		component.BinaryPath = targetBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOperation(UpdateOperation{
		ID:              "update_crashed_before_handoff",
		Kind:            "cpamp",
		Status:          "running",
		Components:      []string{"cpamp"},
		CurrentVersions: map[string]string{"cpamp": "v1.0.0"},
		CurrentBinaries: map[string]string{"cpamp": previousBinary},
		TargetVersions:  map[string]string{"cpamp": "v2.0.0"},
	}); err != nil {
		t.Fatal(err)
	}

	binary, err := CurrentRuntimeBinary(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if binary != "" {
		t.Fatalf("replacement binary = %q, want original runtime", binary)
	}
	pointerData, err := os.ReadFile(filepath.Join(cfg.ComponentDir, "cpamp", "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pointer map[string]string
	if err := json.Unmarshal(pointerData, &pointer); err != nil {
		t.Fatal(err)
	}
	if pointer["version"] != "v1.0.0" || pointer["binaryPath"] != previousBinary {
		t.Fatalf("recovered current pointer = %+v", pointer)
	}
}

func TestCurrentRuntimeTargetPreservesPendingHandoffForDelegation(t *testing.T) {
	t.Setenv("CPA_MANAGER_RUNTIME_DELEGATED", "")
	root := t.TempDir()
	previousBinary := filepath.Join(root, "cpamp-v1")
	targetBinary := filepath.Join(root, "cpamp-v2")
	for path, content := range map[string]string{
		previousBinary: "previous",
		targetBinary:   "target",
	} {
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Config{
		StatePath:      filepath.Join(root, "state.json"),
		ComponentDir:   filepath.Join(root, "components"),
		ManagerBinary:  previousBinary,
		DeploymentMode: model.DeploymentModeIntegrated,
		PanelBasePath:  "/panel",
	}
	store, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Version = "v2.0.0"
		component.BinaryPath = targetBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOperation(UpdateOperation{
		ID:              "update_pending_handoff",
		Kind:            "cpamp",
		Status:          runtimeUpdateStatusHandoffPending,
		Components:      []string{"cpamp"},
		CurrentVersions: map[string]string{"cpamp": "v1.0.0"},
		CurrentBinaries: map[string]string{"cpamp": previousBinary},
		TargetVersions:  map[string]string{"cpamp": "v2.0.0"},
	}); err != nil {
		t.Fatal(err)
	}

	target, err := CurrentRuntimeTarget(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if target.Binary != targetBinary || target.OperationID != "update_pending_handoff" {
		t.Fatalf("runtime target = %+v", target)
	}
	operation, ok := store.Operation("update_pending_handoff")
	if !ok || operation.Status != runtimeUpdateStatusHandoffPending {
		t.Fatalf("pending handoff changed before delegation = %+v, ok=%v", operation, ok)
	}
	assertCurrentComponentPointer(t, cfg.ComponentDir, "cpamp", "v2.0.0", targetBinary)
}

func TestCurrentRuntimeTargetRollsBackMissingPendingHandoffBinary(t *testing.T) {
	t.Setenv("CPA_MANAGER_RUNTIME_DELEGATED", "")
	root := t.TempDir()
	previousBinary := filepath.Join(root, "cpamp-v1")
	if err := os.WriteFile(previousBinary, []byte("previous"), 0o700); err != nil {
		t.Fatal(err)
	}
	targetBinary := filepath.Join(root, "missing-cpamp-v2")
	cfg := Config{
		StatePath:      filepath.Join(root, "state.json"),
		ComponentDir:   filepath.Join(root, "components"),
		ManagerBinary:  previousBinary,
		DeploymentMode: model.DeploymentModeIntegrated,
		PanelBasePath:  "/panel",
	}
	store, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Version = "v2.0.0"
		component.BinaryPath = targetBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOperation(UpdateOperation{
		ID:              "update_missing_handoff_binary",
		Kind:            "cpamp",
		Status:          runtimeUpdateStatusHandoffPending,
		Components:      []string{"cpamp"},
		CurrentVersions: map[string]string{"cpamp": "v1.0.0"},
		CurrentBinaries: map[string]string{"cpamp": previousBinary},
		TargetVersions:  map[string]string{"cpamp": "v2.0.0"},
	}); err != nil {
		t.Fatal(err)
	}

	target, err := CurrentRuntimeTarget(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if target != (RuntimeTarget{}) {
		t.Fatalf("runtime target after recovery = %+v", target)
	}
	recoveredStore, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := recoveredStore.Operation("update_missing_handoff_binary")
	if !ok || operation.Status != runtimeUpdateStatusRolledBack || !operation.RollbackSuccessful {
		t.Fatalf("recovered handoff operation = %+v, ok=%v", operation, ok)
	}
	component := recoveredStore.Snapshot().Components["cpamp"]
	if component.Version != "v1.0.0" || component.BinaryPath != previousBinary {
		t.Fatalf("recovered CPAMP component = %+v", component)
	}
	assertCurrentComponentPointer(t, cfg.ComponentDir, "cpamp", "v1.0.0", previousBinary)
	fallback, err := HandoffFallbackTarget(cfg, "update_missing_handoff_binary")
	if err != nil {
		t.Fatal(err)
	}
	if fallback.Binary != previousBinary || fallback.OperationID != "" {
		t.Fatalf("handoff fallback target = %+v", fallback)
	}
}

func TestResolvePendingRuntimeHandoffIgnoresStaleOperationMarker(t *testing.T) {
	root := t.TempDir()
	activeBinary := filepath.Join(root, "cpamp-v3")
	if err := os.WriteFile(activeBinary, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot := State{
		Components: map[string]ComponentState{
			"cpamp": {Name: "cpamp", Version: "v3.0.0", BinaryPath: activeBinary},
		},
		Operations: map[string]UpdateOperation{
			"update_previous": {
				ID:         "update_previous",
				Status:     runtimeUpdateStatusSucceeded,
				Components: []string{"cpamp"},
			},
			"update_current": {
				ID:             "update_current",
				Status:         runtimeUpdateStatusHandoffPending,
				Components:     []string{"cpamp"},
				TargetVersions: map[string]string{"cpamp": "v3.0.0"},
			},
		},
	}

	operation, pending, err := resolvePendingRuntimeHandoff(
		snapshot,
		"update_previous",
		activeBinary,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !pending || operation.ID != "update_current" {
		t.Fatalf("resolved pending handoff = %+v, pending=%v", operation, pending)
	}
}

func TestCurrentRuntimeBinaryPrefersNewerPackagedCPAMP(t *testing.T) {
	t.Setenv("CPA_MANAGER_RUNTIME_DELEGATED", "")
	packagedBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	managedBinary := filepath.Join(root, "managed-cpamp-v1")
	if err := os.WriteFile(managedBinary, []byte("managed"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		StatePath:      filepath.Join(root, "state.json"),
		ComponentDir:   filepath.Join(root, "components"),
		ManagerBinary:  packagedBinary,
		DeploymentMode: model.DeploymentModeIntegrated,
		PanelBasePath:  "/panel",
		CPAMPVersion:   "v2.0.0",
	}
	state, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Version = "v1.0.0"
		component.BinaryPath = managedBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}

	binary, err := CurrentRuntimeBinary(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if binary != "" {
		t.Fatalf("replacement binary = %q, want packaged runtime", binary)
	}
	persisted, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	component := persisted.Snapshot().Components["cpamp"]
	if component.Version != "v2.0.0" || component.BinaryPath != packagedBinary {
		t.Fatalf("reconciled CPAMP component = %+v", component)
	}
	assertCurrentComponentPointer(t, cfg.ComponentDir, "cpamp", "v2.0.0", packagedBinary)
}

func TestCurrentRuntimeBinaryKeepsNewerManagedCPAMP(t *testing.T) {
	t.Setenv("CPA_MANAGER_RUNTIME_DELEGATED", "")
	packagedBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	managedBinary := filepath.Join(root, "managed-cpamp-v2")
	if err := os.WriteFile(managedBinary, []byte("managed"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		StatePath:      filepath.Join(root, "state.json"),
		ComponentDir:   filepath.Join(root, "components"),
		ManagerBinary:  packagedBinary,
		DeploymentMode: model.DeploymentModeIntegrated,
		PanelBasePath:  "/panel",
		CPAMPVersion:   "v1.0.0",
	}
	state, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Version = "v2.0.0"
		component.BinaryPath = managedBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}

	binary, err := CurrentRuntimeBinary(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if binary != managedBinary {
		t.Fatalf("replacement binary = %q, want %q", binary, managedBinary)
	}
	persisted, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	component := persisted.Snapshot().Components["cpamp"]
	if component.Version != "v2.0.0" || component.BinaryPath != managedBinary {
		t.Fatalf("managed CPAMP component changed = %+v", component)
	}
	assertCurrentComponentPointer(t, cfg.ComponentDir, "cpamp", "v2.0.0", managedBinary)
}

func TestActivePackagedRuntimeBinaryAcceptsEquivalentConfiguredPath(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), filepath.Base(executable))
	if err := os.Link(executable, alias); err != nil {
		t.Skipf("create executable hard link: %v", err)
	}

	activeBinary, active, err := activePackagedRuntimeBinary(alias)
	if err != nil {
		t.Fatal(err)
	}
	if !active || !runtimeBinaryPathsEqual(activeBinary, executable) {
		t.Fatalf("active packaged runtime = %q, active=%v, want %q", activeBinary, active, executable)
	}
}

func TestPackagedComponentsShareDirectoryAcceptsEquivalentDirectoryPaths(t *testing.T) {
	realDir := filepath.Join(t.TempDir(), "package")
	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(t.TempDir(), "package-link")
	if err := os.Symlink(realDir, aliasDir); err != nil {
		t.Skipf("create package directory symlink: %v", err)
	}
	managerBinary := filepath.Join(realDir, "cpa-manager-plus")
	cpaBinary := filepath.Join(aliasDir, "cli-proxy-api")
	for _, binary := range []string{managerBinary, filepath.Join(realDir, "cli-proxy-api")} {
		if err := os.WriteFile(binary, []byte("binary"), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	if !packagedComponentsShareDirectory(managerBinary, cpaBinary) {
		t.Fatalf("equivalent package directories were treated as different: %q and %q", managerBinary, cpaBinary)
	}
}

func TestPrepareRuntimeComponentsReconcilesPackagedCPAByVersion(t *testing.T) {
	packagedManager, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name               string
		packagedVersion    string
		managedVersion     string
		wantPackagedBinary bool
	}{
		{name: "packaged CPA is newer", packagedVersion: "v7.3.0", managedVersion: "v7.2.0", wantPackagedBinary: true},
		{name: "managed CPA is newer", packagedVersion: "v7.2.0", managedVersion: "v7.3.0"},
		{name: "development package cannot replace release", packagedVersion: "dev", managedVersion: "v7.3.0"},
		{name: "unknown package cannot replace release", packagedVersion: "unknown", managedVersion: "v7.3.0"},
		{name: "unparseable package cannot replace release", packagedVersion: "latest", managedVersion: "v7.3.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			packagedCPA := packagedManager
			managedCPA := filepath.Join(root, "managed-cpa")
			if err := os.WriteFile(managedCPA, []byte("binary"), 0o700); err != nil {
				t.Fatal(err)
			}
			cfg := Config{
				StatePath:      filepath.Join(root, "state.json"),
				ComponentDir:   filepath.Join(root, "components"),
				ManagerBinary:  packagedManager,
				CPABinary:      packagedCPA,
				CPAVersion:     test.packagedVersion,
				DeploymentMode: model.DeploymentModeIntegrated,
				PanelBasePath:  "/panel",
			}
			state, err := OpenStateStore(
				cfg.StatePath,
				model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := state.UpdateComponent("cpa", func(component ComponentState) ComponentState {
				component.Version = test.managedVersion
				component.BinaryPath = managedCPA
				return component
			}); err != nil {
				t.Fatal(err)
			}

			snapshot, err := prepareRuntimeComponents(cfg, state)
			if err != nil {
				t.Fatal(err)
			}
			wantVersion := test.managedVersion
			wantBinary := managedCPA
			if test.wantPackagedBinary {
				wantVersion = test.packagedVersion
				wantBinary = packagedCPA
			}
			component := snapshot.Components["cpa"]
			if component.Version != wantVersion || component.BinaryPath != wantBinary {
				t.Fatalf("reconciled CPA component = %+v, want version %q binary %q", component, wantVersion, wantBinary)
			}
			assertCurrentComponentPointer(t, cfg.ComponentDir, "cpa", wantVersion, wantBinary)
		})
	}
}

func TestReplacementRuntimeKeepsUpdatedCPAComponentFromState(t *testing.T) {
	managerBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	packagedCPA := filepath.Join(root, "package", "cli-proxy-api")
	updatedCPA := filepath.Join(root, "components", "cpa", "v7.3.0", "cli-proxy-api")
	for _, binary := range []string{packagedCPA, updatedCPA} {
		if err := os.MkdirAll(filepath.Dir(binary), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(binary, []byte("binary"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Config{
		StatePath:      filepath.Join(root, "state.json"),
		ComponentDir:   filepath.Join(root, "components"),
		ManagerBinary:  managerBinary,
		CPABinary:      packagedCPA,
		CPAVersion:     "v7.2.0",
		DeploymentMode: model.DeploymentModeIntegrated,
		PanelBasePath:  "/panel",
	}
	state, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpa", func(component ComponentState) ComponentState {
		component.Version = "v7.3.0"
		component.BinaryPath = updatedCPA
		return component
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := prepareRuntimeComponents(cfg, state)
	if err != nil {
		t.Fatal(err)
	}
	component := snapshot.Components["cpa"]
	if component.Version != "v7.3.0" || component.BinaryPath != updatedCPA {
		t.Fatalf("CPA component after replacement preparation = %+v", component)
	}
	spec := (&RuntimeController{cfg: cfg, state: state}).cpaSpec()
	if spec.Version != "v7.3.0" || spec.Binary != updatedCPA {
		t.Fatalf("CPA process spec after replacement preparation = %+v", spec)
	}
}

func TestReconcileInstallerManagedRuntimeConfigUpdatesPersistedConnection(t *testing.T) {
	root := t.TempDir()
	state, err := OpenStateStore(
		filepath.Join(root, "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeSlim, "/admin", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateCPAUpstream("http://old-cpa:8317"); err != nil {
		t.Fatal(err)
	}
	snapshot := state.Snapshot()
	snapshot, err = reconcileInstallerManagedRuntimeConfig(Config{
		DeploymentMode:    model.DeploymentModeInstallerManaged,
		CPAURL:            "http://new-cpa:8317",
		CPAUpstreamEnvSet: true,
	}, state, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CPAUpstreamURL != "http://new-cpa:8317" {
		t.Fatalf("reconciled CPA upstream = %q", snapshot.CPAUpstreamURL)
	}
	if snapshot.Deployment.Mode != model.DeploymentModeInstallerManaged ||
		!snapshot.Deployment.RuntimeManaged || snapshot.Deployment.CPAMPUpdatesManaged ||
		snapshot.Deployment.CPAUpdatesManaged || snapshot.Deployment.PanelBasePath != "/admin" {
		t.Fatalf("reconciled deployment = %+v", snapshot.Deployment)
	}
}

func TestReconcileInstallerManagedRuntimeConfigPreservesUIDerivedState(t *testing.T) {
	root := t.TempDir()
	state, err := OpenStateStore(
		filepath.Join(root, "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateCPAUpstream("http://managed-cpa:8317"); err != nil {
		t.Fatal(err)
	}
	snapshot := state.Snapshot()
	got, err := reconcileInstallerManagedRuntimeConfig(Config{
		DeploymentMode: model.DeploymentModeSlim,
		CPAURL:         "http://127.0.0.1:8317",
	}, state, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if got.Deployment.Mode != model.DeploymentModeIntegrated || got.CPAUpstreamURL != "http://managed-cpa:8317" {
		t.Fatalf("UI-derived runtime state was overwritten: %+v", got)
	}
}

func TestPrepareRuntimeComponentsDoesNotPromoteExternalCPAAsPackaged(t *testing.T) {
	packagedManager, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	externalCPA := filepath.Join(root, "external-cpa")
	managedCPA := filepath.Join(root, "managed-cpa")
	for _, binary := range []string{externalCPA, managedCPA} {
		if err := os.WriteFile(binary, []byte("binary"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Config{
		StatePath:      filepath.Join(root, "state.json"),
		ComponentDir:   filepath.Join(root, "components"),
		ManagerBinary:  packagedManager,
		CPABinary:      externalCPA,
		CPAVersion:     "v7.3.0",
		DeploymentMode: model.DeploymentModeIntegrated,
		PanelBasePath:  "/panel",
	}
	state, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpa", func(component ComponentState) ComponentState {
		component.Version = "v7.2.0"
		component.BinaryPath = managedCPA
		return component
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := prepareRuntimeComponents(cfg, state)
	if err != nil {
		t.Fatal(err)
	}
	component := snapshot.Components["cpa"]
	if component.Version != "v7.2.0" || component.BinaryPath != managedCPA {
		t.Fatalf("external CPA was treated as packaged = %+v", component)
	}
	assertCurrentComponentPointer(t, cfg.ComponentDir, "cpa", "v7.2.0", managedCPA)
}

func assertCurrentComponentPointer(
	t *testing.T,
	componentDir string,
	name string,
	wantVersion string,
	wantBinary string,
) {
	t.Helper()
	pointerData, err := os.ReadFile(filepath.Join(componentDir, name, "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pointer map[string]string
	if err := json.Unmarshal(pointerData, &pointer); err != nil {
		t.Fatal(err)
	}
	if pointer["version"] != wantVersion || pointer["binaryPath"] != wantBinary {
		t.Fatalf("%s current pointer = %+v, want version %q binary %q", name, pointer, wantVersion, wantBinary)
	}
}

func TestCurrentRuntimeBinaryRejectsUnsynchronizedComponentPointers(t *testing.T) {
	t.Setenv("CPA_MANAGER_RUNTIME_DELEGATED", "")
	root := t.TempDir()
	currentBinary := filepath.Join(root, "cpamp-v1")
	targetBinary := filepath.Join(root, "cpamp-v2")
	for _, binary := range []string{currentBinary, targetBinary} {
		if err := os.WriteFile(binary, []byte("binary"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	componentDir := filepath.Join(root, "components")
	if err := os.WriteFile(componentDir, []byte("not-a-directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		StatePath:      filepath.Join(root, "state.json"),
		ComponentDir:   componentDir,
		ManagerBinary:  currentBinary,
		DeploymentMode: model.DeploymentModeIntegrated,
		PanelBasePath:  "/panel",
	}
	state, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Version = "v2.0.0"
		component.BinaryPath = targetBinary
		return component
	}); err != nil {
		t.Fatal(err)
	}

	binary, err := CurrentRuntimeBinary(cfg)
	if err == nil || !strings.Contains(err.Error(), "sync runtime component pointers") {
		t.Fatalf("CurrentRuntimeBinary() error = %v", err)
	}
	if binary != "" {
		t.Fatalf("replacement binary = %q after pointer synchronization failure", binary)
	}
}

func TestCurrentRuntimeBinaryRejectsMissingReplacementBinary(t *testing.T) {
	t.Setenv("CPA_MANAGER_RUNTIME_DELEGATED", "")
	root := t.TempDir()
	currentBinary := filepath.Join(root, "cpamp-v1")
	if err := os.WriteFile(currentBinary, []byte("current"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		StatePath:      filepath.Join(root, "state.json"),
		ComponentDir:   filepath.Join(root, "components"),
		ManagerBinary:  currentBinary,
		DeploymentMode: model.DeploymentModeIntegrated,
		PanelBasePath:  "/panel",
	}
	state, err := OpenStateStore(
		cfg.StatePath,
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/panel", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateComponent("cpamp", func(component ComponentState) ComponentState {
		component.Version = "v2.0.0"
		component.BinaryPath = filepath.Join(root, "missing-cpamp-v2")
		return component
	}); err != nil {
		t.Fatal(err)
	}

	binary, err := CurrentRuntimeBinary(cfg)
	if err == nil || !strings.Contains(err.Error(), "inspect replacement runtime binary") {
		t.Fatalf("CurrentRuntimeBinary() error = %v", err)
	}
	if binary != "" {
		t.Fatalf("replacement binary = %q, want empty", binary)
	}
}

func TestReconcileConfiguredPanelBasePathAppliesAndReleasesEnvironmentOwnership(t *testing.T) {
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
		model.DefaultDeploymentState(model.DeploymentModeIntegrated, "/dynamic", model.PanelBasePathSourceRuntime),
	)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := reconcileConfiguredPanelBasePath(Config{
		PanelBasePath:       "/locked",
		PanelBasePathSource: model.PanelBasePathSourceEnvironment,
	}, state)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Deployment.PanelBasePath != "/locked" ||
		snapshot.Deployment.PanelBasePathSource != model.PanelBasePathSourceEnvironment {
		t.Fatalf("environment reconciliation = %+v", snapshot.Deployment)
	}
	if _, err := state.UpdatePanelBasePath("/rejected"); err == nil {
		t.Fatal("environment-managed Base Path accepted a runtime update")
	}

	snapshot, err = reconcileConfiguredPanelBasePath(Config{
		PanelBasePathSource: model.PanelBasePathSourceRuntime,
	}, state)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Deployment.PanelBasePath != "/locked" ||
		snapshot.Deployment.PanelBasePathSource != model.PanelBasePathSourceRuntime {
		t.Fatalf("environment release = %+v", snapshot.Deployment)
	}
	if _, err := state.UpdatePanelBasePath("/dynamic-again"); err != nil {
		t.Fatalf("runtime update after environment release: %v", err)
	}
	if _, err := state.UpdateDeployment(func(deployment model.DeploymentState) model.DeploymentState {
		deployment.PanelBasePathSource = ""
		return deployment
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = reconcileConfiguredPanelBasePath(Config{}, state)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Deployment.PanelBasePathSource != model.PanelBasePathSourceRuntime {
		t.Fatalf("legacy panel Base Path source was not normalized: %+v", snapshot.Deployment)
	}
}
