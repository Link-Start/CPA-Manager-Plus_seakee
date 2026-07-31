package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/app"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/http/response"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/problem"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
	setupsvc "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/setup"
)

type Handler struct {
	App        *app.Context
	mutationMu sync.Mutex
}

func (h *Handler) Setup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.MethodNotAllowed(w)
		return
	}
	h.mutationMu.Lock()
	defer h.mutationMu.Unlock()
	if !h.authorize(w, r) {
		return
	}
	var req setupsvc.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, err)
		return
	}
	deployment, err := h.App.RuntimeControlService.Deployment(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err)
		return
	}
	slimTransitioned := false
	if deployment.Mode == model.DeploymentModeSlim {
		if _, err := h.App.RuntimeControlService.UseExistingCPA(r.Context(), req.CPAUpstreamURL); err != nil {
			response.Error(w, response.SetupErrorStatus(err), err)
			return
		}
		slimTransitioned = true
	}
	result, err := h.App.SetupService.Setup(r.Context(), req, r.Header.Get("Authorization"))
	if err != nil {
		if slimTransitioned {
			rollbackCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if rollbackErr := h.App.RuntimeControlService.RevertSlimProvision(rollbackCtx); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("restore previous Slim runtime: %w", rollbackErr))
			}
		}
		response.Error(w, response.SetupErrorStatus(err), err)
		return
	}
	if slimTransitioned {
		if err := h.commitSlimTransition(); err != nil {
			response.Error(w, response.SetupErrorStatus(err), err)
			return
		}
	}
	response.JSON(w, http.StatusOK, result)
}

func (h *Handler) CPASource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.MethodNotAllowed(w)
		return
	}
	h.mutationMu.Lock()
	defer h.mutationMu.Unlock()
	if !h.authorize(w, r) {
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, err)
		return
	}
	switch req.Action {
	case "use_existing":
		response.JSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"action":   req.Action,
			"nextStep": "cpa_connection",
		})
	case "download_latest":
		provisioned, err := h.App.RuntimeControlService.ProvisionSlim(r.Context())
		if err != nil {
			response.Error(w, response.SetupErrorStatus(err), err)
			return
		}
		monitoring := true
		setupResult, err := h.App.SetupService.Setup(r.Context(), setupsvc.Request{
			CPAUpstreamURL:               provisioned.CPAUpstreamURL,
			CPAManagementKey:             provisioned.CPAManagementKey,
			EnsureUsageStatisticsEnabled: &monitoring,
			RequestMonitoringEnabled:     &monitoring,
		}, r.Header.Get("Authorization"))
		if err != nil {
			rollbackCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if rollbackErr := h.App.RuntimeControlService.RevertSlimProvision(rollbackCtx); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("restore previous Slim runtime: %w", rollbackErr))
			}
			response.Error(w, response.SetupErrorStatus(err), err)
			return
		}
		if err := h.commitSlimTransition(); err != nil {
			response.Error(w, response.SetupErrorStatus(err), err)
			return
		}
		response.JSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"action":     req.Action,
			"nextStep":   setupResult.NextStep,
			"installed":  provisioned.Installed,
			"deployment": provisioned.Deployment,
		})
	default:
		response.Error(w, http.StatusBadRequest, problem.New(
			"slim_cpa_source_invalid",
			"Slim CPA source action is invalid",
			http.StatusBadRequest,
			"slim-cpa-source-invalid",
		))
	}
}

func (h *Handler) commitSlimTransition() error {
	commitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return h.App.RuntimeControlService.CommitSlimProvision(commitCtx)
}

func (h *Handler) AdminKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.MethodNotAllowed(w)
		return
	}
	h.mutationMu.Lock()
	defer h.mutationMu.Unlock()
	if !h.authorize(w, r) {
		return
	}
	var req setupsvc.AdminKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, err)
		return
	}
	if _, err := h.App.RuntimeControlService.ReconcileSlimProvision(r.Context()); err != nil {
		response.Error(w, response.SetupErrorStatus(err), err)
		return
	}
	result, err := h.App.SetupService.InitializeAdmin(r.Context(), req)
	if err != nil {
		response.Error(w, response.SetupErrorStatus(err), err)
		return
	}
	response.JSON(w, http.StatusOK, result)
}

func (h *Handler) GenerateAdminKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.MethodNotAllowed(w)
		return
	}
	if !h.authorize(w, r) {
		return
	}
	result, err := h.App.SetupService.GenerateAdminKey()
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err)
		return
	}
	response.JSON(w, http.StatusOK, result)
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) bool {
	err := h.App.SetupService.AuthorizeSetup(
		r.Context(),
		r.Header.Get("Authorization"),
		r.Header.Get("X-CPAMP-Bootstrap-Token"),
	)
	if err == nil {
		return true
	}
	if errors.Is(err, security.ErrAdminKeyVerificationBusy) {
		w.Header().Set("Retry-After", "1")
	}
	response.Error(w, response.SetupErrorStatus(err), err)
	return false
}
