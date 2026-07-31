package managedruntime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/gateway"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
)

type RuntimeController struct {
	ctx        context.Context
	cfg        Config
	state      *StateStore
	router     *gateway.Router
	secrets    Secrets
	supervisor runtimeProcessSupervisor
	installer  runtimeReleaseInstaller

	mu                  sync.Mutex
	cpaStarted          bool
	cpaCancel           context.CancelFunc
	cpaDone             chan struct{}
	slimPreviousConfig  *Config
	slimPreviousSecrets *Secrets
	runtimeMutationMu   sync.Mutex
	updateMu            sync.Mutex
	activeUpdateID      string
	processWG           sync.WaitGroup
	handoff             chan<- runtimeHandoff
}

type runtimeHandoff struct {
	Binary      string
	OperationID string
}

type runtimeProcessSupervisor interface {
	Run(context.Context, string, ProcessSpecProvider)
	Restart(string) error
}

type runtimeReleaseInstaller interface {
	FetchManifest(context.Context) (ReleaseManifest, error)
	InstallLatest(context.Context, string) (InstalledComponent, error)
	InstallComponent(context.Context, string, ReleaseComponent) (InstalledComponent, error)
}

func NewRuntimeController(
	ctx context.Context,
	cfg Config,
	state *StateStore,
	router *gateway.Router,
	secrets Secrets,
	supervisor *ProcessSupervisor,
) *RuntimeController {
	return &RuntimeController{
		ctx:        ctx,
		cfg:        cfg,
		state:      state,
		router:     router,
		secrets:    secrets,
		supervisor: supervisor,
		installer:  NewReleaseInstaller(cfg),
	}
}

type SlimProvisionResult struct {
	OK               bool                  `json:"ok"`
	CPAUpstreamURL   string                `json:"cpaUpstreamUrl"`
	CPAManagementKey string                `json:"cpaManagementKey,omitempty"`
	Installed        *InstalledComponent   `json:"installed,omitempty"`
	Deployment       model.DeploymentState `json:"deployment"`
}

func (c *RuntimeController) State() State {
	return c.state.Snapshot()
}

func (c *RuntimeController) StartManager() {
	c.processWG.Add(1)
	go func() {
		defer c.processWG.Done()
		c.supervisor.Run(c.ctx, "cpamp", c.managerSpec)
	}()
}

func (c *RuntimeController) StartCPA() error {
	if c.supervisor == nil {
		return errors.New("CPA process supervisor is unavailable")
	}
	spec := c.cpaSpec()
	if strings.TrimSpace(spec.Binary) == "" {
		return errors.New("CPA binary is unavailable")
	}
	parent := c.ctx
	if parent == nil {
		parent = context.Background()
	}
	runCtx, cancel := context.WithCancel(parent)
	done := make(chan struct{})

	c.mu.Lock()
	if c.cpaStarted {
		c.mu.Unlock()
		cancel()
		return nil
	}
	c.cpaStarted = true
	c.cpaCancel = cancel
	c.cpaDone = done
	c.processWG.Add(1)
	c.mu.Unlock()
	go func() {
		c.supervisor.Run(runCtx, "cpa", c.cpaSpec)
		c.mu.Lock()
		if c.cpaDone == done {
			c.cpaStarted = false
			c.cpaCancel = nil
			c.cpaDone = nil
		}
		c.mu.Unlock()
		close(done)
		c.processWG.Done()
	}()
	return nil
}

func (c *RuntimeController) StopCPA(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	if !c.cpaStarted {
		c.mu.Unlock()
		return nil
	}
	cancel := c.cpaCancel
	done := c.cpaDone
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *RuntimeController) SetHandoffChannel(channel chan<- runtimeHandoff) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handoff = channel
}

func (c *RuntimeController) WaitProcesses(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		c.processWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *RuntimeController) UpdatePanelBasePath(value string) (model.DeploymentState, error) {
	previous := c.state.Snapshot().Deployment
	deployment, err := c.state.UpdatePanelBasePath(value)
	if err != nil {
		return model.DeploymentState{}, err
	}
	if err := c.router.UpdatePanelRoute(deployment.PanelBasePath, deployment.RetiredPanelPaths); err != nil {
		rollback, rollbackErr := c.state.UpdatePanelBasePath(previous.PanelBasePath)
		if rollbackErr == nil {
			rollbackErr = c.router.UpdatePanelRoute(rollback.PanelBasePath, rollback.RetiredPanelPaths)
		}
		if rollbackErr != nil {
			return model.DeploymentState{}, errors.Join(err, fmt.Errorf("restore previous panel route: %w", rollbackErr))
		}
		return model.DeploymentState{}, err
	}
	return deployment, nil
}

func (c *RuntimeController) UpdateCPAUpstream(value string) (State, error) {
	previous := c.state.Snapshot().CPAUpstreamURL
	if err := c.router.UpdateCPATarget(value); err != nil {
		return State{}, err
	}
	updated, err := c.state.UpdateCPAUpstream(value)
	if err == nil {
		return updated, nil
	}
	if strings.TrimSpace(previous) != "" {
		if rollbackErr := c.router.UpdateCPATarget(previous); rollbackErr != nil {
			return State{}, errors.Join(err, fmt.Errorf("restore previous CPA upstream: %w", rollbackErr))
		}
	}
	return State{}, err
}

func (c *RuntimeController) ProvisionSlim(ctx context.Context) (SlimProvisionResult, error) {
	c.runtimeMutationMu.Lock()
	defer c.runtimeMutationMu.Unlock()
	snapshot := c.state.Snapshot()
	if err := rejectInProgressRuntimeUpdate(snapshot); err != nil {
		return SlimProvisionResult{}, err
	}
	if snapshot.SlimTransition != nil {
		return c.resumeSlimProvision(snapshot)
	}
	if snapshot.Deployment.Mode == model.DeploymentModeIntegrated {
		return c.currentSlimProvisionResult(snapshot), nil
	}
	if snapshot.Deployment.Mode != model.DeploymentModeSlim {
		return SlimProvisionResult{}, errors.New("CPA download is only available for a Slim deployment")
	}
	installed, err := c.installer.InstallLatest(ctx, "cpa")
	if err != nil {
		return SlimProvisionResult{}, err
	}
	integratedConfig := c.configSnapshot()
	integratedConfig.DeploymentMode = model.DeploymentModeIntegrated
	integratedConfig.CPABinary = installed.BinaryPath
	secrets, err := EnsureAssets(integratedConfig)
	if err != nil {
		return SlimProvisionResult{}, err
	}
	c.mu.Lock()
	previousConfig := c.cfg
	previousSecrets := c.secrets
	previousCPAStarted := c.cpaStarted
	c.mu.Unlock()
	_, err = c.state.BeginSlimTransition(
		slimTransitionProvision,
		integratedConfig.CPAURL,
		previousConfig.CPABinary,
		previousCPAStarted,
		&installed,
	)
	if err != nil {
		return SlimProvisionResult{}, err
	}
	c.rememberSlimTransition(previousConfig, previousSecrets)
	if err := c.state.UpdateComponent("cpa", func(component ComponentState) ComponentState {
		component.BinaryPath = installed.BinaryPath
		component.Version = installed.Version
		component.Status = "installed"
		component.LastError = ""
		return component
	}); err != nil {
		return SlimProvisionResult{}, c.rollbackSlimFailure(err)
	}
	c.mu.Lock()
	c.cfg.DeploymentMode = model.DeploymentModeIntegrated
	c.cfg.CPABinary = installed.BinaryPath
	c.secrets.CPAManagementKey = secrets.CPAManagementKey
	c.mu.Unlock()
	if _, err := c.UpdateCPAUpstream(integratedConfig.CPAURL); err != nil {
		return SlimProvisionResult{}, c.rollbackSlimFailure(err)
	}
	if err := c.StartCPA(); err != nil {
		return SlimProvisionResult{}, c.rollbackSlimFailure(err)
	}
	if err := waitHTTPHealthy(ctx, strings.TrimRight(integratedConfig.CPAURL, "/")+"/healthz", 45*time.Second); err != nil {
		return SlimProvisionResult{}, c.rollbackSlimFailure(fmt.Errorf("start downloaded CPA: %w", err))
	}
	deployment, err := c.state.UpdateDeployment(func(deployment model.DeploymentState) model.DeploymentState {
		deployment.Mode = model.DeploymentModeIntegrated
		deployment.RuntimeManaged = true
		deployment.CPAMPUpdatesManaged = true
		deployment.CPAUpdatesManaged = true
		return deployment
	})
	if err != nil {
		return SlimProvisionResult{}, c.rollbackSlimFailure(err)
	}
	return SlimProvisionResult{
		OK:               true,
		CPAUpstreamURL:   integratedConfig.CPAURL,
		CPAManagementKey: secrets.CPAManagementKey,
		Installed:        &installed,
		Deployment:       deployment,
	}, nil
}

func (c *RuntimeController) UseExistingCPA(value string) (SlimProvisionResult, error) {
	c.runtimeMutationMu.Lock()
	defer c.runtimeMutationMu.Unlock()
	value = strings.TrimSpace(value)
	snapshot := c.state.Snapshot()
	if err := rejectInProgressRuntimeUpdate(snapshot); err != nil {
		return SlimProvisionResult{}, err
	}
	if snapshot.SlimTransition != nil {
		return c.resumeExistingCPA(snapshot, value)
	}
	if snapshot.Deployment.Mode != model.DeploymentModeSlim {
		return SlimProvisionResult{}, errors.New("existing CPA selection is only available for a Slim deployment")
	}
	c.mu.Lock()
	previousConfig := c.cfg
	previousSecrets := c.secrets
	previousCPAStarted := c.cpaStarted
	c.mu.Unlock()
	_, err := c.state.BeginSlimTransition(
		slimTransitionExisting,
		value,
		previousConfig.CPABinary,
		previousCPAStarted,
		nil,
	)
	if err != nil {
		return SlimProvisionResult{}, err
	}
	c.rememberSlimTransition(previousConfig, previousSecrets)
	updated, err := c.UpdateCPAUpstream(value)
	if err != nil {
		return SlimProvisionResult{}, c.rollbackSlimFailure(err)
	}
	return SlimProvisionResult{
		OK:             true,
		CPAUpstreamURL: updated.CPAUpstreamURL,
		Deployment:     updated.Deployment,
	}, nil
}

func (c *RuntimeController) RevertSlimProvision(ctx context.Context) (model.DeploymentState, error) {
	c.runtimeMutationMu.Lock()
	defer c.runtimeMutationMu.Unlock()
	if err := rejectInProgressRuntimeUpdate(c.state.Snapshot()); err != nil {
		return model.DeploymentState{}, err
	}
	return c.revertSlimTransitionLocked(ctx)
}

func (c *RuntimeController) CommitSlimTransition() error {
	c.runtimeMutationMu.Lock()
	defer c.runtimeMutationMu.Unlock()
	if err := rejectInProgressRuntimeUpdate(c.state.Snapshot()); err != nil {
		return err
	}
	if err := c.state.ClearSlimTransition(); err != nil {
		return err
	}
	c.clearSlimTransitionMemory()
	return nil
}

func (c *RuntimeController) rememberSlimTransition(cfg Config, secrets Secrets) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slimPreviousConfig = &cfg
	c.slimPreviousSecrets = &secrets
}

func (c *RuntimeController) clearSlimTransitionMemory() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slimPreviousConfig = nil
	c.slimPreviousSecrets = nil
}

func (c *RuntimeController) resumeSlimProvision(snapshot State) (SlimProvisionResult, error) {
	if snapshot.SlimTransition.Kind != slimTransitionProvision {
		return SlimProvisionResult{}, errors.New("a different Slim runtime transition is already pending")
	}
	if snapshot.Deployment.Mode != model.DeploymentModeIntegrated {
		return SlimProvisionResult{}, errors.New("pending Slim CPA provision has an invalid deployment state")
	}
	return c.currentSlimProvisionResult(snapshot), nil
}

func (c *RuntimeController) currentSlimProvisionResult(snapshot State) SlimProvisionResult {
	var installed *InstalledComponent
	if snapshot.SlimTransition != nil && snapshot.SlimTransition.Installed != nil {
		installedCopy := *snapshot.SlimTransition.Installed
		installed = &installedCopy
	} else if component, ok := snapshot.Components["cpa"]; ok &&
		(strings.TrimSpace(component.Version) != "" || strings.TrimSpace(component.BinaryPath) != "") {
		installed = &InstalledComponent{
			Name:       "cpa",
			Version:    component.Version,
			BinaryPath: component.BinaryPath,
		}
	}
	c.mu.Lock()
	managementKey := c.secrets.CPAManagementKey
	c.mu.Unlock()
	return SlimProvisionResult{
		OK:               true,
		CPAUpstreamURL:   snapshot.CPAUpstreamURL,
		CPAManagementKey: managementKey,
		Installed:        installed,
		Deployment:       snapshot.Deployment,
	}
}

func (c *RuntimeController) resumeExistingCPA(snapshot State, value string) (SlimProvisionResult, error) {
	transition := snapshot.SlimTransition
	if transition.Kind != slimTransitionExisting {
		return SlimProvisionResult{}, errors.New("a different Slim runtime transition is already pending")
	}
	if transition.TargetCPAUpstreamURL != value {
		return SlimProvisionResult{}, errors.New("a Slim transition for another CPA upstream is already pending")
	}
	return SlimProvisionResult{
		OK:             true,
		CPAUpstreamURL: snapshot.CPAUpstreamURL,
		Deployment:     snapshot.Deployment,
	}, nil
}

func (c *RuntimeController) rollbackSlimFailure(cause error) error {
	if c.state.Snapshot().SlimTransition == nil {
		return errors.Join(cause, errors.New("Slim runtime rollback snapshot is unavailable"))
	}
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := c.revertSlimTransitionLocked(rollbackCtx); err != nil {
		return errors.Join(cause, fmt.Errorf("restore previous Slim runtime: %w", err))
	}
	return cause
}

func (c *RuntimeController) revertSlimTransitionLocked(ctx context.Context) (model.DeploymentState, error) {
	current := c.state.Snapshot()
	transition := current.SlimTransition
	if transition == nil {
		return current.Deployment, nil
	}
	c.mu.Lock()
	currentCPAStarted := c.cpaStarted
	cachedPreviousConfig := c.slimPreviousConfig
	cachedPreviousSecrets := c.slimPreviousSecrets
	c.mu.Unlock()
	if c.router == nil {
		return model.DeploymentState{}, errors.New("runtime gateway router is unavailable")
	}
	previousCfg := c.configSnapshot()
	previousCfg.DeploymentMode = transition.PreviousDeployment.Mode
	previousCfg.CPABinary = transition.PreviousCPABinary
	var previousSecrets Secrets
	if cachedPreviousConfig != nil && cachedPreviousSecrets != nil {
		previousCfg = *cachedPreviousConfig
		previousSecrets = *cachedPreviousSecrets
	} else {
		var err error
		previousSecrets, err = EnsureAssets(previousCfg)
		if err != nil {
			return model.DeploymentState{}, fmt.Errorf("restore previous Slim runtime assets: %w", err)
		}
	}
	stoppedCPA := currentCPAStarted && !transition.PreviousCPAStarted
	if stoppedCPA {
		if err := c.StopCPA(ctx); err != nil {
			return model.DeploymentState{}, fmt.Errorf("stop provisioned CPA: %w", err)
		}
	}
	if err := c.router.UpdateCPATarget(transition.PreviousCPAUpstreamURL); err != nil {
		if stoppedCPA {
			if restartErr := c.StartCPA(); restartErr != nil {
				return model.DeploymentState{}, errors.Join(err, fmt.Errorf("restart provisioned CPA after rollback failure: %w", restartErr))
			}
		}
		return model.DeploymentState{}, err
	}
	deployment, err := c.state.RestoreSlimTransition()
	if err != nil {
		var rollbackErr error
		if strings.TrimSpace(current.CPAUpstreamURL) != "" {
			rollbackErr = c.router.UpdateCPATarget(current.CPAUpstreamURL)
		}
		if stoppedCPA {
			rollbackErr = errors.Join(rollbackErr, c.StartCPA())
		}
		if rollbackErr != nil {
			return model.DeploymentState{}, errors.Join(err, fmt.Errorf("restore current runtime after rollback failure: %w", rollbackErr))
		}
		return model.DeploymentState{}, err
	}
	c.mu.Lock()
	c.cfg = previousCfg
	c.secrets = previousSecrets
	c.slimPreviousConfig = nil
	c.slimPreviousSecrets = nil
	c.mu.Unlock()
	if transition.PreviousCPAStarted && !currentCPAStarted {
		if err := c.StartCPA(); err != nil {
			return model.DeploymentState{}, fmt.Errorf("restart previous CPA after Slim rollback: %w", err)
		}
	}
	return deployment, nil
}

func (c *RuntimeController) managerSpec() ProcessSpec {
	snapshot := c.state.Snapshot()
	managerDeploymentMode := snapshot.Deployment.Mode
	if snapshot.SlimTransition != nil {
		managerDeploymentMode = snapshot.SlimTransition.PreviousDeployment.Mode
	}
	c.mu.Lock()
	cfg := c.cfg
	secrets := c.secrets
	c.mu.Unlock()
	binary := strings.TrimSpace(snapshot.Components["cpamp"].BinaryPath)
	if binary == "" {
		binary = cfg.ManagerBinary
	}
	return ProcessSpec{
		Name:    "cpamp",
		Binary:  binary,
		Version: firstNonEmpty(snapshot.Components["cpamp"].Version, cfg.CPAMPVersion),
		Args:    []string{"serve"},
		Dir:     filepath.Dir(binary),
		Env: map[string]string{
			"HTTP_ADDR":                          cfg.ManagerAddr,
			"USAGE_DATA_DIR":                     cfg.DataDir,
			"USAGE_DB_PATH":                      firstNonEmpty(cfg.UsageDBPath, filepath.Join(cfg.DataDir, "usage.sqlite")),
			"CPA_MANAGER_DEPLOYMENT_MODE":        string(managerDeploymentMode),
			"CPA_MANAGER_PANEL_BASE_PATH":        snapshot.Deployment.PanelBasePath,
			"CPA_MANAGER_PANEL_BASE_PATH_SOURCE": snapshot.Deployment.PanelBasePathSource,
			"CPA_MANAGER_RUNTIME_CONTROL_URL":    cfg.ControlURL,
			"CPA_MANAGER_RUNTIME_KEY":            secrets.RuntimeKey,
			"CPA_UPSTREAM_URL":                   snapshot.CPAUpstreamURL,
			"CPA_MANAGEMENT_KEY":                 secrets.CPAManagementKey,
		},
	}
}

func (c *RuntimeController) configSnapshot() Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

func waitHTTPHealthy(ctx context.Context, target string, timeout time.Duration) error {
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(deadlineCtx, http.MethodGet, target, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(req)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("health endpoint returned %s", response.Status)
		} else {
			lastErr = err
		}
		select {
		case <-deadlineCtx.Done():
			if lastErr == nil {
				lastErr = deadlineCtx.Err()
			}
			return lastErr
		case <-ticker.C:
		}
	}
}

func (c *RuntimeController) cpaSpec() ProcessSpec {
	snapshot := c.state.Snapshot()
	c.mu.Lock()
	cfg := c.cfg
	c.mu.Unlock()
	binary := strings.TrimSpace(snapshot.Components["cpa"].BinaryPath)
	if binary == "" {
		binary = cfg.CPABinary
	}
	return ProcessSpec{
		Name:    "cpa",
		Binary:  binary,
		Version: firstNonEmpty(snapshot.Components["cpa"].Version, cfg.CPAVersion),
		Args:    []string{"-config", cfg.CPAConfigPath},
		Dir:     filepath.Dir(cfg.CPAConfigPath),
	}
}
