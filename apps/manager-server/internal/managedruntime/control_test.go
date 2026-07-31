package managedruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/gateway"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
)

func TestSlimExistingHTTPFlowUpdatesTheGatewayAndRuntimeState(t *testing.T) {
	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "manager:"+r.URL.Path)
	}))
	t.Cleanup(manager.Close)
	initialCPA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "initial:"+r.URL.Path)
	}))
	t.Cleanup(initialCPA.Close)
	selectedCPA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "selected:"+r.URL.Path)
	}))
	t.Cleanup(selectedCPA.Close)
	runtimeTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "runtime:"+r.URL.Path)
	}))
	t.Cleanup(runtimeTarget.Close)

	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
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
	controller := &RuntimeController{ctx: context.Background(), state: state, router: router}
	handler := NewControlHandler(controller, "runtime-key")

	body, err := json.Marshal(map[string]string{"cpaUpstreamUrl": selectedCPA.URL})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "/v0/runtime/slim/existing", bytes.NewReader(body))
	request.Header.Set("X-CPAMP-Runtime-Key", "runtime-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := state.Snapshot().CPAUpstreamURL; got != selectedCPA.URL {
		t.Fatalf("CPA upstream = %q", got)
	}

	routed := httptest.NewRecorder()
	router.ServeHTTP(routed, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if routed.Code != http.StatusOK || routed.Body.String() != "selected:/v1/chat/completions" {
		t.Fatalf("routed response = %d %q", routed.Code, routed.Body.String())
	}
	commitRequest := httptest.NewRequest(http.MethodPost, "/v0/runtime/slim/commit", nil)
	commitRequest.Header.Set("X-CPAMP-Runtime-Key", "runtime-key")
	commitResponse := httptest.NewRecorder()
	handler.ServeHTTP(commitResponse, commitRequest)
	if commitResponse.Code != http.StatusOK {
		t.Fatalf("commit status = %d, body = %s", commitResponse.Code, commitResponse.Body.String())
	}
	pending := state.Snapshot().SlimTransition
	if pending != nil {
		t.Fatal("Slim transition remained pending after commit")
	}
}

func TestSlimExistingRevertHTTPFlowRestoresPreviousRuntimeState(t *testing.T) {
	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "manager:"+r.URL.Path)
	}))
	t.Cleanup(manager.Close)
	initialCPA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "initial:"+r.URL.Path)
	}))
	t.Cleanup(initialCPA.Close)
	selectedCPA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "selected:"+r.URL.Path)
	}))
	t.Cleanup(selectedCPA.Close)
	runtimeTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "runtime:"+r.URL.Path)
	}))
	t.Cleanup(runtimeTarget.Close)
	state, err := OpenStateStore(
		filepath.Join(t.TempDir(), "state.json"),
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
	controller := &RuntimeController{ctx: context.Background(), state: state, router: router}
	if _, err := controller.UseExistingCPA(selectedCPA.URL); err != nil {
		t.Fatal(err)
	}
	handler := NewControlHandler(controller, "runtime-key")
	request := httptest.NewRequest(http.MethodPost, "/v0/runtime/slim/revert", nil)
	request.Header.Set("X-CPAMP-Runtime-Key", "runtime-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	deployment := state.Snapshot().Deployment
	if deployment.Mode != model.DeploymentModeSlim || !deployment.RuntimeManaged ||
		deployment.CPAMPUpdatesManaged || deployment.CPAUpdatesManaged {
		t.Fatalf("deployment after revert = %+v", deployment)
	}
	if got := state.Snapshot().CPAUpstreamURL; got != initialCPA.URL {
		t.Fatalf("CPA upstream after revert = %q, want %q", got, initialCPA.URL)
	}
	routed := httptest.NewRecorder()
	router.ServeHTTP(routed, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if routed.Code != http.StatusOK || routed.Body.String() != "initial:/v1/chat/completions" {
		t.Fatalf("routed response after revert = %d %q", routed.Code, routed.Body.String())
	}
}
