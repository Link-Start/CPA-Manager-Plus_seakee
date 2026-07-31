package managedruntime

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
)

type ControlHandler struct {
	controller *RuntimeController
	runtimeKey string
}

func NewControlHandler(controller *RuntimeController, runtimeKey string) http.Handler {
	handler := &ControlHandler{controller: controller, runtimeKey: runtimeKey}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.health)
	mux.HandleFunc("/v0/runtime/status", handler.status)
	mux.HandleFunc("/v0/runtime/panel-base-path", handler.panelBasePath)
	mux.HandleFunc("/v0/runtime/slim/provision", handler.slimProvision)
	mux.HandleFunc("/v0/runtime/slim/existing", handler.slimExisting)
	mux.HandleFunc("/v0/runtime/slim/commit", handler.slimCommit)
	mux.HandleFunc("/v0/runtime/slim/revert", handler.slimRevert)
	mux.HandleFunc("/v0/runtime/updates", handler.updates)
	mux.HandleFunc("/v0/runtime/operations/", handler.operation)
	return mux
}

func (h *ControlHandler) updates(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(r) {
		writeControlError(w, http.StatusUnauthorized, errors.New("invalid runtime control key"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.controller.CheckUpdates(r.Context())
		if err != nil {
			writeControlError(w, http.StatusBadGateway, err)
			return
		}
		writeControlJSON(w, http.StatusOK, result)
	case http.MethodPost:
		var req struct {
			Target string `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeControlError(w, http.StatusBadRequest, err)
			return
		}
		result, err := h.controller.StartUpdate(r.Context(), req.Target)
		if err != nil {
			writeControlError(w, http.StatusConflict, err)
			return
		}
		result.Operation = sanitizeOperation(result.Operation)
		writeControlJSON(w, http.StatusAccepted, result)
	default:
		writeControlError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (h *ControlHandler) slimProvision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeControlError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if !h.authorize(r) {
		writeControlError(w, http.StatusUnauthorized, errors.New("invalid runtime control key"))
		return
	}
	result, err := h.controller.ProvisionSlim(r.Context())
	if err != nil {
		writeControlError(w, http.StatusBadGateway, err)
		return
	}
	writeControlJSON(w, http.StatusOK, result)
}

func (h *ControlHandler) slimExisting(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeControlError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if !h.authorize(r) {
		writeControlError(w, http.StatusUnauthorized, errors.New("invalid runtime control key"))
		return
	}
	var req struct {
		CPAUpstreamURL string `json:"cpaUpstreamUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeControlError(w, http.StatusBadRequest, err)
		return
	}
	result, err := h.controller.UseExistingCPA(req.CPAUpstreamURL)
	if err != nil {
		writeControlError(w, http.StatusBadRequest, err)
		return
	}
	writeControlJSON(w, http.StatusOK, result)
}

func (h *ControlHandler) slimRevert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeControlError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if !h.authorize(r) {
		writeControlError(w, http.StatusUnauthorized, errors.New("invalid runtime control key"))
		return
	}
	deployment, err := h.controller.RevertSlimProvision(r.Context())
	if err != nil {
		writeControlError(w, http.StatusInternalServerError, err)
		return
	}
	writeControlJSON(w, http.StatusOK, map[string]any{"deployment": deployment})
}

func (h *ControlHandler) slimCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeControlError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if !h.authorize(r) {
		writeControlError(w, http.StatusUnauthorized, errors.New("invalid runtime control key"))
		return
	}
	if err := h.controller.CommitSlimTransition(); err != nil {
		writeControlError(w, http.StatusInternalServerError, err)
		return
	}
	writeControlJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *ControlHandler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeControlError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	writeControlJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "cpamp-runtime"})
}

func (h *ControlHandler) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeControlError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if !h.authorize(r) {
		writeControlError(w, http.StatusUnauthorized, errors.New("invalid runtime control key"))
		return
	}
	writeControlJSON(w, http.StatusOK, sanitizeRuntimeState(h.controller.State()))
}

func (h *ControlHandler) panelBasePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeControlError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if !h.authorize(r) {
		writeControlError(w, http.StatusUnauthorized, errors.New("invalid runtime control key"))
		return
	}
	var req struct {
		BasePath string `json:"basePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeControlError(w, http.StatusBadRequest, err)
		return
	}
	deployment, err := h.controller.UpdatePanelBasePath(req.BasePath)
	if err != nil {
		writeControlError(w, http.StatusBadRequest, err)
		return
	}
	writeControlJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"panelPath":  deployment.PanelBasePath,
		"deployment": deployment,
	})
}

func (h *ControlHandler) operation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeControlError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v0/runtime/operations/"), "/")
	if id == "" {
		writeControlError(w, http.StatusNotFound, errors.New("runtime operation not found"))
		return
	}
	if !h.authorize(r) {
		token := strings.TrimSpace(r.Header.Get("X-CPAMP-Operation-Token"))
		if !h.controller.AuthorizeOperation(id, token) {
			writeControlError(w, http.StatusUnauthorized, errors.New("invalid runtime operation token"))
			return
		}
	}
	operation, ok := h.controller.Operation(id)
	if !ok {
		writeControlError(w, http.StatusNotFound, errors.New("runtime operation not found"))
		return
	}
	writeControlJSON(w, http.StatusOK, sanitizeOperation(operation))
}

func (h *ControlHandler) authorize(r *http.Request) bool {
	provided := strings.TrimSpace(r.Header.Get("X-CPAMP-Runtime-Key"))
	return provided != "" && security.EqualHMAC(provided, h.runtimeKey)
}

func writeControlError(w http.ResponseWriter, status int, err error) {
	writeControlJSON(w, status, map[string]any{"error": err.Error()})
}

func writeControlJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func sanitizeOperation(operation UpdateOperation) UpdateOperation {
	operation.OperationTokenHash = ""
	operation.CurrentBinaries = nil
	return operation
}

func sanitizeRuntimeState(state State) State {
	for id, operation := range state.Operations {
		state.Operations[id] = sanitizeOperation(operation)
	}
	return state
}
