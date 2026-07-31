package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
)

func TestWriteCORSAllowsPatch(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/usage-service/account-processing-policy", nil)
	WriteCORS(config.Config{CORSOrigins: []string{"*"}}, rr, req)

	methods := rr.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(methods, "PATCH") {
		t.Fatalf("Access-Control-Allow-Methods = %q, want PATCH", methods)
	}
}

func TestWriteCORSAllowsSetupBootstrapTokenHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/setup/admin-key", nil)
	WriteCORS(config.Config{CORSOrigins: []string{"*"}}, rr, req)

	headers := rr.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(headers, "X-CPAMP-Bootstrap-Token") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want X-CPAMP-Bootstrap-Token", headers)
	}
}
