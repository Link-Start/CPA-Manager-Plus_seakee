package setup

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
	collectorservice "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/collector"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/cpa"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/service/managerconfig"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/store"
)

type Request struct {
	CPAUpstreamURL               string `json:"cpaBaseUrl"`
	ManagementKey                string `json:"managementKey"`
	CPAManagementKey             string `json:"cpaManagementKey"`
	CollectorMode                string `json:"collectorMode"`
	Queue                        string `json:"queue"`
	PopSide                      string `json:"popSide"`
	BatchSize                    int    `json:"batchSize"`
	PollIntervalMS               int    `json:"pollIntervalMs"`
	QueryLimit                   int    `json:"queryLimit"`
	TLSSkipVerify                bool   `json:"tlsSkipVerify"`
	EnsureUsageStatisticsEnabled *bool  `json:"ensureUsageStatisticsEnabled"`
	RequestMonitoringEnabled     *bool  `json:"requestMonitoringEnabled"`
}

type Result struct {
	OK       bool   `json:"ok"`
	Upstream string `json:"upstream"`
	NextStep string `json:"nextStep"`
}

type AdminKeyRequest struct {
	AdminKey string `json:"adminKey"`
}

type AdminKeyResult struct {
	OK       bool   `json:"ok"`
	NextStep string `json:"nextStep"`
}

type GeneratedAdminKeyResult struct {
	AdminKey string `json:"adminKey"`
}

type InfoResult struct {
	Service              string `json:"service"`
	Mode                 string `json:"mode"`
	StartedAt            int64  `json:"startedAt"`
	Configured           bool   `json:"configured"`
	AdminReady           bool   `json:"adminReady"`
	ProjectInitialized   bool   `json:"projectInitialized"`
	SetupRequired        bool   `json:"setupRequired"`
	MigrationStatus      string `json:"migrationStatus,omitempty"`
	DataKeyReady         bool   `json:"dataKeyReady"`
	HasHistoricalData    bool   `json:"hasHistoricalData"`
	DeploymentMode       string `json:"deploymentMode"`
	SetupStep            string `json:"setupStep"`
	BootstrapRequired    bool   `json:"bootstrapRequired"`
	BootstrapTokenHeader string `json:"bootstrapTokenHeader,omitempty"`
}

type Service struct {
	cfg                  config.Config
	store                *store.Store
	collector            *collectorservice.Service
	managerConfigService *managerconfig.Service
	startedAt            int64
	serviceID            string
	verifyAdminKey       func(model.AdminCredential, string) (bool, error)
	mutationMu           sync.Mutex
}

func New(cfg config.Config, store *store.Store, collector *collectorservice.Service, managerConfigService *managerconfig.Service, startedAt int64, serviceID string) *Service {
	return &Service{
		cfg:                  cfg,
		store:                store,
		collector:            collector,
		managerConfigService: managerConfigService,
		startedAt:            startedAt,
		serviceID:            serviceID,
		verifyAdminKey:       security.VerifyAdminKeyBounded,
	}
}

func (s *Service) Info(ctx context.Context) (InfoResult, error) {
	setup, ok, err := s.managerConfigService.ResolveSetup(ctx)
	if err != nil {
		return InfoResult{}, err
	}
	_, adminReady, err := s.store.LoadAdminCredential(ctx)
	if err != nil {
		return InfoResult{}, err
	}
	bootstrapState, bootstrapStateOK, err := s.store.LoadBootstrapState(ctx)
	if err != nil {
		return InfoResult{}, err
	}
	projectInitialized := ok && setup.CPAUpstreamURL != "" && setup.ManagementKey != ""
	if bootstrapStateOK && !projectInitialized {
		projectInitialized = bootstrapState.ProjectInitialized
	}
	deployment, deploymentOK, err := s.store.LoadDeploymentState(ctx)
	if err != nil {
		return InfoResult{}, err
	}
	if !deploymentOK {
		deployment = model.DefaultDeploymentState(model.NormalizeDeploymentMode(s.cfg.DeploymentMode), s.cfg.PanelBasePath, panelBasePathSource(s.cfg))
	}
	setupStep := model.DeriveSetupStep(projectInitialized, adminReady)
	return InfoResult{
		Service:              s.serviceID,
		Mode:                 "embedded",
		StartedAt:            s.startedAt,
		Configured:           projectInitialized,
		AdminReady:           adminReady,
		ProjectInitialized:   projectInitialized,
		SetupRequired:        setupStep != model.SetupStepComplete,
		MigrationStatus:      bootstrapState.Status,
		DataKeyReady:         bootstrapState.DataKeyReady,
		HasHistoricalData:    bootstrapState.HasHistoricalData,
		DeploymentMode:       string(deployment.Mode),
		SetupStep:            setupStep,
		BootstrapRequired:    !adminReady,
		BootstrapTokenHeader: bootstrapTokenHeader(adminReady),
	}, nil
}

func (s *Service) AuthorizeSetup(ctx context.Context, authorizationHeader string, bootstrapToken string) error {
	credential, adminReady, err := s.store.LoadAdminCredential(ctx)
	if err != nil {
		return err
	}
	if adminReady {
		verifyAdminKey := s.verifyAdminKey
		if verifyAdminKey == nil {
			verifyAdminKey = security.VerifyAdminKeyBounded
		}
		verified, verifyErr := verifyAdminKey(credential, security.ExtractBearerToken(authorizationHeader))
		if verifyErr != nil {
			if errors.Is(verifyErr, security.ErrAdminKeyVerificationBusy) {
				return adminVerificationBusyError(verifyErr)
			}
			return verifyErr
		}
		if verified {
			return nil
		}
		return invalidAdminAuthorizationError()
	}
	bootstrapToken = strings.TrimSpace(bootstrapToken)
	if bootstrapToken == "" {
		bootstrapToken = security.ExtractBearerToken(authorizationHeader)
	}
	bootstrapCredential, ok, err := s.store.LoadBootstrapCredential(ctx)
	if err != nil {
		return err
	}
	if !ok || bootstrapCredential.ConsumedAtMS != 0 {
		return bootstrapTokenUnavailableError()
	}
	if bootstrapCredential.ExpiresAtMS > 0 && time.Now().UnixMilli() >= bootstrapCredential.ExpiresAtMS {
		return bootstrapTokenExpiredError()
	}
	if !security.VerifyBootstrapToken(bootstrapCredential, bootstrapToken, time.Now()) {
		return invalidBootstrapTokenError()
	}
	return nil
}

func (s *Service) GenerateAdminKey() (GeneratedAdminKeyResult, error) {
	adminKey, err := security.GenerateAdminKey()
	if err != nil {
		return GeneratedAdminKeyResult{}, err
	}
	return GeneratedAdminKeyResult{AdminKey: adminKey}, nil
}

func (s *Service) InitializeAdmin(ctx context.Context, req AdminKeyRequest) (AdminKeyResult, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if _, ok, err := s.store.LoadAdminCredential(ctx); err != nil {
		return AdminKeyResult{}, err
	} else if ok {
		return AdminKeyResult{}, adminAlreadyInitializedError()
	}
	adminKey := strings.TrimSpace(req.AdminKey)
	if err := security.ValidateAdminKey(adminKey); err != nil {
		return AdminKeyResult{}, adminKeyValidationError(err)
	}
	credential, err := security.NewAdminCredential(adminKey, "setup")
	if err != nil {
		return AdminKeyResult{}, err
	}
	projectInitialized, err := s.projectInitialized(ctx)
	if err != nil {
		return AdminKeyResult{}, err
	}
	state, err := s.bootstrapState(ctx, projectInitialized, true)
	if err != nil {
		return AdminKeyResult{}, err
	}
	created, err := s.store.InitializeAdmin(ctx, credential, state)
	if err != nil {
		return AdminKeyResult{}, err
	}
	if !created {
		return AdminKeyResult{}, adminAlreadyInitializedError()
	}
	return AdminKeyResult{OK: true, NextStep: model.DeriveSetupStep(projectInitialized, true)}, nil
}

func (s *Service) Setup(ctx context.Context, req Request, _ string) (Result, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	req.CPAUpstreamURL = cpa.NormalizeBaseURL(req.CPAUpstreamURL)
	req.CPAManagementKey = strings.TrimSpace(req.CPAManagementKey)
	if req.CPAManagementKey == "" {
		req.CPAManagementKey = strings.TrimSpace(req.ManagementKey)
	}
	req.ManagementKey = req.CPAManagementKey
	req.CollectorMode = managerconfig.CollectorMode(req.CollectorMode)
	if req.Queue == "" {
		req.Queue = s.cfg.Queue
	}
	if req.PopSide == "" {
		req.PopSide = s.cfg.PopSide
	}
	req.PopSide = managerconfig.NormalizePopSide(req.PopSide, s.cfg.PopSide)
	req.BatchSize = managerconfig.PositiveOrDefault(req.BatchSize, s.cfg.BatchSize, 100)
	req.PollIntervalMS = managerconfig.PositiveOrDefault(req.PollIntervalMS, int(s.cfg.PollInterval/time.Millisecond), 500)
	req.QueryLimit = managerconfig.PositiveOrDefault(req.QueryLimit, s.cfg.QueryLimit, 50000)
	requestMonitoringEnabled := requestMonitoringEnabled(req)
	if req.CPAUpstreamURL == "" || req.ManagementKey == "" {
		return Result{}, cpaConnectionRequiredError()
	}
	managementAPIValidated := false
	if existing, source, ok, err := s.managerConfigService.ResolveSetupWithSource(ctx); err != nil {
		return Result{}, err
	} else if source == managerconfig.SourceEnv && setupDiffers(existing, req) {
		return Result{}, setupEnvironmentManagedError()
	} else if ok && existing.ManagementKey != "" && req.ManagementKey != existing.ManagementKey {
		if cpa.NormalizeBaseURL(existing.CPAUpstreamURL) != req.CPAUpstreamURL {
			return Result{}, existingManagementKeyError()
		}
		if err := cpa.ValidateManagementAPI(ctx, req.CPAUpstreamURL, req.ManagementKey); err != nil {
			return Result{}, cpaValidationError(err)
		}
		managementAPIValidated = true
	}
	if !managementAPIValidated {
		if err := cpa.ValidateManagementAPI(ctx, req.CPAUpstreamURL, req.ManagementKey); err != nil {
			return Result{}, cpaValidationError(err)
		}
	}
	managerCfg := s.managerConfigService.DefaultManagerConfig()
	if existingManagerCfg, _, ok, err := s.managerConfigService.ResolveManagerConfigWithSource(ctx); err != nil {
		return Result{}, err
	} else if ok {
		managerCfg = existingManagerCfg
	}
	managerCfg.CPAConnection.CPABaseURL = req.CPAUpstreamURL
	managerCfg.CPAConnection.ManagementKey = req.ManagementKey
	managerCfg.Collector.Enabled = managerconfig.BoolPtr(requestMonitoringEnabled)
	managerCfg.Collector.CollectorMode = req.CollectorMode
	managerCfg.Collector.Queue = req.Queue
	managerCfg.Collector.PopSide = req.PopSide
	managerCfg.Collector.BatchSize = req.BatchSize
	managerCfg.Collector.PollIntervalMS = req.PollIntervalMS
	managerCfg.Collector.QueryLimit = req.QueryLimit
	managerCfg.Collector.TLSSkipVerify = req.TLSSkipVerify
	if requestMonitoringEnabled {
		if err := cpa.ValidateCollectorConfig(
			ctx,
			managerCfg.CPAConnection.CPABaseURL,
			managerCfg.CPAConnection.ManagementKey,
			managerCfg.Collector.PollIntervalMS,
		); err != nil {
			return Result{}, collectorConfigValidationError(err)
		}
	}
	ensureUsageStatisticsEnabled := requestMonitoringEnabled
	if req.EnsureUsageStatisticsEnabled != nil {
		ensureUsageStatisticsEnabled = requestMonitoringEnabled && *req.EnsureUsageStatisticsEnabled
	}
	if ensureUsageStatisticsEnabled {
		if err := cpa.SetUsageStatisticsEnabled(ctx, req.CPAUpstreamURL, req.ManagementKey, true); err != nil {
			return Result{}, usageStatisticsEnableError(err)
		}
	}
	setup := store.Setup{
		CPAUpstreamURL: req.CPAUpstreamURL,
		ManagementKey:  req.ManagementKey,
		Queue:          req.Queue,
		PopSide:        req.PopSide,
	}
	adminReady, err := s.adminReady(ctx)
	if err != nil {
		return Result{}, err
	}
	state, err := s.bootstrapState(ctx, true, adminReady)
	if err != nil {
		return Result{}, err
	}
	if err := s.store.SaveSetupInitialization(ctx, setup, managerCfg, state); err != nil {
		return Result{}, err
	}
	if requestMonitoringEnabled {
		s.collector.Start(context.Background(), managerCfg)
	} else {
		s.collector.Stop(context.Background())
	}
	return Result{OK: true, Upstream: setup.CPAUpstreamURL, NextStep: model.DeriveSetupStep(true, adminReady)}, nil
}

func (s *Service) bootstrapState(ctx context.Context, projectInitialized bool, adminReady bool) (store.BootstrapState, error) {
	state, ok, err := s.store.LoadBootstrapState(ctx)
	if err != nil {
		return store.BootstrapState{}, err
	}
	if !ok {
		state = store.BootstrapState{Version: model.BootstrapStateVersion}
	} else if state.Version < model.BootstrapStateVersion {
		state.Version = model.BootstrapStateVersion
	}
	if deployment, deploymentOK, err := s.store.LoadDeploymentState(ctx); err != nil {
		return store.BootstrapState{}, err
	} else if deploymentOK {
		state.DeploymentMode = deployment.Mode
	}
	state.Status = model.DeriveBootstrapStatus(projectInitialized, adminReady, state.HasHistoricalData)
	state.SetupStep = model.DeriveSetupStep(projectInitialized, adminReady)
	state.BootstrapRequired = !adminReady
	state.CPAReady = projectInitialized
	state.AdminReady = adminReady
	state.ProjectInitialized = projectInitialized
	state.DataKeyReady = true
	return state, nil
}

func (s *Service) projectInitialized(ctx context.Context) (bool, error) {
	setup, ok, err := s.managerConfigService.ResolveSetup(ctx)
	if err != nil {
		return false, err
	}
	return ok && setup.CPAUpstreamURL != "" && setup.ManagementKey != "", nil
}

func (s *Service) adminReady(ctx context.Context) (bool, error) {
	_, ok, err := s.store.LoadAdminCredential(ctx)
	return ok, err
}

func panelBasePathSource(cfg config.Config) string {
	if cfg.PanelBasePathEnvSet {
		return model.PanelBasePathSourceEnvironment
	}
	if strings.TrimSpace(cfg.PanelBasePathSource) != "" {
		return strings.TrimSpace(cfg.PanelBasePathSource)
	}
	return model.PanelBasePathSourceDefault
}

func bootstrapTokenHeader(adminReady bool) string {
	if adminReady {
		return ""
	}
	return "X-CPAMP-Bootstrap-Token"
}

func setupDiffers(existing store.Setup, req Request) bool {
	return cpa.NormalizeBaseURL(existing.CPAUpstreamURL) != req.CPAUpstreamURL ||
		existing.ManagementKey != req.ManagementKey ||
		existing.Queue != req.Queue ||
		existing.PopSide != req.PopSide
}

func requestMonitoringEnabled(req Request) bool {
	if req.RequestMonitoringEnabled == nil {
		return true
	}
	return *req.RequestMonitoringEnabled
}
