package bootstrap

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/cpa"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/managerconfig"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/store"
)

type Result struct {
	GeneratedAdminKey       string
	GeneratedBootstrapToken string
	AdminCreated            bool
	BootstrapTokenCreated   bool
	DataKeyCreated          bool
	MigratedLegacy          bool
	HasHistoricalData       bool
	State                   store.BootstrapState
	Deployment              store.DeploymentState
}

func Run(ctx context.Context, cfg config.Config, st *store.Store, dataKeyCreated bool) (Result, error) {
	result := Result{DataKeyCreated: dataKeyCreated}
	deployment, err := ensureDeploymentState(ctx, cfg, st)
	if err != nil {
		return Result{}, err
	}
	result.Deployment = deployment

	adminCreated, adminReady, err := ensureAdminCredential(ctx, cfg, st)
	if err != nil {
		return Result{}, err
	}
	result.AdminCreated = adminCreated
	bootstrapTokenCreated, generatedBootstrapToken, err := ensureBootstrapCredential(ctx, cfg, st, adminReady)
	if err != nil {
		return Result{}, err
	}
	result.BootstrapTokenCreated = bootstrapTokenCreated
	result.GeneratedBootstrapToken = generatedBootstrapToken

	historical, err := st.HasHistoricalData(ctx)
	if err != nil {
		return Result{}, err
	}
	result.HasHistoricalData = historical

	previousState, stateFound, err := st.LoadBootstrapState(ctx)
	if err != nil {
		return Result{}, err
	}
	if !stateFound || !previousState.MigratedLegacy {
		migrated, err := migrateLegacyConfig(ctx, cfg, st)
		if err != nil {
			return Result{}, err
		}
		result.MigratedLegacy = migrated
	} else {
		result.MigratedLegacy = previousState.MigratedLegacy
	}

	projectInitialized, err := projectInitialized(ctx, cfg, st)
	if err != nil {
		return Result{}, err
	}
	state := store.BootstrapState{
		Version:            model.BootstrapStateVersion,
		Status:             model.DeriveBootstrapStatus(projectInitialized, adminReady, historical),
		SetupStep:          model.DeriveSetupStep(projectInitialized, adminReady),
		DeploymentMode:     deployment.Mode,
		BootstrapRequired:  !adminReady,
		CPAReady:           projectInitialized,
		AdminReady:         adminReady,
		ProjectInitialized: projectInitialized,
		DataKeyReady:       true,
		MigratedLegacy:     result.MigratedLegacy,
		HasHistoricalData:  historical,
	}
	if err := st.SaveBootstrapState(ctx, state); err != nil {
		return Result{}, err
	}
	state, ok, err := st.LoadBootstrapState(ctx)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, errors.New("bootstrap state was not found after it was saved")
	}
	result.State = state
	return result, nil
}

func ensureAdminCredential(ctx context.Context, cfg config.Config, st *store.Store) (bool, bool, error) {
	if _, ok, err := st.LoadAdminCredential(ctx); err != nil || ok {
		return false, ok, err
	}
	adminKey := strings.TrimSpace(cfg.AdminKey)
	if adminKey == "" {
		return false, false, nil
	}
	credential, err := security.NewAdminCredential(adminKey, "env")
	if err != nil {
		return false, false, err
	}
	if err := st.SaveAdminCredential(ctx, credential); err != nil {
		return false, false, err
	}
	return true, true, nil
}

func ensureBootstrapCredential(ctx context.Context, cfg config.Config, st *store.Store, adminReady bool) (bool, string, error) {
	credential, ok, err := st.LoadBootstrapCredential(ctx)
	if err != nil {
		return false, "", err
	}
	if adminReady {
		if ok && credential.ConsumedAtMS == 0 {
			credential.ConsumedAtMS = time.Now().UnixMilli()
			if err := st.SaveBootstrapCredential(ctx, credential); err != nil {
				return false, "", err
			}
		}
		return false, "", nil
	}
	token, err := security.GenerateBootstrapToken()
	if err != nil {
		return false, "", err
	}
	ttl := cfg.BootstrapTokenTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	credential, err = security.NewBootstrapCredential(token, ttl)
	if err != nil {
		return false, "", err
	}
	if err := st.SaveBootstrapCredential(ctx, credential); err != nil {
		return false, "", err
	}
	return true, token, nil
}

func ensureDeploymentState(ctx context.Context, cfg config.Config, st *store.Store) (store.DeploymentState, error) {
	if existing, ok, err := st.LoadDeploymentState(ctx); err != nil {
		return store.DeploymentState{}, err
	} else if ok && existing.SchemaVersion > 0 {
		source := strings.TrimSpace(cfg.PanelBasePathSource)
		var reconciled store.DeploymentState
		var changed bool
		if strings.EqualFold(source, model.PanelBasePathSourceRuntime) {
			reconciled, changed, err = reconcileRuntimeDeployment(existing, cfg)
			if err != nil {
				return store.DeploymentState{}, err
			}
		} else if cfg.PanelBasePathEnvSet {
			reconciled, changed, err = reconcileEnvironmentPanelBasePath(existing, cfg.PanelBasePath)
			if err != nil {
				return store.DeploymentState{}, err
			}
		} else if model.PanelBasePathEnvironmentManaged(existing.PanelBasePathSource) {
			reconciled = existing
			reconciled.PanelBasePathSource = model.PanelBasePathSourceDatabase
			changed = true
		}
		if changed {
			if err := st.SaveDeploymentState(ctx, reconciled); err != nil {
				return store.DeploymentState{}, err
			}
			reconciled, _, err = st.LoadDeploymentState(ctx)
			return reconciled, err
		}
		return existing, nil
	}
	source := strings.TrimSpace(cfg.PanelBasePathSource)
	if source == "" {
		source = model.PanelBasePathSourceDefault
	}
	if cfg.PanelBasePathEnvSet {
		source = model.PanelBasePathSourceEnvironment
	}
	state := model.DefaultDeploymentState(
		model.NormalizeDeploymentMode(cfg.DeploymentMode),
		strings.TrimSpace(cfg.PanelBasePath),
		source,
	)
	if state.PanelBasePath == "" {
		state.PanelBasePath = "/management.html"
	}
	if err := st.SaveDeploymentState(ctx, state); err != nil {
		return store.DeploymentState{}, err
	}
	state, _, err := st.LoadDeploymentState(ctx)
	return state, err
}

func reconcileEnvironmentPanelBasePath(existing store.DeploymentState, value string) (store.DeploymentState, bool, error) {
	normalized, err := model.NormalizePanelBasePath(value)
	if err != nil {
		return store.DeploymentState{}, false, err
	}
	changed := existing.PanelBasePath != normalized ||
		existing.PanelBasePathSource != model.PanelBasePathSourceEnvironment
	if !changed {
		return existing, false, nil
	}
	previous := existing.PanelBasePath
	existing.RetiredPanelPaths = slices.DeleteFunc(
		existing.RetiredPanelPaths,
		func(candidate string) bool { return candidate == normalized },
	)
	if previous != "" && previous != normalized && !slices.Contains(existing.RetiredPanelPaths, previous) {
		existing.RetiredPanelPaths = append(existing.RetiredPanelPaths, previous)
		if len(existing.RetiredPanelPaths) > 32 {
			existing.RetiredPanelPaths = existing.RetiredPanelPaths[len(existing.RetiredPanelPaths)-32:]
		}
	}
	existing.PanelBasePath = normalized
	existing.PanelBasePathSource = model.PanelBasePathSourceEnvironment
	return existing, true, nil
}

func reconcileRuntimeDeployment(existing store.DeploymentState, cfg config.Config) (store.DeploymentState, bool, error) {
	panelBasePath, err := model.NormalizePanelBasePath(cfg.PanelBasePath)
	if err != nil {
		return store.DeploymentState{}, false, err
	}
	runtimeState := model.DefaultDeploymentState(
		model.NormalizeDeploymentMode(cfg.DeploymentMode),
		panelBasePath,
		model.PanelBasePathSourceRuntime,
	)
	changed := existing.Mode != runtimeState.Mode ||
		existing.PanelBasePath != runtimeState.PanelBasePath ||
		existing.PanelBasePathSource != runtimeState.PanelBasePathSource ||
		existing.RuntimeManaged != runtimeState.RuntimeManaged ||
		existing.CPAMPUpdatesManaged != runtimeState.CPAMPUpdatesManaged ||
		existing.CPAUpdatesManaged != runtimeState.CPAUpdatesManaged
	if !changed {
		return existing, false, nil
	}
	existing.Mode = runtimeState.Mode
	existing.PanelBasePath = runtimeState.PanelBasePath
	existing.PanelBasePathSource = runtimeState.PanelBasePathSource
	existing.RuntimeManaged = runtimeState.RuntimeManaged
	existing.CPAMPUpdatesManaged = runtimeState.CPAMPUpdatesManaged
	existing.CPAUpdatesManaged = runtimeState.CPAUpdatesManaged
	return existing, true, nil
}

func migrateLegacyConfig(ctx context.Context, cfg config.Config, st *store.Store) (bool, error) {
	migrated := false
	managerCfg, managerOK, err := st.LoadManagerConfig(ctx)
	if err != nil {
		return false, err
	}
	setup, setupOK, err := st.LoadSetup(ctx)
	if err != nil {
		return false, err
	}
	if !managerOK && setupOK && setup.CPAUpstreamURL != "" && setup.ManagementKey != "" {
		managerCfg = managerConfigFromSetup(cfg, setup)
		if err := st.SaveManagerConfig(ctx, managerCfg); err != nil {
			return false, err
		}
		migrated = true
	} else if managerOK {
		if err := st.SaveManagerConfig(ctx, managerCfg); err != nil {
			return false, err
		}
		migrated = true
	}
	if setupOK && setup.CPAUpstreamURL != "" && setup.ManagementKey != "" {
		if err := st.SaveSetup(ctx, setup); err != nil {
			return false, err
		}
		migrated = true
	}
	return migrated, nil
}

func managerConfigFromSetup(cfg config.Config, setup store.Setup) store.ManagerConfig {
	pollIntervalMS := int(cfg.PollInterval / time.Millisecond)
	return store.ManagerConfig{
		CPAConnection: store.ManagerCPAConnectionConfig{
			CPABaseURL:    cpa.NormalizeBaseURL(setup.CPAUpstreamURL),
			ManagementKey: setup.ManagementKey,
		},
		Collector: store.ManagerCollectorConfig{
			Enabled:        managerconfig.BoolPtr(true),
			CollectorMode:  managerconfig.CollectorMode(cfg.CollectorMode),
			Queue:          managerconfig.ValueOr(setup.Queue, cfg.Queue),
			PopSide:        managerconfig.NormalizePopSide(setup.PopSide, cfg.PopSide),
			BatchSize:      managerconfig.PositiveOrDefault(cfg.BatchSize, 100, 100),
			PollIntervalMS: managerconfig.PositiveOrDefault(pollIntervalMS, 500, 500),
			QueryLimit:     managerconfig.PositiveOrDefault(cfg.QueryLimit, 50000, 50000),
			TLSSkipVerify:  cfg.TLSSkipVerify,
		},
	}
}

func projectInitialized(ctx context.Context, cfg config.Config, st *store.Store) (bool, error) {
	if cfg.CPAUpstreamURL != "" && cfg.ManagementKey != "" {
		return true, nil
	}
	if managerCfg, ok, err := st.LoadManagerConfig(ctx); err != nil {
		return false, err
	} else if ok && managerCfg.CPAConnection.CPABaseURL != "" && managerCfg.CPAConnection.ManagementKey != "" {
		return true, nil
	}
	if setup, ok, err := st.LoadSetup(ctx); err != nil {
		return false, err
	} else if ok && setup.CPAUpstreamURL != "" && setup.ManagementKey != "" {
		return true, nil
	}
	return false, nil
}
