package runtimecontrol

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/managedruntime"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/problem"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/store"
)

func TestProvisionSlimUsesLongRuntimeRequestTimeout(t *testing.T) {
	deployment := model.DefaultDeploymentState(
		model.DeploymentModeSlim,
		"/panel",
		model.PanelBasePathSourceRuntime,
	)
	integrated := deployment
	integrated.Mode = model.DeploymentModeIntegrated
	integrated.CPAMPUpdatesManaged = true
	integrated.CPAUpdatesManaged = true

	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v0/runtime/slim/provision" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(40 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(managedruntime.SlimProvisionResult{
			OK:             true,
			CPAUpstreamURL: "http://127.0.0.1:8317",
			Deployment:     integrated,
		})
	}))
	t.Cleanup(runtimeServer.Close)

	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveDeploymentState(t.Context(), deployment); err != nil {
		t.Fatal(err)
	}
	service := New(config.Config{
		DeploymentMode:    string(model.DeploymentModeSlim),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
	}, st)
	service.requestTimeout = 5 * time.Millisecond
	service.longRequestTimeout = 250 * time.Millisecond

	result, err := service.ProvisionSlim(t.Context())
	if err != nil {
		t.Fatalf("ProvisionSlim() error = %v", err)
	}
	if result.Deployment.Mode != model.DeploymentModeIntegrated {
		t.Fatalf("deployment mode = %q", result.Deployment.Mode)
	}
	result, err = service.ProvisionSlim(t.Context())
	if err != nil {
		t.Fatalf("idempotent ProvisionSlim() error = %v", err)
	}
	if result.Deployment.Mode != model.DeploymentModeIntegrated {
		t.Fatalf("idempotent deployment mode = %q", result.Deployment.Mode)
	}
}

func TestUseExistingCPANormalizesRuntimeSelection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "adds default scheme",
			input: "existing-cpa.example:8317",
			want:  "http://existing-cpa.example:8317",
		},
		{
			name:  "removes management API suffix",
			input: "https://existing-cpa.example:8317/v0/management/",
			want:  "https://existing-cpa.example:8317",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deployment := model.DefaultDeploymentState(
				model.DeploymentModeSlim,
				"/panel",
				model.PanelBasePathSourceRuntime,
			)
			var received string
			runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != "/v0/runtime/slim/existing" {
					http.NotFound(w, r)
					return
				}
				var request struct {
					CPAUpstreamURL string `json:"cpaUpstreamUrl"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				received = request.CPAUpstreamURL
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(managedruntime.SlimProvisionResult{
					OK:             true,
					CPAUpstreamURL: request.CPAUpstreamURL,
					Deployment:     deployment,
				})
			}))
			t.Cleanup(runtimeServer.Close)

			st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			if err := st.SaveDeploymentState(t.Context(), deployment); err != nil {
				t.Fatal(err)
			}
			service := New(config.Config{
				DeploymentMode:    string(model.DeploymentModeSlim),
				RuntimeControlURL: runtimeServer.URL,
				RuntimeKey:        "runtime-key",
			}, st)

			result, err := service.UseExistingCPA(t.Context(), test.input)
			if err != nil {
				t.Fatal(err)
			}
			if received != test.want {
				t.Fatalf("runtime CPA upstream = %q, want %q", received, test.want)
			}
			if result.CPAUpstreamURL != test.want {
				t.Fatalf("result CPA upstream = %q, want %q", result.CPAUpstreamURL, test.want)
			}
		})
	}
}

func TestUseExistingCPARejectsNonSlimDeploymentWithoutRuntimeMutation(t *testing.T) {
	for _, mode := range []model.DeploymentMode{
		model.DeploymentModeExternal,
		model.DeploymentModeInstallerManaged,
		model.DeploymentModeIntegrated,
	} {
		t.Run(string(mode), func(t *testing.T) {
			var runtimeCalls atomic.Int32
			runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				runtimeCalls.Add(1)
				http.Error(w, "unexpected runtime request", http.StatusInternalServerError)
			}))
			t.Cleanup(runtimeServer.Close)

			st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			deployment := model.DefaultDeploymentState(mode, "/panel", model.PanelBasePathSourceRuntime)
			if err := st.SaveDeploymentState(t.Context(), deployment); err != nil {
				t.Fatal(err)
			}
			service := New(config.Config{
				DeploymentMode:    string(mode),
				RuntimeControlURL: runtimeServer.URL,
				RuntimeKey:        "runtime-key",
			}, st)

			_, err = service.UseExistingCPA(t.Context(), "http://existing-cpa:8317")
			typed, ok := problem.As(err)
			if !ok || typed.Code != "slim_cpa_provision_unavailable" || typed.Status != http.StatusConflict {
				t.Fatalf("UseExistingCPA() error = %#v", typed)
			}
			if runtimeCalls.Load() != 0 {
				t.Fatalf("runtime calls = %d, want 0", runtimeCalls.Load())
			}
		})
	}
}

func TestUseExistingCPARollsBackRuntimeWhenManagerPersistenceFails(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	deployment := model.DefaultDeploymentState(
		model.DeploymentModeSlim,
		"/panel",
		model.PanelBasePathSourceRuntime,
	)
	if err := st.SaveDeploymentState(t.Context(), deployment); err != nil {
		t.Fatal(err)
	}
	var closeOnce sync.Once
	var revertCalls atomic.Int32
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v0/runtime/slim/existing":
			closeOnce.Do(func() { _ = st.Close() })
			_ = json.NewEncoder(w).Encode(managedruntime.SlimProvisionResult{
				OK:             true,
				CPAUpstreamURL: "http://existing-cpa:8317",
				Deployment:     deployment,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/revert":
			revertCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"deployment": deployment})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(runtimeServer.Close)

	service := New(config.Config{
		DeploymentMode:    string(model.DeploymentModeSlim),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
	}, st)
	_, err = service.UseExistingCPA(t.Context(), "http://existing-cpa:8317")
	if err == nil {
		t.Fatal("expected manager persistence failure")
	}
	if !strings.Contains(err.Error(), "restore previous Slim runtime") {
		t.Fatalf("error did not report rollback failure: %v", err)
	}
	if revertCalls.Load() != 1 {
		t.Fatalf("runtime revert calls = %d, want 1", revertCalls.Load())
	}
}

func TestReconcileSlimProvisionCommitsPersistedSetup(t *testing.T) {
	const managementKey = "management-key"
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
	slim.MigrationVersion = 7
	slim.MigrationCheckpoints = []string{"legacy-native", "runtime-v1"}
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
	var revertCalls atomic.Int32
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-CPAMP-Runtime-Key") != "runtime-key" {
			http.Error(w, "invalid runtime key", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/runtime/status":
			_ = json.NewEncoder(w).Encode(pending)
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/commit":
			commitCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/revert":
			revertCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"deployment": slim})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(runtimeServer.Close)

	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// A restarted Manager is initially bootstrapped with the transition's
	// previous Slim mode. Reconciliation must restore the runtime target mode
	// before clearing the transition.
	if err := st.SaveDeploymentState(t.Context(), slim); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSetup(t.Context(), store.Setup{
		CPAUpstreamURL: cpaServer.URL + "/v0/management/",
		ManagementKey:  managementKey,
	}); err != nil {
		t.Fatal(err)
	}
	service := New(config.Config{
		DeploymentMode:    string(model.DeploymentModeIntegrated),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
	}, st)

	recovered, err := service.ReconcileSlimProvision(t.Context())
	if err != nil {
		t.Fatalf("ReconcileSlimProvision() error = %v", err)
	}
	if !recovered {
		t.Fatal("expected pending Slim transition to be reconciled")
	}
	if commitCalls.Load() != 1 || revertCalls.Load() != 0 {
		t.Fatalf("commit calls = %d, revert calls = %d", commitCalls.Load(), revertCalls.Load())
	}
	persisted, ok, err := st.LoadDeploymentState(t.Context())
	if err != nil || !ok || persisted.Mode != model.DeploymentModeIntegrated ||
		persisted.MigrationVersion != slim.MigrationVersion ||
		!slices.Equal(persisted.MigrationCheckpoints, slim.MigrationCheckpoints) {
		t.Fatalf("persisted deployment after recovery = %+v, ok=%v, err=%v", persisted, ok, err)
	}
}

func TestReconcileSlimProvisionRevertsWhenSetupWasNotPersisted(t *testing.T) {
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
		CPAUpstreamURL: "http://127.0.0.1:8317",
		SlimTransition: &managedruntime.SlimTransition{
			Kind:                   "provision",
			PreviousDeployment:     slim,
			PreviousCPAUpstreamURL: "http://old-cpa:8317",
			TargetCPAUpstreamURL:   "http://127.0.0.1:8317",
		},
	}
	var commitCalls atomic.Int32
	var revertCalls atomic.Int32
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/runtime/status":
			_ = json.NewEncoder(w).Encode(pending)
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/commit":
			commitCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/revert":
			revertCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"deployment": slim})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(runtimeServer.Close)

	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveDeploymentState(t.Context(), integrated); err != nil {
		t.Fatal(err)
	}
	service := New(config.Config{
		DeploymentMode:    string(model.DeploymentModeIntegrated),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
	}, st)

	recovered, err := service.ReconcileSlimProvision(t.Context())
	if err != nil {
		t.Fatalf("ReconcileSlimProvision() error = %v", err)
	}
	if !recovered {
		t.Fatal("expected pending Slim transition to be reconciled")
	}
	if commitCalls.Load() != 0 || revertCalls.Load() != 1 {
		t.Fatalf("commit calls = %d, revert calls = %d", commitCalls.Load(), revertCalls.Load())
	}
	persisted, ok, err := st.LoadDeploymentState(t.Context())
	if err != nil || !ok || persisted.Mode != model.DeploymentModeSlim {
		t.Fatalf("persisted deployment after recovery = %+v, ok=%v, err=%v", persisted, ok, err)
	}
}

func TestReconcileSlimProvisionKeepsPendingTransitionWhenPersistedCPAIsUnavailable(t *testing.T) {
	cpaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(cpaServer.Close)

	integrated := model.DefaultDeploymentState(
		model.DeploymentModeIntegrated,
		"/panel",
		model.PanelBasePathSourceRuntime,
	)
	pending := managedruntime.State{
		Deployment:     integrated,
		CPAUpstreamURL: cpaServer.URL,
		SlimTransition: &managedruntime.SlimTransition{
			Kind:                 "provision",
			PreviousDeployment:   model.DefaultDeploymentState(model.DeploymentModeSlim, "/panel", model.PanelBasePathSourceRuntime),
			TargetCPAUpstreamURL: cpaServer.URL,
		},
	}
	var commitCalls atomic.Int32
	var revertCalls atomic.Int32
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/runtime/status":
			_ = json.NewEncoder(w).Encode(pending)
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/commit":
			commitCalls.Add(1)
		case r.Method == http.MethodPost && r.URL.Path == "/v0/runtime/slim/revert":
			revertCalls.Add(1)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(runtimeServer.Close)

	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveDeploymentState(t.Context(), integrated); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSetup(t.Context(), store.Setup{
		CPAUpstreamURL: cpaServer.URL,
		ManagementKey:  "management-key",
	}); err != nil {
		t.Fatal(err)
	}
	service := New(config.Config{
		DeploymentMode:    string(model.DeploymentModeIntegrated),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
	}, st)

	recovered, err := service.ReconcileSlimProvision(t.Context())
	if !recovered {
		t.Fatal("expected pending Slim transition to be detected")
	}
	typed, ok := problem.As(err)
	if !ok || typed.Code != "slim_transition_recovery_pending" {
		t.Fatalf("reconciliation error = %#v", err)
	}
	if commitCalls.Load() != 0 || revertCalls.Load() != 0 {
		t.Fatalf("commit calls = %d, revert calls = %d", commitCalls.Load(), revertCalls.Load())
	}
}

func TestReconcileSlimProvisionRepairsDeploymentAfterLostRevertResponse(t *testing.T) {
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
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v0/runtime/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(managedruntime.State{Deployment: slim})
	}))
	t.Cleanup(runtimeServer.Close)

	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveDeploymentState(t.Context(), integrated); err != nil {
		t.Fatal(err)
	}
	service := New(config.Config{
		DeploymentMode:    string(model.DeploymentModeIntegrated),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
	}, st)

	recovered, err := service.ReconcileSlimProvision(t.Context())
	if err != nil {
		t.Fatalf("ReconcileSlimProvision() error = %v", err)
	}
	if !recovered {
		t.Fatal("expected runtime deployment mismatch to be reconciled")
	}
	persisted, ok, err := st.LoadDeploymentState(t.Context())
	if err != nil || !ok || persisted.Mode != model.DeploymentModeSlim {
		t.Fatalf("persisted deployment after recovery = %+v, ok=%v, err=%v", persisted, ok, err)
	}
}

func TestReconcileSlimProvisionRepairsDynamicBasePathAfterLostManagerResponse(t *testing.T) {
	managerDeployment := model.DefaultDeploymentState(
		model.DeploymentModeIntegrated,
		"/panel",
		model.PanelBasePathSourceRuntime,
	)
	runtimeDeployment := model.DefaultDeploymentState(
		model.DeploymentModeIntegrated,
		"/admin",
		model.PanelBasePathSourceRuntime,
	)
	runtimeDeployment.RetiredPanelPaths = []string{"/management.html", "/panel"}
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v0/runtime/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(managedruntime.State{Deployment: runtimeDeployment})
	}))
	t.Cleanup(runtimeServer.Close)

	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveDeploymentState(t.Context(), managerDeployment); err != nil {
		t.Fatal(err)
	}
	service := New(config.Config{
		DeploymentMode:    string(model.DeploymentModeIntegrated),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
	}, st)

	recovered, err := service.ReconcileSlimProvision(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !recovered {
		t.Fatal("expected dynamic Base Path mismatch to be reconciled")
	}
	persisted, ok, err := st.LoadDeploymentState(t.Context())
	if err != nil || !ok {
		t.Fatalf("load reconciled deployment ok=%v err=%v", ok, err)
	}
	if persisted.PanelBasePath != "/admin" ||
		!slices.Equal(persisted.RetiredPanelPaths, runtimeDeployment.RetiredPanelPaths) {
		t.Fatalf("reconciled dynamic Base Path deployment = %+v", persisted)
	}
}

func TestMergeRuntimeDeploymentPreservesManagerMigrationMetadata(t *testing.T) {
	runtimeDeployment := model.DefaultDeploymentState(
		model.DeploymentModeIntegrated,
		"/admin",
		model.PanelBasePathSourceRuntime,
	)
	runtimeDeployment.RetiredPanelPaths = []string{"/management.html"}
	managerDeployment := model.DefaultDeploymentState(
		model.DeploymentModeSlim,
		"/panel",
		model.PanelBasePathSourceRuntime,
	)
	managerDeployment.MigrationVersion = 9
	managerDeployment.MigrationCheckpoints = []string{"legacy-docker", "runtime-v2"}

	merged := mergeRuntimeDeployment(runtimeDeployment, managerDeployment)
	if merged.Mode != model.DeploymentModeIntegrated || merged.PanelBasePath != "/admin" ||
		!slices.Equal(merged.RetiredPanelPaths, runtimeDeployment.RetiredPanelPaths) {
		t.Fatalf("runtime-owned deployment fields were not preserved: %+v", merged)
	}
	if merged.MigrationVersion != managerDeployment.MigrationVersion ||
		!slices.Equal(merged.MigrationCheckpoints, managerDeployment.MigrationCheckpoints) {
		t.Fatalf("manager migration metadata was not preserved: %+v", merged)
	}
	merged.MigrationCheckpoints[0] = "mutated"
	if managerDeployment.MigrationCheckpoints[0] != "legacy-docker" {
		t.Fatal("merged migration checkpoints alias manager deployment state")
	}
}

func TestUpdatePanelBasePathUsesRuntimeForInstallerManagedDeployment(t *testing.T) {
	deployment := model.DefaultDeploymentState(
		model.DeploymentModeInstallerManaged,
		"/panel",
		model.PanelBasePathSourceRuntime,
	)
	var updateCalls atomic.Int32
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v0/runtime/panel-base-path" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-CPAMP-Runtime-Key") != "runtime-key" {
			http.Error(w, "missing runtime key", http.StatusUnauthorized)
			return
		}
		var request struct {
			BasePath string `json:"basePath"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updateCalls.Add(1)
		deployment.PanelBasePath = request.BasePath
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"deployment": deployment})
	}))
	t.Cleanup(runtimeServer.Close)

	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveDeploymentState(t.Context(), deployment); err != nil {
		t.Fatal(err)
	}
	service := New(config.Config{
		DeploymentMode:    string(model.DeploymentModeInstallerManaged),
		RuntimeControlURL: runtimeServer.URL,
		RuntimeKey:        "runtime-key",
	}, st)

	result, err := service.UpdatePanelBasePath(t.Context(), "/admin")
	if err != nil {
		t.Fatal(err)
	}
	if updateCalls.Load() != 1 {
		t.Fatalf("runtime panel-base-path calls = %d, want 1", updateCalls.Load())
	}
	if result.PanelPath != "/admin" || !result.Deployment.RuntimeManaged {
		t.Fatalf("panel base path result = %+v", result)
	}
	persisted, ok, err := st.LoadDeploymentState(t.Context())
	if err != nil || !ok {
		t.Fatalf("load deployment state ok=%v err=%v", ok, err)
	}
	if persisted.PanelBasePath != "/admin" || !persisted.RuntimeManaged {
		t.Fatalf("persisted deployment = %+v", persisted)
	}
}
