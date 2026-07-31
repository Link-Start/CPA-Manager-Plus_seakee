package gateway

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/protocol"
)

type RouteState struct {
	PanelBasePath     string
	RetiredPanelPaths map[string]struct{}
}

type Router struct {
	managerProxy *httputil.ReverseProxy
	runtimeProxy *httputil.ReverseProxy
	cpaProxy     atomic.Pointer[httputil.ReverseProxy]
	routes       atomic.Pointer[RouteState]
}

func New(managerURL string, cpaURL string, runtimeURL string, panelBasePath string, retiredPanelPaths []string) (*Router, error) {
	managerTarget, err := parseTarget(managerURL)
	if err != nil {
		return nil, err
	}
	cpaTarget, err := parseTarget(cpaURL)
	if err != nil {
		return nil, err
	}
	runtimeTarget, err := parseTarget(runtimeURL)
	if err != nil {
		return nil, err
	}
	router := &Router{
		managerProxy: newProxy(managerTarget),
		runtimeProxy: newProxy(runtimeTarget),
	}
	router.cpaProxy.Store(newProxy(cpaTarget))
	if err := router.UpdatePanelRoute(panelBasePath, retiredPanelPaths); err != nil {
		return nil, err
	}
	return router, nil
}

func (r *Router) UpdateCPATarget(cpaURL string) error {
	target, err := parseTarget(cpaURL)
	if err != nil {
		return err
	}
	r.cpaProxy.Store(newProxy(target))
	return nil
}

func (r *Router) UpdatePanelRoute(panelBasePath string, retiredPanelPaths []string) error {
	normalized, err := model.NormalizePanelBasePath(panelBasePath)
	if err != nil {
		return err
	}
	retired := make(map[string]struct{}, len(retiredPanelPaths)+1)
	for _, candidate := range retiredPanelPaths {
		candidate, err = model.NormalizePanelBasePath(candidate)
		if err != nil || candidate == normalized {
			continue
		}
		retired[candidate] = struct{}{}
	}
	r.routes.Store(&RouteState{PanelBasePath: normalized, RetiredPanelPaths: retired})
	return nil
}

func (r *Router) PanelBasePath() string {
	state := r.routes.Load()
	if state == nil {
		return "/management.html"
	}
	return state.PanelBasePath
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if strings.TrimSpace(req.Header.Get(protocol.GatewayHopHeader)) != "" {
		http.Error(w, "gateway proxy loop detected", http.StatusLoopDetected)
		return
	}
	if strings.TrimSpace(req.Header.Get(protocol.DirectCPARequestHeader)) != "" {
		http.Error(w, "CPA upstream must point directly to CPA", http.StatusMisdirectedRequest)
		return
	}
	state := r.routes.Load()
	if state == nil {
		http.Error(w, "gateway route state is unavailable", http.StatusServiceUnavailable)
		return
	}
	requestPath := cleanRequestPath(req.URL.Path)
	if requestPath == state.PanelBasePath {
		r.managerProxy.ServeHTTP(w, req)
		return
	}
	if _, retired := state.RetiredPanelPaths[requestPath]; retired {
		http.NotFound(w, req)
		return
	}
	if requestPath == "/management.html" && state.PanelBasePath != "/management.html" {
		http.NotFound(w, req)
		return
	}
	if req.URL.Path == "/" && state.PanelBasePath == "/management.html" {
		http.Redirect(w, req, state.PanelBasePath, http.StatusTemporaryRedirect)
		return
	}
	if isRuntimeOperationPath(req.URL.Path) {
		r.runtimeProxy.ServeHTTP(w, req)
		return
	}
	if isManagerPath(req.URL.Path) {
		r.managerProxy.ServeHTTP(w, req)
		return
	}
	cpaProxy := r.cpaProxy.Load()
	if cpaProxy == nil {
		http.Error(w, "CPA gateway target is unavailable", http.StatusServiceUnavailable)
		return
	}
	cpaProxy.ServeHTTP(w, req)
}

func parseTarget(raw string) (*url.URL, error) {
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, errors.New("gateway target must use http or https")
	}
	if target.Host == "" {
		return nil, errors.New("gateway target host is required")
	}
	return target, nil
}

func newProxy(target *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set(protocol.GatewayHopHeader, "1")
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "gateway upstream unavailable", http.StatusBadGateway)
	}
	return proxy
}

func cleanRequestPath(value string) string {
	if value == "/" {
		return value
	}
	return strings.TrimSuffix(value, "/")
}

func isRuntimeOperationPath(value string) bool {
	return strings.HasPrefix(value, "/v0/runtime/operations/")
}

func isManagerPath(value string) bool {
	clean := strings.TrimRight(value, "/")
	switch clean {
	case "/health", "/status", "/usage-service/info", "/usage-service/config",
		"/usage-service/account-processing-policy", "/usage-service/quota-cooldowns",
		"/setup", "/setup/admin-key", "/setup/admin-key/generate", "/v1/models", "/models":
		return true
	}
	return clean == "/v0/management" ||
		strings.HasPrefix(value, "/v0/management/") ||
		strings.HasPrefix(value, "/usage-service/") ||
		strings.HasPrefix(value, "/setup/") ||
		strings.HasPrefix(value, "/v0/resource/plugins/")
}
