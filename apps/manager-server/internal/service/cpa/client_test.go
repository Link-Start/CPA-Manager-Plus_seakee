package cpa

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/protocol"
)

func TestValidateManagementAPIMarksDirectCPARequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(protocol.DirectCPARequestHeader) != "1" {
			t.Errorf("direct CPA header = %q", r.Header.Get(protocol.DirectCPARequestHeader))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	if err := ValidateManagementAPI(t.Context(), server.URL, "test-key"); err != nil {
		t.Fatalf("validate management API: %v", err)
	}
}
