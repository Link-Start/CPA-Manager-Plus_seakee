package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
)

type saturatedAdminVerifier struct{}

func (saturatedAdminVerifier) VerifyHeader(context.Context, string) (bool, error) {
	return false, security.ErrAdminKeyVerificationBusy
}

func TestAuthorizeAdminReturnsTooManyRequestsWhenVerificationIsSaturated(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if AuthorizeAdmin(recorder, request, saturatedAdminVerifier{}) {
		t.Fatal("saturated verifier was authorized")
	}
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") != "1" {
		t.Fatalf("response status=%d retry-after=%q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
	var payload map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "admin_verification_busy" {
		t.Fatalf("response payload = %+v", payload)
	}
}
