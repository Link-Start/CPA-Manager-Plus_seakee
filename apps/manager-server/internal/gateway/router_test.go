package gateway

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/protocol"
)

func TestListenReservesAllGatewayPortsBeforeServing(t *testing.T) {
	server, err := Listen([]string{"127.0.0.1:0", "127.0.0.1:0"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if len(server.listeners) != 2 {
		t.Fatalf("listeners = %d", len(server.listeners))
	}
	for _, listener := range server.listeners {
		connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatalf("reserved listener %s is not reachable: %v", listener.Addr(), err)
		}
		_ = connection.Close()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve after cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gateway server did not stop")
	}
}

func TestGatewayServesTheSameHandlerOnEveryConfiguredPort(t *testing.T) {
	server, err := Listen(
		[]string{"127.0.0.1:0", "127.0.0.1:0"},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "gateway:"+r.URL.Path)
		}),
	)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = server.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("gateway server did not stop")
		}
	})

	for _, listener := range server.listeners {
		response, err := http.Get("http://" + listener.Addr().String() + "/v1/models")
		if err != nil {
			t.Fatalf("request %s: %v", listener.Addr(), err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatalf("read %s: %v", listener.Addr(), readErr)
		}
		if response.StatusCode != http.StatusOK || string(body) != "gateway:/v1/models" {
			t.Fatalf("gateway %s = %d %q", listener.Addr(), response.StatusCode, body)
		}
	}
}

func TestRouterDispatchesManagerCPAAndRuntimePaths(t *testing.T) {
	manager := httptest.NewServer(upstreamHandler("manager"))
	t.Cleanup(manager.Close)
	cpa := httptest.NewServer(upstreamHandler("cpa"))
	t.Cleanup(cpa.Close)
	runtimeServer := httptest.NewServer(upstreamHandler("runtime"))
	t.Cleanup(runtimeServer.Close)

	router, err := New(manager.URL, cpa.URL, runtimeServer.URL, "/panel", []string{"/management.html", "/admin"})
	if err != nil {
		t.Fatalf("create router: %v", err)
	}

	assertRoute(t, router, "/panel", http.StatusOK, "manager:/panel")
	assertRoute(t, router, "/v0/management", http.StatusOK, "manager:/v0/management")
	assertRoute(t, router, "/v0/management/config", http.StatusOK, "manager:/v0/management/config")
	assertRoute(t, router, "/v1/chat/completions", http.StatusOK, "cpa:/v1/chat/completions")
	assertRoute(t, router, "/oauth-callback", http.StatusOK, "cpa:/oauth-callback")
	assertRoute(t, router, "/oauth/callback", http.StatusOK, "cpa:/oauth/callback")
	assertRoute(t, router, "/v0/runtime/operations/operation-1", http.StatusOK, "runtime:/v0/runtime/operations/operation-1")
	assertRoute(t, router, "/management.html", http.StatusNotFound, "404 page not found")
	assertRoute(t, router, "/admin", http.StatusNotFound, "404 page not found")
}

func TestRouterRejectsCPAOAuthCallbackAsPanelPath(t *testing.T) {
	manager := httptest.NewServer(upstreamHandler("manager"))
	t.Cleanup(manager.Close)
	cpa := httptest.NewServer(upstreamHandler("cpa"))
	t.Cleanup(cpa.Close)
	runtimeServer := httptest.NewServer(upstreamHandler("runtime"))
	t.Cleanup(runtimeServer.Close)

	router, err := New(manager.URL, cpa.URL, runtimeServer.URL, "/panel", nil)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	for _, panelPath := range []string{"/oauth-callback", "/oauth/callback"} {
		if err := router.UpdatePanelRoute(panelPath, nil); err == nil {
			t.Fatalf("OAuth callback %q was accepted as the panel path", panelPath)
		}
	}
	assertRoute(t, router, "/panel", http.StatusOK, "manager:/panel")
	assertRoute(t, router, "/oauth-callback", http.StatusOK, "cpa:/oauth-callback")
	assertRoute(t, router, "/oauth/callback", http.StatusOK, "cpa:/oauth/callback")
}

func TestRouterUpdatesCPAUpstreamAtomically(t *testing.T) {
	manager := httptest.NewServer(upstreamHandler("manager"))
	t.Cleanup(manager.Close)
	firstCPA := httptest.NewServer(upstreamHandler("cpa-first"))
	t.Cleanup(firstCPA.Close)
	secondCPA := httptest.NewServer(upstreamHandler("cpa-second"))
	t.Cleanup(secondCPA.Close)
	runtimeServer := httptest.NewServer(upstreamHandler("runtime"))
	t.Cleanup(runtimeServer.Close)

	router, err := New(manager.URL, firstCPA.URL, runtimeServer.URL, "/panel", nil)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	assertRoute(t, router, "/v1/chat/completions", http.StatusOK, "cpa-first:/v1/chat/completions")
	if err := router.UpdateCPATarget(secondCPA.URL); err != nil {
		t.Fatalf("update CPA target: %v", err)
	}
	assertRoute(t, router, "/v1/chat/completions", http.StatusOK, "cpa-second:/v1/chat/completions")
}

func TestRouterUpdatesPanelPathAtomically(t *testing.T) {
	manager := httptest.NewServer(upstreamHandler("manager"))
	t.Cleanup(manager.Close)
	cpa := httptest.NewServer(upstreamHandler("cpa"))
	t.Cleanup(cpa.Close)
	runtimeServer := httptest.NewServer(upstreamHandler("runtime"))
	t.Cleanup(runtimeServer.Close)

	router, err := New(manager.URL, cpa.URL, runtimeServer.URL, "/management.html", nil)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	assertRoute(t, router, "/", http.StatusTemporaryRedirect, "")
	if err := router.UpdatePanelRoute("/admin", []string{"/management.html"}); err != nil {
		t.Fatalf("update route: %v", err)
	}
	assertRoute(t, router, "/admin", http.StatusOK, "manager:/admin")
	assertRoute(t, router, "/management.html", http.StatusNotFound, "404 page not found")
	assertRoute(t, router, "/", http.StatusOK, "cpa:/")
}

func TestRouterRejectsDirectCPAValidationRequests(t *testing.T) {
	manager := httptest.NewServer(upstreamHandler("manager"))
	t.Cleanup(manager.Close)
	cpa := httptest.NewServer(upstreamHandler("cpa"))
	t.Cleanup(cpa.Close)
	runtimeServer := httptest.NewServer(upstreamHandler("runtime"))
	t.Cleanup(runtimeServer.Close)

	router, err := New(manager.URL, cpa.URL, runtimeServer.URL, "/panel", nil)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v0/management/config", nil)
	req.Header.Set(protocol.DirectCPARequestHeader, "1")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusMisdirectedRequest {
		t.Fatalf("direct CPA validation status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestRouterRejectsRecursiveGatewayHop(t *testing.T) {
	manager := httptest.NewServer(upstreamHandler("manager"))
	t.Cleanup(manager.Close)
	cpa := httptest.NewServer(upstreamHandler("cpa"))
	t.Cleanup(cpa.Close)
	runtimeServer := httptest.NewServer(upstreamHandler("runtime"))
	t.Cleanup(runtimeServer.Close)

	router, err := New(manager.URL, cpa.URL, runtimeServer.URL, "/panel", nil)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set(protocol.GatewayHopHeader, "1")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusLoopDetected {
		t.Fatalf("recursive gateway status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func upstreamHandler(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, name+":"+r.URL.Path)
	})
}

func assertRoute(t testing.TB, handler http.Handler, target string, status int, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != status {
		t.Fatalf("%s status = %d, body = %s", target, rr.Code, rr.Body.String())
	}
	if body != "" && !strings.Contains(rr.Body.String(), body) {
		t.Fatalf("%s body = %q, want %q", target, rr.Body.String(), body)
	}
}
