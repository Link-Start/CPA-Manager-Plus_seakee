package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/http/response"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/problem"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
)

type AdminVerifier interface {
	VerifyHeader(ctx context.Context, authorizationHeader string) (bool, error)
}

type PanelVerifier interface {
	VerifyPanelHeader(ctx context.Context, authorizationHeader string) (bool, error)
	PanelUsesExternalManagementKey(ctx context.Context) (bool, error)
}

func AuthorizeAdmin(w http.ResponseWriter, r *http.Request, verifier AdminVerifier) bool {
	ok, err := verifier.VerifyHeader(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, security.ErrAdminKeyVerificationBusy) {
			writeAdminVerificationBusy(w)
			return false
		}
		response.Error(w, http.StatusInternalServerError, err)
		return false
	}
	if ok {
		return true
	}
	response.Error(w, http.StatusUnauthorized, errors.New("invalid admin key"))
	return false
}

func AuthorizePanel(w http.ResponseWriter, r *http.Request, verifier PanelVerifier) bool {
	ok, err := verifier.VerifyPanelHeader(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, security.ErrAdminKeyVerificationBusy) {
			writeAdminVerificationBusy(w)
			return false
		}
		response.Error(w, http.StatusInternalServerError, err)
		return false
	}
	if ok {
		return true
	}
	external, err := verifier.PanelUsesExternalManagementKey(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err)
		return false
	}
	if external {
		response.Error(w, http.StatusUnauthorized, errors.New("invalid management key"))
		return false
	}
	response.Error(w, http.StatusUnauthorized, errors.New("invalid admin key"))
	return false
}

func writeAdminVerificationBusy(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "1")
	response.Error(w, http.StatusTooManyRequests, problem.New(
		"admin_verification_busy",
		"admin key verification is temporarily busy; retry shortly",
		http.StatusTooManyRequests,
		"admin-verification-busy",
	))
}
