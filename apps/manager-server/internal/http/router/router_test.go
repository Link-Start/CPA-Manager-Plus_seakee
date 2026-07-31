package router

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/app"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	panelsvc "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/panel"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/store"
)

func TestPanelRouteUsesPersistedDynamicBasePath(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	deployment := model.DefaultDeploymentState(
		model.DeploymentModeIntegrated,
		"/admin",
		model.PanelBasePathSourceRuntime,
	)
	deployment.RetiredPanelPaths = []string{"/management.html", "/old-panel"}
	if err := st.SaveDeploymentState(t.Context(), deployment); err != nil {
		t.Fatal(err)
	}
	handler := New(&app.Context{
		Config: config.Config{
			DeploymentMode: string(model.DeploymentModeIntegrated),
			PanelBasePath:  "/management.html",
		},
		Store: st,
		PanelService: panelsvc.New("", fstest.MapFS{
			"web/management.html": &fstest.MapFile{Data: []byte("dynamic panel")},
		}),
	})

	for _, path := range []string{"/admin", "/admin/"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || recorder.Body.String() != "dynamic panel" {
			t.Fatalf("GET %s = %d %q", path, recorder.Code, recorder.Body.String())
		}
	}
	for _, path := range []string{"/", "/management.html", "/old-panel", "/old-panel/"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want %d", path, recorder.Code, http.StatusNotFound)
		}
	}
}

func TestPanelRouteFallsBackToConfiguredDefaultBeforeDeploymentStateExists(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	handler := New(&app.Context{
		Config: config.Config{
			DeploymentMode: string(model.DeploymentModeIntegrated),
			PanelBasePath:  "/management.html",
		},
		Store: st,
		PanelService: panelsvc.New("", fstest.MapFS{
			"web/management.html": &fstest.MapFile{Data: []byte("default panel")},
		}),
	})

	root := httptest.NewRecorder()
	handler.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/", nil))
	if root.Code != http.StatusTemporaryRedirect || root.Header().Get("Location") != "/management.html" {
		t.Fatalf("GET / = %d location %q", root.Code, root.Header().Get("Location"))
	}
	panel := httptest.NewRecorder()
	handler.ServeHTTP(panel, httptest.NewRequest(http.MethodGet, "/management.html", nil))
	if panel.Code != http.StatusOK || panel.Body.String() != "default panel" {
		t.Fatalf("GET /management.html = %d %q", panel.Code, panel.Body.String())
	}
}
