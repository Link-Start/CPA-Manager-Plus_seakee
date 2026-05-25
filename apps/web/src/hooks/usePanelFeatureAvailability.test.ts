import { describe, expect, it } from 'vitest';
import type { ManagerConfig } from '@/services/api/usageService';
import {
  buildPanelManagerServiceCandidates,
  managerConfigMatchesPanel,
  resolvePanelFeatureAvailability,
} from './usePanelFeatureAvailability';

const buildManagerConfig = (overrides: Partial<ManagerConfig> = {}): ManagerConfig => ({
  cpaConnection: {
    cpaBaseUrl: 'http://cpa.local:8317',
    managementKey: 'management-key',
  },
  collector: {
    enabled: true,
    collectorMode: 'auto',
    queue: 'usage',
    popSide: 'right',
    batchSize: 100,
    pollIntervalMs: 500,
    queryLimit: 50000,
  },
  externalUsageService: {
    enabled: true,
    serviceBase: 'http://manager.local:18317',
  },
  ...overrides,
});

describe('panel feature availability', () => {
  it('uses the current embedded Manager Server as the only Docker-mode candidate', () => {
    expect(
      buildPanelManagerServiceCandidates({
        panelHostedByUsageService: true,
        panelBase: 'http://manager.local:18317',
        apiBase: 'http://cpa.local:8317',
        usageServiceEnabled: true,
        usageServiceBase: 'http://old-manager.local:18317',
      })
    ).toEqual(['http://manager.local:18317']);
  });

  it('requires external Manager Server config to match the current CPA panel', () => {
    expect(
      managerConfigMatchesPanel({
        panelHostedByUsageService: false,
        apiBase: 'http://cpa.local:8317',
        config: buildManagerConfig(),
      })
    ).toBe(true);

    expect(
      managerConfigMatchesPanel({
        panelHostedByUsageService: false,
        apiBase: 'http://other-cpa.local:8317',
        config: buildManagerConfig(),
      })
    ).toBe(false);

    expect(
      managerConfigMatchesPanel({
        panelHostedByUsageService: false,
        apiBase: 'http://cpa.local:8317',
        config: buildManagerConfig({
          externalUsageService: { enabled: false, serviceBase: '' },
        }),
      })
    ).toBe(false);
  });

  it('marks Manager-only features available while separately gating request monitoring', () => {
    const availability = resolvePanelFeatureAvailability({
      panelHostedByUsageService: false,
      panelBase: 'http://cpa.local:8317',
      managerServiceBase: 'http://manager.local:18317',
      managerConfig: buildManagerConfig({
        collector: {
          ...buildManagerConfig().collector,
          enabled: false,
        },
      }),
      hasManagerCandidate: true,
      managementKey: 'management-key',
    });

    expect(availability.managerServiceAvailable).toBe(true);
    expect(availability.modelPricesAvailable).toBe(true);
    expect(availability.serverCodexInspectionAvailable).toBe(true);
    expect(availability.requestMonitoringAvailable).toBe(false);
    expect(availability.reason).toBe('monitoring_disabled');
  });
});
