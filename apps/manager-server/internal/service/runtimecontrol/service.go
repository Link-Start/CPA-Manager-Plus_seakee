package runtimecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
	managedruntime "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/managedruntime"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/problem"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/cpa"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/store"
)

type UpdateCapabilities struct {
	CPAMP bool `json:"cpamp"`
	CPA   bool `json:"cpa"`
	All   bool `json:"all"`
}

type StatusResult struct {
	Deployment         model.DeploymentState `json:"deployment"`
	RuntimeAvailable   bool                  `json:"runtimeAvailable"`
	RuntimeUnavailable string                `json:"runtimeUnavailableReason,omitempty"`
	Runtime            *managedruntime.State `json:"runtime,omitempty"`
	UpdateCapabilities UpdateCapabilities    `json:"updateCapabilities"`
}

type PanelBasePathResult struct {
	OK         bool                  `json:"ok"`
	PanelPath  string                `json:"panelPath"`
	Deployment model.DeploymentState `json:"deployment"`
}

type Service struct {
	cfg                config.Config
	store              *store.Store
	client             *http.Client
	requestTimeout     time.Duration
	longRequestTimeout time.Duration
}

const (
	defaultRuntimeRequestTimeout = 10 * time.Second
	longRuntimeRequestTimeout    = 10 * time.Minute
)

func New(cfg config.Config, store *store.Store) *Service {
	return &Service{
		cfg:                cfg,
		store:              store,
		client:             &http.Client{},
		requestTimeout:     defaultRuntimeRequestTimeout,
		longRequestTimeout: longRuntimeRequestTimeout,
	}
}

func (s *Service) Status(ctx context.Context) (StatusResult, error) {
	deployment, err := s.deployment(ctx)
	if err != nil {
		return StatusResult{}, err
	}
	result := StatusResult{
		Deployment: deployment,
		UpdateCapabilities: UpdateCapabilities{
			CPAMP: deployment.CPAMPUpdatesManaged,
			CPA:   deployment.CPAUpdatesManaged,
			All:   deployment.CPAMPUpdatesManaged && deployment.CPAUpdatesManaged,
		},
	}
	if !deployment.RuntimeManaged {
		result.RuntimeUnavailable = "deployment_not_runtime_managed"
		return result, nil
	}
	if strings.TrimSpace(s.cfg.RuntimeKey) == "" {
		result.RuntimeUnavailable = "runtime_control_key_missing"
		return result, nil
	}
	var runtimeState managedruntime.State
	if err := s.runtimeJSON(ctx, http.MethodGet, "/v0/runtime/status", nil, &runtimeState); err != nil {
		result.RuntimeUnavailable = "runtime_control_unreachable"
		return result, nil
	}
	result.RuntimeAvailable = true
	result.Runtime = &runtimeState
	return result, nil
}

func (s *Service) UpdatePanelBasePath(ctx context.Context, value string) (PanelBasePathResult, error) {
	if s.cfg.PanelBasePathEnvSet {
		return PanelBasePathResult{}, problem.New(
			"panel_base_path_env_managed",
			"panel base path is managed by an environment variable",
			http.StatusConflict,
			"panel-base-path-environment",
		)
	}
	normalized, err := model.NormalizePanelBasePath(value)
	if err != nil {
		return PanelBasePathResult{}, problem.Wrap(
			err,
			"panel_base_path_invalid",
			err.Error(),
			http.StatusBadRequest,
			"panel-base-path-invalid",
		)
	}
	deployment, err := s.deployment(ctx)
	if err != nil {
		return PanelBasePathResult{}, err
	}
	previousDeployment := deployment
	runtimeUpdated := false
	if deployment.RuntimeManaged {
		if strings.TrimSpace(s.cfg.RuntimeKey) == "" {
			return PanelBasePathResult{}, runtimeUnavailableError(errors.New("runtime control key is missing"))
		}
		var response struct {
			Deployment model.DeploymentState `json:"deployment"`
		}
		if err := s.runtimeJSON(
			ctx,
			http.MethodPut,
			"/v0/runtime/panel-base-path",
			map[string]string{"basePath": normalized},
			&response,
		); err != nil {
			return PanelBasePathResult{}, runtimeUnavailableError(err)
		}
		deployment = mergeRuntimeDeployment(response.Deployment, previousDeployment)
		runtimeUpdated = true
	} else {
		deployment = updateLocalPanelPath(deployment, normalized)
	}
	if err := s.store.SaveDeploymentState(ctx, deployment); err != nil {
		if runtimeUpdated {
			var rollback struct {
				Deployment model.DeploymentState `json:"deployment"`
			}
			if rollbackErr := s.runtimeJSON(
				ctx,
				http.MethodPut,
				"/v0/runtime/panel-base-path",
				map[string]string{"basePath": previousDeployment.PanelBasePath},
				&rollback,
			); rollbackErr != nil {
				return PanelBasePathResult{}, errors.Join(err, fmt.Errorf("restore previous runtime panel base path: %w", rollbackErr))
			}
		}
		return PanelBasePathResult{}, err
	}
	return PanelBasePathResult{OK: true, PanelPath: deployment.PanelBasePath, Deployment: deployment}, nil
}

func (s *Service) Deployment(ctx context.Context) (model.DeploymentState, error) {
	return s.deployment(ctx)
}

func (s *Service) ProvisionSlim(ctx context.Context) (managedruntime.SlimProvisionResult, error) {
	deployment, err := s.deployment(ctx)
	if err != nil {
		return managedruntime.SlimProvisionResult{}, err
	}
	if !deployment.RuntimeManaged ||
		(deployment.Mode != model.DeploymentModeSlim && deployment.Mode != model.DeploymentModeIntegrated) {
		return managedruntime.SlimProvisionResult{}, problem.New(
			"slim_cpa_provision_unavailable",
			"CPA download is only available for a Slim runtime deployment",
			http.StatusConflict,
			"slim-cpa-provision-unavailable",
		)
	}
	if strings.TrimSpace(s.cfg.RuntimeKey) == "" {
		return managedruntime.SlimProvisionResult{}, runtimeUnavailableError(errors.New("runtime control key is missing"))
	}
	var result managedruntime.SlimProvisionResult
	if err := s.runtimeJSONLong(ctx, http.MethodPost, "/v0/runtime/slim/provision", nil, &result); err != nil {
		return managedruntime.SlimProvisionResult{}, problem.Wrap(
			err,
			"slim_cpa_provision_failed",
			"failed to download and start the compatible CPA release",
			http.StatusBadGateway,
			"slim-cpa-download-failed",
		)
	}
	result.Deployment = mergeRuntimeDeployment(result.Deployment, deployment)
	if err := s.store.SaveDeploymentState(ctx, result.Deployment); err != nil {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if rollbackErr := s.RevertSlimProvision(rollbackCtx); rollbackErr != nil {
			return managedruntime.SlimProvisionResult{}, errors.Join(err, fmt.Errorf("restore previous Slim runtime: %w", rollbackErr))
		}
		return managedruntime.SlimProvisionResult{}, err
	}
	return result, nil
}

func (s *Service) RevertSlimProvision(ctx context.Context) error {
	if strings.TrimSpace(s.cfg.RuntimeKey) == "" {
		return runtimeUnavailableError(errors.New("runtime control key is missing"))
	}
	var response struct {
		Deployment model.DeploymentState `json:"deployment"`
	}
	if err := s.runtimeJSON(ctx, http.MethodPost, "/v0/runtime/slim/revert", nil, &response); err != nil {
		return runtimeUnavailableError(err)
	}
	managerDeployment, err := s.deployment(ctx)
	if err != nil {
		return err
	}
	response.Deployment = mergeRuntimeDeployment(response.Deployment, managerDeployment)
	return s.store.SaveDeploymentState(ctx, response.Deployment)
}

func (s *Service) CommitSlimProvision(ctx context.Context) error {
	if strings.TrimSpace(s.cfg.RuntimeKey) == "" {
		return runtimeUnavailableError(errors.New("runtime control key is missing"))
	}
	if err := s.runtimeJSON(ctx, http.MethodPost, "/v0/runtime/slim/commit", nil, nil); err != nil {
		return runtimeUnavailableError(err)
	}
	return nil
}

// ReconcileSlimProvision resolves a transition that was interrupted between
// persisting the Manager setup and committing the runtime transition. A
// persisted setup that still validates against the transition target is the
// durable commit record; otherwise the runtime is restored to its previous
// Slim state. It also repairs Manager deployment state when a completed
// runtime commit or revert response was lost.
func (s *Service) ReconcileSlimProvision(ctx context.Context) (bool, error) {
	deployment, err := s.deployment(ctx)
	if err != nil {
		return false, err
	}
	if !deployment.RuntimeManaged || strings.TrimSpace(s.cfg.RuntimeKey) == "" {
		return false, nil
	}

	var runtimeState managedruntime.State
	if err := s.runtimeJSON(ctx, http.MethodGet, "/v0/runtime/status", nil, &runtimeState); err != nil {
		return false, runtimeUnavailableError(err)
	}
	transition := runtimeState.SlimTransition
	if transition == nil {
		if runtimeState.Deployment.SchemaVersion == 0 || deploymentStatesEquivalent(deployment, runtimeState.Deployment) {
			return false, nil
		}
		reconciledDeployment := mergeRuntimeDeployment(runtimeState.Deployment, deployment)
		if err := s.store.SaveDeploymentState(ctx, reconciledDeployment); err != nil {
			return true, err
		}
		return true, nil
	}

	setup, setupOK, err := s.store.LoadSetup(ctx)
	if err != nil {
		return true, err
	}
	target := cpa.NormalizeBaseURL(transition.TargetCPAUpstreamURL)
	setupTarget := cpa.NormalizeBaseURL(setup.CPAUpstreamURL)
	if !setupOK || target == "" || setupTarget != target || strings.TrimSpace(setup.ManagementKey) == "" {
		return true, s.RevertSlimProvision(ctx)
	}
	if err := cpa.ValidateManagementAPI(ctx, target, setup.ManagementKey); err != nil {
		return true, problem.Wrap(
			err,
			"slim_transition_recovery_pending",
			"Slim runtime transition is waiting for the persisted CPA connection to recover",
			http.StatusBadGateway,
			"slim-transition-recovery-pending",
		)
	}
	reconciledDeployment := mergeRuntimeDeployment(runtimeState.Deployment, deployment)
	if err := s.store.SaveDeploymentState(ctx, reconciledDeployment); err != nil {
		return true, err
	}
	if err := s.CommitSlimProvision(ctx); err != nil {
		return true, err
	}
	return true, nil
}

func deploymentStatesEquivalent(left model.DeploymentState, right model.DeploymentState) bool {
	return left.SchemaVersion == right.SchemaVersion &&
		left.Mode == right.Mode &&
		left.PanelBasePath == right.PanelBasePath &&
		left.PanelBasePathSource == right.PanelBasePathSource &&
		left.RuntimeManaged == right.RuntimeManaged &&
		left.CPAMPUpdatesManaged == right.CPAMPUpdatesManaged &&
		left.CPAUpdatesManaged == right.CPAUpdatesManaged &&
		slices.Equal(left.RetiredPanelPaths, right.RetiredPanelPaths)
}

func mergeRuntimeDeployment(runtimeDeployment model.DeploymentState, managerDeployment model.DeploymentState) model.DeploymentState {
	runtimeDeployment.MigrationVersion = managerDeployment.MigrationVersion
	runtimeDeployment.MigrationCheckpoints = slices.Clone(managerDeployment.MigrationCheckpoints)
	return runtimeDeployment
}

func (s *Service) UseExistingCPA(ctx context.Context, cpaBaseURL string) (managedruntime.SlimProvisionResult, error) {
	cpaBaseURL = cpa.NormalizeBaseURL(cpaBaseURL)
	deployment, err := s.deployment(ctx)
	if err != nil {
		return managedruntime.SlimProvisionResult{}, err
	}
	if deployment.Mode != model.DeploymentModeSlim || !deployment.RuntimeManaged {
		return managedruntime.SlimProvisionResult{}, problem.New(
			"slim_cpa_provision_unavailable",
			"existing CPA selection is only available for a Slim runtime deployment",
			http.StatusConflict,
			"slim-cpa-provision-unavailable",
		)
	}
	if strings.TrimSpace(s.cfg.RuntimeKey) == "" {
		return managedruntime.SlimProvisionResult{}, runtimeUnavailableError(errors.New("runtime control key is missing"))
	}
	var result managedruntime.SlimProvisionResult
	if err := s.runtimeJSON(
		ctx,
		http.MethodPut,
		"/v0/runtime/slim/existing",
		map[string]string{"cpaUpstreamUrl": cpaBaseURL},
		&result,
	); err != nil {
		return managedruntime.SlimProvisionResult{}, runtimeUnavailableError(err)
	}
	result.Deployment = mergeRuntimeDeployment(result.Deployment, deployment)
	if err := s.store.SaveDeploymentState(ctx, result.Deployment); err != nil {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if rollbackErr := s.RevertSlimProvision(rollbackCtx); rollbackErr != nil {
			return managedruntime.SlimProvisionResult{}, errors.Join(err, fmt.Errorf("restore previous Slim runtime: %w", rollbackErr))
		}
		return managedruntime.SlimProvisionResult{}, err
	}
	return result, nil
}

func (s *Service) CheckUpdates(ctx context.Context) (managedruntime.UpdateCheckResult, error) {
	deployment, err := s.deployment(ctx)
	if err != nil {
		return managedruntime.UpdateCheckResult{}, err
	}
	if !deployment.CPAMPUpdatesManaged || !deployment.CPAUpdatesManaged {
		return managedruntime.UpdateCheckResult{}, problem.New(
			"runtime_update_not_managed",
			"updates are only managed for an Integrated deployment",
			http.StatusConflict,
			"runtime-update-not-managed",
		)
	}
	if strings.TrimSpace(s.cfg.RuntimeKey) == "" {
		return managedruntime.UpdateCheckResult{}, runtimeUnavailableError(errors.New("runtime control key is missing"))
	}
	var result managedruntime.UpdateCheckResult
	if err := s.runtimeJSONLong(ctx, http.MethodGet, "/v0/runtime/updates", nil, &result); err != nil {
		return managedruntime.UpdateCheckResult{}, problem.Wrap(
			err,
			"runtime_update_check_failed",
			"failed to check signed runtime releases",
			http.StatusBadGateway,
			"runtime-update-check-failed",
		)
	}
	return result, nil
}

func (s *Service) StartUpdate(ctx context.Context, target string) (managedruntime.StartUpdateResult, error) {
	deployment, err := s.deployment(ctx)
	if err != nil {
		return managedruntime.StartUpdateResult{}, err
	}
	if !deployment.CPAMPUpdatesManaged || !deployment.CPAUpdatesManaged {
		return managedruntime.StartUpdateResult{}, problem.New(
			"runtime_update_not_managed",
			"updates are only managed for an Integrated deployment",
			http.StatusConflict,
			"runtime-update-not-managed",
		)
	}
	if strings.TrimSpace(s.cfg.RuntimeKey) == "" {
		return managedruntime.StartUpdateResult{}, runtimeUnavailableError(errors.New("runtime control key is missing"))
	}
	var result managedruntime.StartUpdateResult
	if err := s.runtimeJSONLong(
		ctx,
		http.MethodPost,
		"/v0/runtime/updates",
		map[string]string{"target": target},
		&result,
	); err != nil {
		return managedruntime.StartUpdateResult{}, problem.Wrap(
			err,
			"runtime_update_start_failed",
			"failed to start the runtime update",
			http.StatusConflict,
			"runtime-update-start-failed",
		)
	}
	return result, nil
}

func (s *Service) Operation(ctx context.Context, id string) (managedruntime.UpdateOperation, error) {
	if strings.TrimSpace(s.cfg.RuntimeKey) == "" {
		return managedruntime.UpdateOperation{}, runtimeUnavailableError(errors.New("runtime control key is missing"))
	}
	var result managedruntime.UpdateOperation
	if err := s.runtimeJSON(
		ctx,
		http.MethodGet,
		"/v0/runtime/operations/"+strings.TrimSpace(id),
		nil,
		&result,
	); err != nil {
		return managedruntime.UpdateOperation{}, problem.Wrap(
			err,
			"runtime_operation_not_found",
			"runtime update operation is unavailable",
			http.StatusNotFound,
			"runtime-operation-not-found",
		)
	}
	return result, nil
}

func (s *Service) deployment(ctx context.Context) (model.DeploymentState, error) {
	deployment, ok, err := s.store.LoadDeploymentState(ctx)
	if err != nil {
		return model.DeploymentState{}, err
	}
	if ok {
		return deployment, nil
	}
	source := model.PanelBasePathSourceDefault
	if s.cfg.PanelBasePathEnvSet {
		source = model.PanelBasePathSourceEnvironment
	}
	basePath, err := model.NormalizePanelBasePath(s.cfg.PanelBasePath)
	if err != nil {
		return model.DeploymentState{}, err
	}
	return model.DefaultDeploymentState(model.NormalizeDeploymentMode(s.cfg.DeploymentMode), basePath, source), nil
}

func (s *Service) runtimeJSON(ctx context.Context, method string, path string, body any, out any) error {
	return s.runtimeJSONWithTimeout(ctx, s.requestTimeout, method, path, body, out)
}

func (s *Service) runtimeJSONLong(ctx context.Context, method string, path string, body any, out any) error {
	return s.runtimeJSONWithTimeout(ctx, s.longRequestTimeout, method, path, body, out)
}

func (s *Service) runtimeJSONWithTimeout(
	ctx context.Context,
	timeout time.Duration,
	method string,
	path string,
	body any,
	out any,
) error {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.cfg.RuntimeControlURL, "/")+path, payload)
	if err != nil {
		return err
	}
	request.Header.Set("X-CPAMP-Runtime-Key", s.cfg.RuntimeKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("runtime control request failed: %s", response.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}

func updateLocalPanelPath(deployment model.DeploymentState, normalized string) model.DeploymentState {
	previous := deployment.PanelBasePath
	deployment.RetiredPanelPaths = slices.DeleteFunc(
		deployment.RetiredPanelPaths,
		func(candidate string) bool { return candidate == normalized },
	)
	if previous != "" && previous != normalized && !slices.Contains(deployment.RetiredPanelPaths, previous) {
		deployment.RetiredPanelPaths = append(deployment.RetiredPanelPaths, previous)
		if len(deployment.RetiredPanelPaths) > 32 {
			deployment.RetiredPanelPaths = deployment.RetiredPanelPaths[len(deployment.RetiredPanelPaths)-32:]
		}
	}
	deployment.PanelBasePath = normalized
	deployment.PanelBasePathSource = model.PanelBasePathSourceDatabase
	return deployment
}

func runtimeUnavailableError(err error) error {
	return problem.Wrap(
		err,
		"runtime_control_unavailable",
		"CPAMP runtime control is unavailable",
		http.StatusServiceUnavailable,
		"runtime-control-unavailable",
	)
}
