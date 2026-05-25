import { useEffect, useMemo, useState } from 'react';
import {
  isUsageServiceId,
  normalizeUsageServiceBase,
  usageServiceApi,
  type ManagerConfig,
} from '@/services/api/usageService';
import { useAuthStore, useUsageServiceStore } from '@/stores';
import { detectApiBaseFromLocation } from '@/utils/connection';

export type PanelHostMode = 'manager_embedded' | 'external_panel';

export type PanelFeatureUnavailableReason =
  | 'checking'
  | 'service_not_configured'
  | 'service_unavailable'
  | 'monitoring_disabled';

export interface PanelFeatureAvailability {
  checking: boolean;
  panelHostMode: PanelHostMode;
  panelBase: string;
  managerServiceBase: string;
  managerServiceAvailable: boolean;
  requestMonitoringAvailable: boolean;
  modelPricesAvailable: boolean;
  serverCodexInspectionAvailable: boolean;
  dockerSetupAvailable: boolean;
  externalManagerConfigAvailable: boolean;
  reason: PanelFeatureUnavailableReason | '';
}

export interface ResolvePanelFeatureAvailabilityInput {
  checking?: boolean;
  panelHostedByUsageService: boolean;
  panelBase: string;
  managerServiceBase: string;
  managerConfig: ManagerConfig | null;
  hasManagerCandidate: boolean;
  managementKey: string;
}

const normalizeBase = (value?: string) => normalizeUsageServiceBase(value || '');

const buildUnavailableState = (
  input: ResolvePanelFeatureAvailabilityInput,
  reason: PanelFeatureUnavailableReason
): PanelFeatureAvailability => ({
  checking: input.checking === true,
  panelHostMode: input.panelHostedByUsageService ? 'manager_embedded' : 'external_panel',
  panelBase: normalizeBase(input.panelBase),
  managerServiceBase: '',
  managerServiceAvailable: false,
  requestMonitoringAvailable: false,
  modelPricesAvailable: false,
  serverCodexInspectionAvailable: false,
  dockerSetupAvailable: input.panelHostedByUsageService,
  externalManagerConfigAvailable: !input.panelHostedByUsageService,
  reason,
});

export function resolvePanelFeatureAvailability(
  input: ResolvePanelFeatureAvailabilityInput
): PanelFeatureAvailability {
  if (!input.managementKey) {
    return buildUnavailableState(input, 'service_not_configured');
  }

  const managerServiceBase = normalizeBase(input.managerServiceBase);
  if (!managerServiceBase || !input.managerConfig) {
    return buildUnavailableState(
      input,
      input.hasManagerCandidate ? 'service_unavailable' : 'service_not_configured'
    );
  }

  const hasCPAConnection = Boolean(
    input.managerConfig.cpaConnection?.cpaBaseUrl &&
      input.managerConfig.cpaConnection?.managementKey
  );
  const collectorEnabled = input.managerConfig.collector?.enabled !== false;
  const requestMonitoringAvailable = hasCPAConnection && collectorEnabled;

  return {
    checking: input.checking === true,
    panelHostMode: input.panelHostedByUsageService ? 'manager_embedded' : 'external_panel',
    panelBase: normalizeBase(input.panelBase),
    managerServiceBase,
    managerServiceAvailable: true,
    requestMonitoringAvailable,
    modelPricesAvailable: true,
    serverCodexInspectionAvailable: true,
    dockerSetupAvailable: input.panelHostedByUsageService,
    externalManagerConfigAvailable: !input.panelHostedByUsageService,
    reason: requestMonitoringAvailable
      ? ''
      : !hasCPAConnection
        ? 'service_not_configured'
        : 'monitoring_disabled',
  };
}

export interface BuildPanelManagerServiceCandidatesInput {
  panelHostedByUsageService: boolean;
  panelBase: string;
  apiBase: string;
  usageServiceEnabled: boolean;
  usageServiceBase: string;
  storedPanelBase?: string;
  storedPanelHostMode?: PanelHostMode | '';
}

export function buildPanelManagerServiceCandidates({
  panelHostedByUsageService,
  panelBase,
  apiBase,
  usageServiceEnabled,
  usageServiceBase,
  storedPanelBase,
  storedPanelHostMode,
}: BuildPanelManagerServiceCandidatesInput): string[] {
  const normalizedPanelBase = normalizeBase(panelBase);
  if (panelHostedByUsageService) {
    return normalizedPanelBase ? [normalizedPanelBase] : [];
  }

  const candidates: string[] = [];
  const normalizedStoredPanelBase = normalizeBase(storedPanelBase);
  const storedConfigMatchesPanel =
    !normalizedStoredPanelBase ||
    normalizedStoredPanelBase === normalizedPanelBase ||
    storedPanelHostMode === 'external_panel';

  if (usageServiceEnabled && storedConfigMatchesPanel) {
    const normalizedUsageServiceBase = normalizeBase(usageServiceBase);
    if (normalizedUsageServiceBase) {
      candidates.push(normalizedUsageServiceBase);
    }
  }

  const normalizedApiBase = normalizeBase(apiBase);
  if (normalizedApiBase && normalizedApiBase !== normalizedPanelBase) {
    candidates.push(normalizedApiBase);
  }

  return Array.from(new Set(candidates));
}

export function managerConfigMatchesPanel({
  panelHostedByUsageService,
  apiBase,
  config,
}: {
  panelHostedByUsageService: boolean;
  apiBase: string;
  config: ManagerConfig;
}): boolean {
  if (panelHostedByUsageService) {
    return true;
  }
  if (config.externalUsageService?.enabled !== true) {
    return false;
  }
  const configuredCPA = normalizeBase(config.cpaConnection?.cpaBaseUrl);
  const currentCPA = normalizeBase(apiBase);
  return Boolean(configuredCPA && currentCPA && configuredCPA === currentCPA);
}

const initialAvailability: PanelFeatureAvailability = {
  checking: true,
  panelHostMode: 'external_panel',
  panelBase: '',
  managerServiceBase: '',
  managerServiceAvailable: false,
  requestMonitoringAvailable: false,
  modelPricesAvailable: false,
  serverCodexInspectionAvailable: false,
  dockerSetupAvailable: false,
  externalManagerConfigAvailable: true,
  reason: 'checking',
};

export function usePanelFeatureAvailability(): PanelFeatureAvailability {
  const apiBase = useAuthStore((state) => state.apiBase);
  const managementKey = useAuthStore((state) => state.managementKey);
  const usageServiceEnabled = useUsageServiceStore((state) => state.enabled);
  const usageServiceBase = useUsageServiceStore((state) => state.serviceBase);
  const usageServiceRevision = useUsageServiceStore((state) => state.revision);
  const storedPanelBase = useUsageServiceStore((state) => state.panelBase);
  const storedPanelHostMode = useUsageServiceStore((state) => state.panelHostMode);
  const panelBase = useMemo(() => detectApiBaseFromLocation(), []);
  const [state, setState] = useState<PanelFeatureAvailability>(initialAvailability);

  useEffect(() => {
    let cancelled = false;

    const detect = async () => {
      const normalizedPanelBase = normalizeBase(panelBase);
      if (!managementKey) {
        setState(
          resolvePanelFeatureAvailability({
            checking: false,
            panelHostedByUsageService: false,
            panelBase: normalizedPanelBase,
            managerServiceBase: '',
            managerConfig: null,
            hasManagerCandidate: false,
            managementKey,
          })
        );
        return;
      }

      setState((current) => ({
        ...current,
        checking: true,
        panelBase: normalizedPanelBase,
        reason: 'checking',
      }));

      let panelHostedByUsageService = false;
      try {
        const info = await usageServiceApi.getInfo(normalizedPanelBase);
        panelHostedByUsageService = isUsageServiceId(info.service);
      } catch {
        panelHostedByUsageService = false;
      }

      const candidates = buildPanelManagerServiceCandidates({
        panelHostedByUsageService,
        panelBase: normalizedPanelBase,
        apiBase,
        usageServiceEnabled,
        usageServiceBase,
        storedPanelBase,
        storedPanelHostMode,
      });

      for (const candidate of candidates) {
        try {
          const info = await usageServiceApi.getInfo(candidate);
          if (!isUsageServiceId(info.service)) continue;
          const response = await usageServiceApi.getManagerConfig(candidate, managementKey);
          if (
            !managerConfigMatchesPanel({
              panelHostedByUsageService,
              apiBase,
              config: response.config,
            })
          ) {
            continue;
          }
          if (cancelled) return;
          setState(
            resolvePanelFeatureAvailability({
              checking: false,
              panelHostedByUsageService,
              panelBase: normalizedPanelBase,
              managerServiceBase: candidate,
              managerConfig: response.config,
              hasManagerCandidate: candidates.length > 0,
              managementKey,
            })
          );
          return;
        } catch {
          // Continue probing; a regular CPA endpoint or unreachable Manager Server is expected here.
        }
      }

      if (cancelled) return;
      setState(
        resolvePanelFeatureAvailability({
          checking: false,
          panelHostedByUsageService,
          panelBase: normalizedPanelBase,
          managerServiceBase: '',
          managerConfig: null,
          hasManagerCandidate: candidates.length > 0,
          managementKey,
        })
      );
    };

    void detect();

    return () => {
      cancelled = true;
    };
  }, [
    apiBase,
    managementKey,
    panelBase,
    storedPanelBase,
    storedPanelHostMode,
    usageServiceBase,
    usageServiceEnabled,
    usageServiceRevision,
  ]);

  return state;
}
