package collector

import (
	"context"
	"time"

	collectorpkg "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/collector"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/store"
)

type Service struct {
	manager *collectorpkg.Manager
}

func New(manager *collectorpkg.Manager) *Service {
	return &Service{manager: manager}
}

func (s *Service) Start(ctx context.Context, cfg store.ManagerConfig) {
	s.manager.Start(ctx, RuntimeConfigFromManagerConfig(cfg))
}

func (s *Service) StartRuntime(ctx context.Context, cfg collectorpkg.RuntimeConfig) {
	s.manager.Start(ctx, cfg)
}

func (s *Service) Stop(ctx context.Context) {
	_ = ctx
	s.manager.Stop()
}

func (s *Service) Restart(ctx context.Context, cfg store.ManagerConfig) {
	s.Stop(ctx)
	s.Start(ctx, cfg)
}

func (s *Service) Status() collectorpkg.Status {
	return s.manager.Status()
}

func RuntimeConfigFromManagerConfig(cfg store.ManagerConfig) collectorpkg.RuntimeConfig {
	return collectorpkg.RuntimeConfig{
		CPAUpstreamURL: cfg.CPAConnection.CPABaseURL,
		ManagementKey:  cfg.CPAConnection.ManagementKey,
		CollectorMode:  cfg.Collector.CollectorMode,
		Queue:          cfg.Collector.Queue,
		PopSide:        cfg.Collector.PopSide,
		BatchSize:      cfg.Collector.BatchSize,
		PollInterval:   time.Duration(cfg.Collector.PollIntervalMS) * time.Millisecond,
		TLSSkipVerify:  cfg.Collector.TLSSkipVerify,
	}
}

func RuntimeConfigFromManagerConfigWithFallback(managerCfg store.ManagerConfig, base config.Config) collectorpkg.RuntimeConfig {
	pollInterval := time.Duration(managerCfg.Collector.PollIntervalMS) * time.Millisecond
	if pollInterval <= 0 {
		pollInterval = base.PollInterval
	}
	batchSize := managerCfg.Collector.BatchSize
	if batchSize <= 0 {
		batchSize = base.BatchSize
	}
	return collectorpkg.RuntimeConfig{
		CPAUpstreamURL: managerCfg.CPAConnection.CPABaseURL,
		ManagementKey:  managerCfg.CPAConnection.ManagementKey,
		CollectorMode:  valueOr(managerCfg.Collector.CollectorMode, base.CollectorMode),
		Queue:          valueOr(managerCfg.Collector.Queue, base.Queue),
		PopSide:        valueOr(managerCfg.Collector.PopSide, base.PopSide),
		BatchSize:      batchSize,
		PollInterval:   pollInterval,
		TLSSkipVerify:  managerCfg.Collector.TLSSkipVerify,
	}
}

func ManagerCollectorEnabled(managerCfg store.ManagerConfig) bool {
	return managerCfg.Collector.Enabled == nil || *managerCfg.Collector.Enabled
}

func valueOr(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
