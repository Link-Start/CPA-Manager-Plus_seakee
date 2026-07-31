package runtimecontrol

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/app"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/http/middleware"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/http/response"
)

type Handler struct {
	App *app.Context
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	if !middleware.AuthorizePanel(w, r, h.App.AdminAuthService) {
		return
	}
	clean := strings.TrimRight(r.URL.Path, "/")
	switch clean {
	case "/v0/management/runtime":
		h.status(w, r)
	case "/v0/management/runtime/panel-base-path":
		h.panelBasePath(w, r)
	case "/v0/management/runtime/updates":
		h.updates(w, r)
	default:
		if strings.HasPrefix(clean, "/v0/management/runtime/operations/") {
			h.operation(w, r, strings.TrimPrefix(clean, "/v0/management/runtime/operations/"))
			return
		}
		http.NotFound(w, r)
	}
}

func (h *Handler) updates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result, err := h.App.RuntimeControlService.CheckUpdates(r.Context())
		if err != nil {
			response.Error(w, http.StatusBadGateway, err)
			return
		}
		response.JSON(w, http.StatusOK, result)
	case http.MethodPost:
		var req struct {
			Target string `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, http.StatusBadRequest, err)
			return
		}
		result, err := h.App.RuntimeControlService.StartUpdate(r.Context(), req.Target)
		if err != nil {
			response.Error(w, http.StatusConflict, err)
			return
		}
		response.JSON(w, http.StatusAccepted, result)
	default:
		response.MethodNotAllowed(w)
	}
}

func (h *Handler) operation(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		response.MethodNotAllowed(w)
		return
	}
	if strings.TrimSpace(id) == "" {
		http.NotFound(w, r)
		return
	}
	result, err := h.App.RuntimeControlService.Operation(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusNotFound, err)
		return
	}
	response.JSON(w, http.StatusOK, result)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.MethodNotAllowed(w)
		return
	}
	result, err := h.App.RuntimeControlService.Status(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err)
		return
	}
	response.JSON(w, http.StatusOK, result)
}

func (h *Handler) panelBasePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		response.MethodNotAllowed(w)
		return
	}
	var req struct {
		BasePath string `json:"basePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, err)
		return
	}
	result, err := h.App.RuntimeControlService.UpdatePanelBasePath(r.Context(), req.BasePath)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err)
		return
	}
	response.JSON(w, http.StatusOK, result)
}
