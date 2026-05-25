import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { IconCheck, IconCrosshair, IconShield, IconTrash2 } from '@/components/ui/icons';
import { Input } from '@/components/ui/Input';
import { Select, type SelectOption } from '@/components/ui/Select';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { CodexInspectionModeTabs } from '@/features/monitoring/components/CodexInspectionModeTabs';
import { Panel } from '@/features/monitoring/components/CodexInspectionPanels';
import {
  formatActionLabel,
  formatPercent,
  formatTimestamp,
  type StatusTone,
} from '@/features/monitoring/model/codexInspectionPresentation';
import { usePanelFeatureAvailability } from '@/hooks/usePanelFeatureAvailability';
import {
  getUsageServiceErrorCode,
  usageServiceApi,
  type CodexInspectionLog,
  type CodexInspectionResult,
  type CodexInspectionRun,
  type CodexInspectionRunDetail,
  type ManagerCodexInspectionConfig,
  type ManagerCodexInspectionScheduleMode,
  type ManagerConfig,
} from '@/services/api/usageService';
import { useAuthStore, useNotificationStore } from '@/stores';
import styles from './CodexInspectionPage.module.scss';

type ServerCodexInspectionDraft = {
  enabled: boolean;
  scheduleMode: ManagerCodexInspectionScheduleMode;
  intervalMinutes: string;
  timePoints: string;
  timeZone: string;
  targetType: string;
  workers: string;
  deleteWorkers: string;
  timeout: string;
  retries: string;
  userAgent: string;
  usedPercentThreshold: string;
  sampleSize: string;
  autoActionMode: string;
};

type NormalizedServerCodexInspectionConfig = {
  enabled: boolean;
  schedule: {
    mode: ManagerCodexInspectionScheduleMode;
    intervalMinutes: number;
    timePoints: string[];
    timeZone: string;
  };
  targetType: string;
  workers: number;
  deleteWorkers: number;
  timeout: number;
  retries: number;
  userAgent: string;
  usedPercentThreshold: number;
  sampleSize: number;
  autoActionMode: string;
};

const DEFAULT_SERVER_CODEX_CONFIG: NormalizedServerCodexInspectionConfig = {
  enabled: false,
  schedule: {
    mode: 'interval',
    intervalMinutes: 60,
    timePoints: [],
    timeZone: '',
  },
  targetType: 'codex',
  workers: 4,
  deleteWorkers: 4,
  timeout: 15000,
  retries: 0,
  userAgent: 'codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal',
  usedPercentThreshold: 100,
  sampleSize: 0,
  autoActionMode: 'none',
};

const RUNS_LIMIT = 30;

const COMMON_TIME_ZONES: ReadonlyArray<string> = [
  'UTC',
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Singapore',
  'Asia/Hong_Kong',
  'Asia/Kolkata',
  'Europe/London',
  'Europe/Berlin',
  'Europe/Moscow',
  'America/New_York',
  'America/Los_Angeles',
];

const detectBrowserTimeZone = (): string => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || '';
  } catch {
    return '';
  }
};

const isScheduleMode = (value: unknown): value is ManagerCodexInspectionScheduleMode =>
  value === 'interval' || value === 'time_points';

const resolveServerCodexConfig = (
  config?: ManagerCodexInspectionConfig | null
): NormalizedServerCodexInspectionConfig => {
  const schedule = config?.schedule ?? {};
  const scheduleMode = isScheduleMode(schedule.mode)
    ? schedule.mode
    : schedule.timePoints && schedule.timePoints.length > 0
      ? 'time_points'
      : DEFAULT_SERVER_CODEX_CONFIG.schedule.mode;

  return {
    ...DEFAULT_SERVER_CODEX_CONFIG,
    ...config,
    enabled: config?.enabled ?? DEFAULT_SERVER_CODEX_CONFIG.enabled,
    schedule: {
      mode: scheduleMode,
      intervalMinutes:
        schedule.intervalMinutes && schedule.intervalMinutes > 0
          ? schedule.intervalMinutes
          : DEFAULT_SERVER_CODEX_CONFIG.schedule.intervalMinutes,
      timePoints: schedule.timePoints ?? DEFAULT_SERVER_CODEX_CONFIG.schedule.timePoints,
      timeZone: typeof schedule.timeZone === 'string' ? schedule.timeZone : DEFAULT_SERVER_CODEX_CONFIG.schedule.timeZone,
    },
    targetType: config?.targetType || DEFAULT_SERVER_CODEX_CONFIG.targetType,
    workers: config?.workers && config.workers > 0 ? config.workers : DEFAULT_SERVER_CODEX_CONFIG.workers,
    deleteWorkers:
      config?.deleteWorkers && config.deleteWorkers > 0
        ? config.deleteWorkers
        : DEFAULT_SERVER_CODEX_CONFIG.deleteWorkers,
    timeout: config?.timeout && config.timeout > 0 ? config.timeout : DEFAULT_SERVER_CODEX_CONFIG.timeout,
    retries:
      config?.retries !== undefined && config.retries >= 0
        ? config.retries
        : DEFAULT_SERVER_CODEX_CONFIG.retries,
    userAgent: config?.userAgent || DEFAULT_SERVER_CODEX_CONFIG.userAgent,
    usedPercentThreshold:
      config?.usedPercentThreshold !== undefined
        ? config.usedPercentThreshold
        : DEFAULT_SERVER_CODEX_CONFIG.usedPercentThreshold,
    sampleSize:
      config?.sampleSize !== undefined && config.sampleSize >= 0
        ? config.sampleSize
        : DEFAULT_SERVER_CODEX_CONFIG.sampleSize,
    autoActionMode: config?.autoActionMode || DEFAULT_SERVER_CODEX_CONFIG.autoActionMode,
  };
};

const toDraft = (config?: ManagerCodexInspectionConfig | null): ServerCodexInspectionDraft => {
  const resolved = resolveServerCodexConfig(config);
  return {
    enabled: resolved.enabled,
    scheduleMode: resolved.schedule.mode as ManagerCodexInspectionScheduleMode,
    intervalMinutes: String(resolved.schedule.intervalMinutes),
    timePoints: resolved.schedule.timePoints.join(', '),
    timeZone: resolved.schedule.timeZone,
    targetType: resolved.targetType,
    workers: String(resolved.workers),
    deleteWorkers: String(resolved.deleteWorkers),
    timeout: String(resolved.timeout),
    retries: String(resolved.retries),
    userAgent: resolved.userAgent,
    usedPercentThreshold: String(resolved.usedPercentThreshold),
    sampleSize: String(resolved.sampleSize),
    autoActionMode: resolved.autoActionMode,
  };
};

const normalizeTimePoint = (value: string): string | null => {
  const match = value.trim().match(/^(\d{1,2}):(\d{1,2})$/);
  if (!match) return null;
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  if (!Number.isInteger(hour) || !Number.isInteger(minute)) return null;
  if (hour < 0 || hour > 23 || minute < 0 || minute > 59) return null;
  return `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`;
};

const splitTimePointTokens = (raw: string): string[] =>
  raw
    .split(/[\s,;，；]+/)
    .map((value) => value.trim())
    .filter(Boolean);

const parseTimePoints = (raw: string): string[] =>
  Array.from(
    new Set(
      splitTimePointTokens(raw)
        .map(normalizeTimePoint)
        .filter((value): value is string => Boolean(value))
    )
  ).sort();

const normalizeTimePointList = (values: string[]): string[] =>
  Array.from(
    new Set(
      values
        .map(normalizeTimePoint)
        .filter((value): value is string => Boolean(value))
    )
  ).sort();

const readInteger = (raw: string, min: number): number | null => {
  const value = Number(raw);
  if (!Number.isInteger(value) || value < min) return null;
  return value;
};

const readPercent = (raw: string): number | null => {
  const value = Number(raw);
  if (!Number.isFinite(value) || value < 0 || value > 100) return null;
  return value;
};

const createConfigFromDraft = (
  draft: ServerCodexInspectionDraft
): ManagerCodexInspectionConfig | null => {
  const workers = readInteger(draft.workers, 1);
  const deleteWorkers = readInteger(draft.deleteWorkers, 1);
  const timeout = readInteger(draft.timeout, 1);
  const retries = readInteger(draft.retries, 0);
  const sampleSize = readInteger(draft.sampleSize, 0);
  const usedPercentThreshold = readPercent(draft.usedPercentThreshold);
  const parsedIntervalMinutes = readInteger(draft.intervalMinutes, 1);
  const intervalMinutes =
    parsedIntervalMinutes ?? DEFAULT_SERVER_CODEX_CONFIG.schedule.intervalMinutes;
  const hasInvalidTimePoint =
    draft.scheduleMode === 'time_points' &&
    splitTimePointTokens(draft.timePoints).some((value) => normalizeTimePoint(value) === null);
  const timePoints = parseTimePoints(draft.timePoints);

  if (
    workers === null ||
    deleteWorkers === null ||
    timeout === null ||
    retries === null ||
    sampleSize === null ||
    usedPercentThreshold === null ||
    (draft.scheduleMode === 'interval' && parsedIntervalMinutes === null) ||
    !draft.targetType.trim()
  ) {
    return null;
  }

  if (draft.scheduleMode === 'time_points' && (hasInvalidTimePoint || timePoints.length === 0)) {
    return null;
  }

  return {
    enabled: draft.enabled,
    schedule:
      draft.scheduleMode === 'time_points'
        ? {
            mode: 'time_points',
            timePoints,
            intervalMinutes,
            timeZone: draft.timeZone.trim(),
          }
        : {
            mode: 'interval',
            intervalMinutes,
            timePoints,
            timeZone: draft.timeZone.trim(),
          },
    targetType: draft.targetType.trim(),
    workers,
    deleteWorkers,
    timeout,
    retries,
    userAgent: draft.userAgent.trim(),
    usedPercentThreshold,
    sampleSize,
    autoActionMode: draft.autoActionMode,
  };
};

const statusToneClass: Record<StatusTone, string> = {
  idle: styles['tone-idle'],
  info: styles['tone-info'],
  good: styles['tone-good'],
  warn: styles['tone-warn'],
  bad: styles['tone-bad'],
};

const actionToneClass: Record<string, string> = {
  keep: styles.actionKeep,
  delete: styles.actionDelete,
  disable: styles.actionDisable,
  enable: styles.actionEnable,
};

const logLevelClass: Record<string, string> = {
  info: styles.logInfo,
  success: styles.logSuccess,
  warning: styles.logWarning,
  error: styles.logError,
};

function getRunTone(run?: CodexInspectionRun | null): StatusTone {
  switch (run?.status) {
    case 'completed':
      return 'good';
    case 'failed':
      return 'bad';
    case 'running':
      return 'info';
    default:
      return 'idle';
  }
}

function getRunStatusLabel(run: CodexInspectionRun | null | undefined, t: ReturnType<typeof useTranslation>['t']) {
  switch (run?.status) {
    case 'completed':
      return t('monitoring.codex_inspection_status_success');
    case 'failed':
      return t('monitoring.codex_inspection_status_error');
    case 'running':
      return t('monitoring.codex_inspection_status_running');
    default:
      return t('monitoring.codex_inspection_status_idle');
  }
}

function formatDuration(run: CodexInspectionRun | null | undefined, t: ReturnType<typeof useTranslation>['t']) {
  if (!run?.startedAtMs || !run.finishedAtMs) return t('common.not_set');
  const seconds = Math.max(0, Math.round((run.finishedAtMs - run.startedAtMs) / 1000));
  return t('monitoring.server_codex_inspection_duration_value', { seconds });
}

function formatTrigger(run: CodexInspectionRun | null | undefined, t: ReturnType<typeof useTranslation>['t']) {
  if (!run) return t('common.not_set');
  if (run.triggerType === 'scheduled') return t('monitoring.server_codex_inspection_trigger_scheduled');
  return t('monitoring.server_codex_inspection_trigger_manual');
}

function formatSchedule(config: NormalizedServerCodexInspectionConfig, t: ReturnType<typeof useTranslation>['t']) {
  if (config.schedule.mode === 'time_points') {
    const base = t('monitoring.server_codex_inspection_schedule_time_points_value', {
      points: config.schedule.timePoints.join(', '),
    });
    const tz = config.schedule.timeZone?.trim();
    return tz ? `${base} (${tz})` : base;
  }
  return t('monitoring.server_codex_inspection_schedule_interval_value', {
    minutes: config.schedule.intervalMinutes,
  });
}

function getComparableConfig(config: NormalizedServerCodexInspectionConfig) {
  return {
    enabled: config.enabled,
    scheduleMode: config.schedule.mode,
    intervalMinutes: config.schedule.intervalMinutes,
    timePoints: normalizeTimePointList(config.schedule.timePoints),
    timeZone: (config.schedule.timeZone || '').trim(),
    targetType: config.targetType.trim(),
    workers: config.workers,
    deleteWorkers: config.deleteWorkers,
    timeout: config.timeout,
    retries: config.retries,
    userAgent: config.userAgent.trim(),
    usedPercentThreshold: config.usedPercentThreshold,
    sampleSize: config.sampleSize,
    autoActionMode: config.autoActionMode,
  };
}

function configsEquivalent(
  current: NormalizedServerCodexInspectionConfig,
  next: NormalizedServerCodexInspectionConfig
) {
  return JSON.stringify(getComparableConfig(current)) === JSON.stringify(getComparableConfig(next));
}

function resolveActionLabel(action: string, t: ReturnType<typeof useTranslation>['t']) {
  if (action === 'delete' || action === 'disable' || action === 'enable' || action === 'keep') {
    return formatActionLabel(action, t);
  }
  return action || t('common.not_set');
}

function getUsageServiceDisplayError(error: unknown, t: ReturnType<typeof useTranslation>['t']) {
  const code = getUsageServiceErrorCode(error);
  if (code) {
    return t(`usage_service_errors.${code}`, {
      defaultValue: t('usage_service_errors.request_failed'),
    });
  }
  if (error instanceof Error && error.message) return error.message;
  return t('usage_service_errors.request_failed');
}

function formatServiceHost(base: string): string {
  if (!base) return '';
  try {
    const url = new URL(base);
    return url.host;
  } catch {
    return base;
  }
}

export function ServerCodexInspectionPage() {
  const { t, i18n } = useTranslation();
  const managementKey = useAuthStore((state) => state.managementKey);
  const featureAvailability = usePanelFeatureAvailability();
  const showNotification = useNotificationStore((state) => state.showNotification);
  const showConfirmation = useNotificationStore((state) => state.showConfirmation);

  const [serviceBase, setServiceBase] = useState('');
  const [managerConfig, setManagerConfig] = useState<ManagerConfig | null>(null);
  const [draft, setDraft] = useState<ServerCodexInspectionDraft>(() => toDraft(null));
  const [runs, setRuns] = useState<CodexInspectionRun[]>([]);
  const [detail, setDetail] = useState<CodexInspectionRunDetail | null>(null);
  const [selectedRunId, setSelectedRunId] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');
  const [logsCollapsed, setLogsCollapsed] = useState(false);
  const [resultFilter, setResultFilter] = useState<'all' | 'delete' | 'disable' | 'enable' | 'keep'>('all');
  const [logLevelFilter, setLogLevelFilter] = useState<'all' | 'info' | 'success' | 'warning' | 'error'>('all');
  const refreshInFlightRef = useRef(false);

  const loadRunDetail = useCallback(
    async (base: string, id: number) => {
      const nextDetail = await usageServiceApi.getCodexInspectionRun(base, managementKey, id);
      setDetail(nextDetail);
      setSelectedRunId(nextDetail.run.id);
      return nextDetail;
    },
    [managementKey]
  );

  const loadPageData = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const resolvedBase = featureAvailability.managerServiceBase;
      if (!resolvedBase || !featureAvailability.serverCodexInspectionAvailable) {
        throw new Error(t('monitoring.server_codex_inspection_service_unavailable'));
      }
      const response = await usageServiceApi.getManagerConfig(resolvedBase, managementKey);
      const responseConfig = response.config;

      setServiceBase(resolvedBase);
      setManagerConfig(responseConfig);
      setDraft(toDraft(responseConfig.codexInspection));

      const runsResponse = await usageServiceApi.listCodexInspectionRuns(
        resolvedBase,
        managementKey,
        RUNS_LIMIT
      );
      setRuns(runsResponse.items);
      const nextSelectedId = runsResponse.items[0]?.id;
      if (nextSelectedId) {
        await loadRunDetail(resolvedBase, nextSelectedId);
      } else {
        setDetail(null);
        setSelectedRunId(null);
      }
    } catch (error: unknown) {
      const message =
        error instanceof Error
          ? error.message
          : getUsageServiceDisplayError(error, t);
      setError(message);
      setRuns([]);
      setDetail(null);
      setSelectedRunId(null);
    } finally {
      setLoading(false);
    }
  }, [
    featureAvailability.managerServiceBase,
    featureAvailability.serverCodexInspectionAvailable,
    loadRunDetail,
    managementKey,
    t,
  ]);

  useEffect(() => {
    if (featureAvailability.checking) {
      return;
    }
    if (!managementKey || !featureAvailability.serverCodexInspectionAvailable) {
      setLoading(false);
      setError(t('monitoring.server_codex_inspection_connection_required'));
      return;
    }
    void loadPageData();
  }, [
    featureAvailability.checking,
    featureAvailability.serverCodexInspectionAvailable,
    loadPageData,
    managementKey,
    t,
  ]);

  const selectedConfig = useMemo(
    () => resolveServerCodexConfig(managerConfig?.codexInspection),
    [managerConfig?.codexInspection]
  );
  const draftConfig = useMemo(() => createConfigFromDraft(draft), [draft]);
  const normalizedDraftConfig = useMemo(
    () => (draftConfig ? resolveServerCodexConfig(draftConfig) : null),
    [draftConfig]
  );
  const hasUnsavedChanges = Boolean(
    managerConfig && (!normalizedDraftConfig || !configsEquivalent(selectedConfig, normalizedDraftConfig))
  );
  const savedScheduleLabel = formatSchedule(selectedConfig, t);
  const hasRunningRun = runs.some((run) => run.status === 'running') || detail?.run.status === 'running';
  const latestRun = runs[0] ?? null;
  const activeRun = detail?.run ?? latestRun;
  const activeTone = getRunTone(activeRun);
  const actionCounts = activeRun
    ? activeRun.deleteCount + activeRun.disableCount + activeRun.enableCount
    : 0;

  const scheduleOptions = useMemo(
    () => [
      { value: 'interval', label: t('monitoring.server_codex_inspection_schedule_interval') },
      { value: 'time_points', label: t('monitoring.server_codex_inspection_schedule_time_points') },
    ],
    [t]
  );

  const browserTimeZone = useMemo(detectBrowserTimeZone, []);
  const timeZoneOptions = useMemo(() => {
    const seen = new Set<string>();
    const options: SelectOption[] = [
      { value: '', label: t('monitoring.server_codex_inspection_time_zone_server_default') },
    ];
    const push = (value: string, label: string) => {
      if (!value || seen.has(value)) return;
      seen.add(value);
      options.push({ value, label });
    };
    if (browserTimeZone && browserTimeZone !== 'UTC') {
      push(
        browserTimeZone,
        t('monitoring.server_codex_inspection_time_zone_browser', { tz: browserTimeZone })
      );
    }
    COMMON_TIME_ZONES.forEach((zone) => push(zone, zone));
    if (draft.timeZone && !seen.has(draft.timeZone)) {
      push(draft.timeZone, draft.timeZone);
    }
    return options;
  }, [browserTimeZone, draft.timeZone, t]);

  const updateDraft = <K extends keyof ServerCodexInspectionDraft>(
    key: K,
    value: ServerCodexInspectionDraft[K]
  ) => {
    setDraft((previous) => ({ ...previous, [key]: value }));
  };

  const refreshRuns = useCallback(async (options?: { silent?: boolean }) => {
    if (refreshInFlightRef.current) return;
    refreshInFlightRef.current = true;
    const silent = options?.silent ?? false;
    if (!serviceBase) {
      try {
        await loadPageData();
      } finally {
        refreshInFlightRef.current = false;
      }
      return;
    }
    if (!silent) setLoading(true);
    setError('');
    try {
      const response = await usageServiceApi.listCodexInspectionRuns(
        serviceBase,
        managementKey,
        RUNS_LIMIT
      );
      setRuns(response.items);
      const nextSelectedId =
        selectedRunId && response.items.some((run) => run.id === selectedRunId)
          ? selectedRunId
          : response.items[0]?.id;
      if (nextSelectedId) {
        await loadRunDetail(serviceBase, nextSelectedId);
      } else {
        setDetail(null);
        setSelectedRunId(null);
      }
    } catch (error: unknown) {
      setError(getUsageServiceDisplayError(error, t));
    } finally {
      if (!silent) setLoading(false);
      refreshInFlightRef.current = false;
    }
  }, [loadPageData, loadRunDetail, managementKey, selectedRunId, serviceBase, t]);

  useEffect(() => {
    if (!serviceBase || (!selectedConfig.enabled && !hasRunningRun)) return;
    const timer = window.setInterval(() => {
      if (saving || running) return;
      void refreshRuns({ silent: true });
    }, 30_000);

    return () => window.clearInterval(timer);
  }, [hasRunningRun, refreshRuns, running, saving, selectedConfig.enabled, serviceBase]);

  const handleSave = async () => {
    if (!serviceBase || !managerConfig) {
      showNotification(t('monitoring.server_codex_inspection_service_unavailable'), 'warning');
      return;
    }
    const codexInspection = createConfigFromDraft(draft);
    if (!codexInspection) {
      showNotification(t('monitoring.server_codex_inspection_config_invalid'), 'warning');
      return;
    }
    setSaving(true);
    try {
      const response = await usageServiceApi.saveManagerConfig(
        serviceBase,
        {
          ...managerConfig,
          codexInspection,
        },
        managementKey
      );
      setManagerConfig(response.config);
      setDraft(toDraft(response.config.codexInspection));
      showNotification(t('monitoring.server_codex_inspection_config_saved'), 'success');
    } catch (error: unknown) {
      showNotification(
        `${t('notification.save_failed')}: ${getUsageServiceDisplayError(error, t)}`,
        'error'
      );
    } finally {
      setSaving(false);
    }
  };

  const executeServerRun = useCallback(async () => {
    if (!serviceBase) {
      showNotification(t('monitoring.server_codex_inspection_service_unavailable'), 'warning');
      return;
    }
    setRunning(true);
    setError('');
    try {
      const nextDetail = await usageServiceApi.runCodexInspection(serviceBase, managementKey);
      setDetail(nextDetail);
      setSelectedRunId(nextDetail.run.id);
      const response = await usageServiceApi.listCodexInspectionRuns(
        serviceBase,
        managementKey,
        RUNS_LIMIT
      );
      setRuns(response.items);
      showNotification(t('monitoring.server_codex_inspection_run_success'), 'success');
    } catch (error: unknown) {
      const message = getUsageServiceDisplayError(error, t);
      showNotification(`${t('monitoring.server_codex_inspection_run_failed')}: ${message}`, 'error');
      await refreshRuns();
    } finally {
      setRunning(false);
    }
  }, [managementKey, refreshRuns, serviceBase, showNotification, t]);

  const handleRunNow = () => {
    showConfirmation({
      title: t('monitoring.server_codex_inspection_run_confirm_title'),
      message: t('monitoring.server_codex_inspection_run_confirm_body'),
      confirmText: t('monitoring.server_codex_inspection_run_now'),
      cancelText: t('common.cancel'),
      variant: selectedConfig.autoActionMode === 'delete' ? 'danger' : 'primary',
      onConfirm: executeServerRun,
    });
  };

  const handleSelectRun = async (runID: number) => {
    if (!serviceBase || runID === selectedRunId) return;
    setSelectedRunId(runID);
    try {
      await loadRunDetail(serviceBase, runID);
    } catch (error: unknown) {
      showNotification(getUsageServiceDisplayError(error, t), 'error');
    }
  };

  const renderStatusPanel = () => {
    const lastRunTime = activeRun?.finishedAtMs
      ? new Date(activeRun.finishedAtMs).toLocaleTimeString(i18n.language)
      : '--';
    const durationLabel = formatDuration(activeRun, t);
    const serviceHost = formatServiceHost(serviceBase);
    const executionModeLabel = t('monitoring.codex_inspection_mode_server');
    return (
      <Panel
        title={t('monitoring.server_codex_inspection_title')}
        subtitle={t('monitoring.server_codex_inspection_desc')}
        className={styles.statusPanel}
        extra={
          <div className={styles.statusActions}>
            <Button variant="secondary" size="sm" onClick={() => void refreshRuns()} loading={loading}>
              {t('common.refresh')}
            </Button>
            <Button size="sm" onClick={handleRunNow} loading={running} disabled={!serviceBase || running}>
              {t('monitoring.server_codex_inspection_run_now')}
            </Button>
          </div>
        }
      >
        <div className={styles.statusBar}>
          <div className={styles.statusInfo}>
            <span className={`${styles.statusBadge} ${statusToneClass[activeTone]}`}>
              <span className={styles.statusDot} aria-hidden="true" />
              {getRunStatusLabel(activeRun, t)}
            </span>
            <span
              className={`${styles.statusBadge} ${
                selectedConfig.enabled ? statusToneClass.good : statusToneClass.idle
              }`}
            >
              <span className={styles.statusDot} aria-hidden="true" />
              {selectedConfig.enabled
                ? t('monitoring.server_codex_inspection_schedule_enabled')
                : t('monitoring.server_codex_inspection_schedule_disabled')}
            </span>
            <div className={styles.statusMeta}>
              <span>
                {t('monitoring.codex_inspection_execution_mode')}: {executionModeLabel}
              </span>
              <span>{savedScheduleLabel}</span>
              <span>
                {t('monitoring.server_codex_inspection_last_run')}: {lastRunTime}
                {activeRun?.finishedAtMs ? ` · ${durationLabel}` : ''}
              </span>
              {serviceHost ? <span title={serviceBase}>{serviceHost}</span> : null}
            </div>
          </div>
        </div>

        <details className={styles.infoNote}>
          <summary>{t('monitoring.server_codex_inspection_info_summary')}</summary>
          <ul className={styles.infoNoteList}>
            <li>
              <strong>{t('monitoring.server_codex_inspection_worker_poll')}:</strong>{' '}
              {t('monitoring.server_codex_inspection_effect_hint')}
            </li>
            <li>
              <strong>{t('monitoring.server_codex_inspection_time_basis')}:</strong>{' '}
              {t('monitoring.server_codex_inspection_server_time_hint')}
            </li>
            <li>
              <strong>{t('monitoring.server_codex_inspection_history_refresh')}:</strong>{' '}
              {t('monitoring.server_codex_inspection_auto_refresh_hint')}
            </li>
          </ul>
        </details>

        <div className={styles.summaryGrid}>
          <div className={styles.summaryCard}>
            <span className={styles.summaryLabel}>{t('monitoring.codex_inspection_total_accounts')}</span>
            <strong className={styles.summaryValue}>{activeRun?.probeSetCount ?? 0}</strong>
            <span className={styles.summaryMeta}>
              {t('monitoring.server_codex_inspection_total_files', {
                count: activeRun?.totalFiles ?? 0,
              })}
            </span>
          </div>
          <div className={styles.summaryCard}>
            <span className={styles.summaryLabel}>{t('monitoring.codex_inspection_sampled_accounts')}</span>
            <strong className={styles.summaryValue}>{activeRun?.sampledCount ?? 0}</strong>
            <span className={styles.summaryMeta}>{formatTrigger(activeRun, t)}</span>
          </div>
          <div className={`${styles.summaryCard} ${styles['tone-bad']}`}>
            <span className={styles.summaryLabel}>{t('monitoring.codex_inspection_delete_count')}</span>
            <strong className={styles.summaryValue}>{activeRun?.deleteCount ?? 0}</strong>
            <span className={styles.summaryMeta}>
              {t('monitoring.server_codex_inspection_action_total_value', { count: actionCounts })}
            </span>
          </div>
          <div className={`${styles.summaryCard} ${styles['tone-warn']}`}>
            <span className={styles.summaryLabel}>{t('monitoring.codex_inspection_disable_count')}</span>
            <strong className={styles.summaryValue}>{activeRun?.disableCount ?? 0}</strong>
            <span className={styles.summaryMeta}>
              {t('monitoring.codex_inspection_threshold')}: {selectedConfig.usedPercentThreshold}%
            </span>
          </div>
          <div className={`${styles.summaryCard} ${styles['tone-good']}`}>
            <span className={styles.summaryLabel}>{t('monitoring.codex_inspection_enable_count')}</span>
            <strong className={styles.summaryValue}>{activeRun?.enableCount ?? 0}</strong>
            <span className={styles.summaryMeta}>
              {t('monitoring.server_codex_inspection_keep_count', {
                count: activeRun?.keepCount ?? 0,
              })}
            </span>
          </div>
        </div>
      </Panel>
    );
  };

  const handleDiscard = () => {
    if (!managerConfig) return;
    setDraft(toDraft(managerConfig.codexInspection));
  };

  const renderConfigPanel = () => (
    <Panel
      title={t('monitoring.server_codex_inspection_config_title')}
      subtitle={t('monitoring.server_codex_inspection_config_desc')}
      extra={
        <div className={styles.serverConfigActions}>
          {hasUnsavedChanges ? (
            <span className={styles.serverUnsavedBadge}>
              {t('monitoring.server_codex_inspection_unsaved')}
            </span>
          ) : null}
          {hasUnsavedChanges ? (
            <Button variant="secondary" size="sm" onClick={handleDiscard} disabled={saving}>
              {t('monitoring.server_codex_inspection_discard')}
            </Button>
          ) : null}
          <Button
            size="sm"
            onClick={handleSave}
            loading={saving}
            disabled={loading || saving || !hasUnsavedChanges}
          >
            {t('monitoring.server_codex_inspection_save_apply')}
          </Button>
        </div>
      }
    >
      <div className={styles.serverConfigGrid}>
        <div className={`${styles.serverField} ${styles.serverFieldWide}`}>
          <ToggleSwitch
            checked={draft.enabled}
            onChange={(value) => updateDraft('enabled', value)}
            label={t('monitoring.server_codex_inspection_enable_schedule')}
          />
        </div>

        <div className={`${styles.serverField} ${styles.serverFieldWide}`}>
          <span className={styles.serverFieldLabel}>
            {t('monitoring.server_codex_inspection_schedule_mode')}
          </span>
          <div className={styles.scheduleSegmented} role="tablist" aria-label={t('monitoring.server_codex_inspection_schedule_mode')}>
            {scheduleOptions.map((opt) => {
              const active = draft.scheduleMode === opt.value;
              return (
                <button
                  key={opt.value}
                  type="button"
                  role="tab"
                  aria-selected={active}
                  className={`${styles.scheduleSegmentButton} ${active ? styles.scheduleSegmentButtonActive : ''}`}
                  onClick={() =>
                    updateDraft(
                      'scheduleMode',
                      isScheduleMode(opt.value)
                        ? opt.value
                        : DEFAULT_SERVER_CODEX_CONFIG.schedule.mode
                    )
                  }
                >
                  {opt.label}
                </button>
              );
            })}
          </div>
        </div>

        {draft.scheduleMode === 'interval' ? (
          <div className={styles.serverField}>
            <Input
              label={t('monitoring.server_codex_inspection_interval_minutes')}
              type="number"
              min="1"
              value={draft.intervalMinutes}
              onChange={(event) => updateDraft('intervalMinutes', event.target.value)}
            />
          </div>
        ) : (
          <>
            <div className={`${styles.serverField} ${styles.serverFieldHalf}`}>
              <Input
                label={t('monitoring.server_codex_inspection_time_points')}
                value={draft.timePoints}
                onChange={(event) => updateDraft('timePoints', event.target.value)}
                placeholder="09:00, 13:30, 22:00"
                hint={t('monitoring.server_codex_inspection_time_points_hint')}
              />
            </div>
            <div className={`${styles.serverField} ${styles.serverFieldHalf}`}>
              <span className={styles.serverFieldLabel}>
                {t('monitoring.server_codex_inspection_time_zone')}
              </span>
              <Select
                value={draft.timeZone}
                options={timeZoneOptions}
                onChange={(value) => updateDraft('timeZone', value)}
                ariaLabel={t('monitoring.server_codex_inspection_time_zone')}
              />
            </div>
          </>
        )}

        <div className={styles.serverField}>
          <Input
            label={t('monitoring.codex_inspection_settings_used_percent_threshold_label')}
            type="number"
            min="0"
            max="100"
            value={draft.usedPercentThreshold}
            onChange={(event) => updateDraft('usedPercentThreshold', event.target.value)}
          />
        </div>
        <div className={styles.serverField}>
          <Input
            label={t('monitoring.codex_inspection_settings_sample_size_label')}
            type="number"
            min="0"
            value={draft.sampleSize}
            onChange={(event) => updateDraft('sampleSize', event.target.value)}
          />
        </div>

        <div className={styles.autoActionField}>
          <span className={styles.serverFieldLabel}>
            {t('monitoring.codex_inspection_settings_auto_action_mode_label')}
          </span>
          <div className={styles.settingsAutoCards}>
            {(['none', 'disable', 'delete'] as const).map((mode) => {
              const active = draft.autoActionMode === mode;
              const toneClass =
                mode === 'delete'
                  ? styles.settingsAutoOptionDelete
                  : mode === 'disable'
                    ? styles.settingsAutoOptionDisable
                    : styles.settingsAutoOptionNone;
              const ModeIcon = mode === 'delete' ? IconTrash2 : mode === 'disable' ? IconShield : IconCrosshair;
              return (
                <button
                  key={mode}
                  type="button"
                  className={`${styles.settingsAutoOption} ${toneClass} ${active ? styles.settingsAutoOptionActive : ''}`}
                  onClick={() => updateDraft('autoActionMode', mode)}
                  aria-pressed={active}
                >
                  <span className={styles.settingsAutoOptionIcon}>
                    <ModeIcon size={28} />
                  </span>
                  <span className={styles.settingsAutoOptionText}>
                    <strong>{t(`monitoring.codex_inspection_settings_auto_action_mode_${mode}`)}</strong>
                    <small>{t(`monitoring.codex_inspection_settings_auto_action_mode_${mode}_desc`)}</small>
                  </span>
                  <span className={styles.settingsAutoOptionCheck}>
                    {active ? <IconCheck size={14} /> : null}
                  </span>
                </button>
              );
            })}
          </div>
          <p className={styles.settingsAutoHint}>
            {t('monitoring.codex_inspection_settings_auto_action_mode_hint')}
          </p>
          {draft.autoActionMode !== 'none' ? (
            <p
              className={`${styles.settingsAutoWarning} ${
                draft.autoActionMode === 'delete'
                  ? styles.settingsAutoWarningDelete
                  : styles.settingsAutoWarningDisable
              }`}
            >
              {draft.autoActionMode === 'delete'
                ? t('monitoring.codex_inspection_settings_auto_action_mode_delete_warning')
                : t('monitoring.codex_inspection_settings_auto_action_mode_disable_warning')}
            </p>
          ) : null}
        </div>
      </div>

      <details className={styles.advancedSection}>
        <summary>
          <span>{t('monitoring.server_codex_inspection_advanced_title')}</span>
          <span className={styles.advancedSummaryHint}>
            {t('monitoring.server_codex_inspection_advanced_hint')}
          </span>
        </summary>
        <div className={styles.advancedBody}>
          <div className={styles.serverField}>
            <Input
              label={t('monitoring.codex_inspection_settings_target_type_label')}
              value={draft.targetType}
              onChange={(event) => updateDraft('targetType', event.target.value)}
            />
          </div>
          <div className={styles.serverField}>
            <Input
              label={t('monitoring.codex_inspection_settings_workers_label')}
              type="number"
              min="1"
              value={draft.workers}
              onChange={(event) => updateDraft('workers', event.target.value)}
            />
          </div>
          <div className={styles.serverField}>
            <Input
              label={t('monitoring.codex_inspection_settings_delete_workers_label')}
              type="number"
              min="1"
              value={draft.deleteWorkers}
              onChange={(event) => updateDraft('deleteWorkers', event.target.value)}
            />
          </div>
          <div className={styles.serverField}>
            <Input
              label={t('monitoring.codex_inspection_settings_timeout_label')}
              type="number"
              min="1"
              value={draft.timeout}
              onChange={(event) => updateDraft('timeout', event.target.value)}
            />
          </div>
          <div className={styles.serverField}>
            <Input
              label={t('monitoring.codex_inspection_settings_retries_label')}
              type="number"
              min="0"
              value={draft.retries}
              onChange={(event) => updateDraft('retries', event.target.value)}
            />
          </div>
          <div className={`${styles.serverField} ${styles.serverFieldWide}`}>
            <Input
              label={t('monitoring.codex_inspection_settings_user_agent_label')}
              value={draft.userAgent}
              onChange={(event) => updateDraft('userAgent', event.target.value)}
            />
          </div>
        </div>
      </details>
    </Panel>
  );

  const renderRunsPanel = () => (
    <Panel
      title={t('monitoring.server_codex_inspection_history_title')}
      subtitle={t('monitoring.server_codex_inspection_history_desc')}
    >
      {runs.length > 0 ? (
        <div className={styles.runHistoryList} role="tablist" aria-label={t('monitoring.server_codex_inspection_history_title')}>
          {runs.map((run) => {
            const tone = getRunTone(run);
            const selected = run.id === selectedRunId;
            const ariaLabel = `${getRunStatusLabel(run, t)} · #${run.id} · ${formatTimestamp(run.startedAtMs, i18n.language)}`;
            return (
              <button
                type="button"
                key={run.id}
                role="tab"
                aria-selected={selected}
                aria-label={ariaLabel}
                className={`${styles.runHistoryCard} ${selected ? styles.runHistoryCardActive : ''}`}
                onClick={() => void handleSelectRun(run.id)}
              >
                <div className={styles.runHistoryCardHead}>
                  <span className={`${styles.statusBadge} ${statusToneClass[tone]}`}>
                    <span className={styles.statusDot} aria-hidden="true" />
                    {getRunStatusLabel(run, t)}
                  </span>
                  <span className={styles.runHistoryCardId}>#{run.id}</span>
                </div>
                <div className={styles.runHistoryCardMeta}>
                  <span>{formatTimestamp(run.startedAtMs, i18n.language)}</span>
                  <span>{formatTrigger(run, t)} · {t('monitoring.codex_inspection_sampled_accounts')}: {run.sampledCount}</span>
                </div>
                <div className={styles.runHistoryCardActionPills}>
                  {run.deleteCount > 0 ? (
                    <span className={`${styles.runHistoryCardPill} ${styles.runHistoryCardPillDelete}`}>
                      {t('monitoring.codex_inspection_action_delete')} {run.deleteCount}
                    </span>
                  ) : null}
                  {run.disableCount > 0 ? (
                    <span className={`${styles.runHistoryCardPill} ${styles.runHistoryCardPillDisable}`}>
                      {t('monitoring.codex_inspection_action_disable')} {run.disableCount}
                    </span>
                  ) : null}
                  {run.enableCount > 0 ? (
                    <span className={`${styles.runHistoryCardPill} ${styles.runHistoryCardPillEnable}`}>
                      {t('monitoring.codex_inspection_action_enable')} {run.enableCount}
                    </span>
                  ) : null}
                  {run.keepCount > 0 ? (
                    <span className={`${styles.runHistoryCardPill} ${styles.runHistoryCardPillKeep}`}>
                      {t('monitoring.codex_inspection_action_keep')} {run.keepCount}
                    </span>
                  ) : null}
                </div>
              </button>
            );
          })}
        </div>
      ) : (
        <div className={styles.emptyBlock}>{t('monitoring.server_codex_inspection_history_empty')}</div>
      )}
    </Panel>
  );

  const renderResultsPanel = (results: CodexInspectionResult[]) => {
    const counts: Record<'all' | 'delete' | 'disable' | 'enable' | 'keep', number> = {
      all: results.length,
      delete: 0,
      disable: 0,
      enable: 0,
      keep: 0,
    };
    for (const item of results) {
      if (item.action === 'delete' || item.action === 'disable' || item.action === 'enable' || item.action === 'keep') {
        counts[item.action] += 1;
      }
    }
    const filterOptions: ReadonlyArray<{ value: typeof resultFilter; label: string }> = [
      { value: 'all', label: t('monitoring.server_codex_inspection_filter_all') },
      { value: 'delete', label: t('monitoring.codex_inspection_action_delete') },
      { value: 'disable', label: t('monitoring.codex_inspection_action_disable') },
      { value: 'enable', label: t('monitoring.codex_inspection_action_enable') },
      { value: 'keep', label: t('monitoring.codex_inspection_action_keep') },
    ];
    const filtered = resultFilter === 'all' ? results : results.filter((item) => item.action === resultFilter);
    return (
      <Panel
        title={t('monitoring.codex_inspection_results_title')}
        subtitle={t('monitoring.server_codex_inspection_results_desc')}
        extra={
          results.length > 0 ? (
            <div className={styles.segmentedControl} role="tablist" aria-label={t('monitoring.codex_inspection_results_title')}>
              {filterOptions.map((opt) => {
                const active = resultFilter === opt.value;
                return (
                  <button
                    key={opt.value}
                    type="button"
                    role="tab"
                    aria-selected={active}
                    className={`${styles.segmentButton} ${active ? styles.segmentButtonActive : ''}`}
                    onClick={() => setResultFilter(opt.value)}
                  >
                    {opt.label}
                    <span className={styles.segmentCount}>{counts[opt.value]}</span>
                  </button>
                );
              })}
            </div>
          ) : undefined
        }
      >
        {filtered.length > 0 ? (
          <div className={styles.tableWrap}>
            <table className={styles.table}>
              <colgroup>
                <col className={styles.accountColumn} />
                <col className={styles.stateColumn} />
                <col className={styles.httpColumn} />
                <col className={styles.usageColumn} />
                <col className={styles.actionColumn} />
                <col className={styles.operationColumn} />
              </colgroup>
              <thead>
                <tr>
                  <th>{t('monitoring.account_label')}</th>
                  <th>{t('monitoring.codex_inspection_current_state')}</th>
                  <th>{t('monitoring.codex_inspection_http_status')}</th>
                  <th>{t('monitoring.codex_inspection_used_percent')}</th>
                  <th>{t('monitoring.codex_inspection_next_action')}</th>
                  <th>{t('monitoring.server_codex_inspection_results_state_detail')}</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((item) => (
                  <tr key={item.id || item.accountKey}>
                    <td>
                      <div className={styles.primaryCell}>
                        <span className={styles.primaryAccount}>{item.displayAccount}</span>
                        <small className={styles.primaryFile}>
                          {item.fileName}
                          {item.authIndex ? (
                            <span className={styles.primaryIndex}>{` · #${item.authIndex}`}</span>
                          ) : null}
                        </small>
                        {item.actionReason ? (
                          <small className={styles.primaryReason}>{item.actionReason}</small>
                        ) : null}
                      </div>
                    </td>
                    <td>
                      <span
                        className={`${styles.stateChip} ${
                          item.disabled ? styles.stateDisabled : styles.stateEnabled
                        }`}
                      >
                        {item.disabled
                          ? t('monitoring.codex_inspection_state_disabled')
                          : t('monitoring.codex_inspection_state_enabled')}
                      </span>
                    </td>
                    <td className={styles.monoCell}>{item.statusCode ?? '--'}</td>
                    <td className={styles.monoCell}>{formatPercent(item.usedPercent ?? null)}</td>
                    <td>
                      <span className={`${styles.actionBadge} ${actionToneClass[item.action] ?? styles.actionKeep}`}>
                        {resolveActionLabel(item.action, t)}
                      </span>
                    </td>
                    <td>
                      {item.error ? (
                        <span className={styles.primaryError}>{item.error}</span>
                      ) : (
                        <span className={styles.primaryReason}>{item.status || item.state || '--'}</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : results.length === 0 ? (
          <div className={styles.emptyAction}>
            <span>{t('monitoring.codex_inspection_empty')}</span>
            {serviceBase ? (
              <Button size="sm" onClick={handleRunNow} loading={running} disabled={running}>
                {t('monitoring.server_codex_inspection_run_now')}
              </Button>
            ) : null}
          </div>
        ) : (
          <div className={styles.emptyBlock}>{t('monitoring.server_codex_inspection_filter_no_match')}</div>
        )}
      </Panel>
    );
  };

  const handleCopyLogs = useCallback(
    async (logs: CodexInspectionLog[]) => {
      if (!logs.length) return;
      const lines = logs.map((entry) => {
        const ts = new Date(entry.createdAtMs).toISOString();
        const detail = entry.detail
          ? ` ${typeof entry.detail === 'string' ? entry.detail : JSON.stringify(entry.detail)}`
          : '';
        return `[${ts}] [${entry.level}] ${entry.message}${detail}`;
      });
      try {
        await navigator.clipboard.writeText(lines.join('\n'));
        showNotification(t('monitoring.server_codex_inspection_logs_copied'), 'success');
      } catch {
        showNotification(t('monitoring.server_codex_inspection_logs_copy_failed'), 'error');
      }
    },
    [showNotification, t]
  );

  const renderLogsPanel = (logs: CodexInspectionLog[]) => {
    const counts: Record<'all' | 'info' | 'success' | 'warning' | 'error', number> = {
      all: logs.length,
      info: 0,
      success: 0,
      warning: 0,
      error: 0,
    };
    for (const entry of logs) {
      if (entry.level === 'info' || entry.level === 'success' || entry.level === 'warning' || entry.level === 'error') {
        counts[entry.level] += 1;
      }
    }
    const filterOptions: ReadonlyArray<{ value: typeof logLevelFilter; label: string }> = [
      { value: 'all', label: t('monitoring.server_codex_inspection_filter_all') },
      { value: 'info', label: t('monitoring.server_codex_inspection_log_level_info') },
      { value: 'success', label: t('monitoring.server_codex_inspection_log_level_success') },
      { value: 'warning', label: t('monitoring.server_codex_inspection_log_level_warning') },
      { value: 'error', label: t('monitoring.server_codex_inspection_log_level_error') },
    ];
    const filtered = logLevelFilter === 'all' ? logs : logs.filter((entry) => entry.level === logLevelFilter);
    return (
      <Panel
        title={t('monitoring.codex_inspection_logs_title')}
        subtitle={t('monitoring.server_codex_inspection_logs_desc')}
        extra={
          <div className={styles.logToolbar}>
            {logs.length > 0 ? (
              <div className={styles.logFilterGroup} role="tablist" aria-label={t('monitoring.codex_inspection_logs_title')}>
                <div className={styles.segmentedControl}>
                  {filterOptions.map((opt) => {
                    const active = logLevelFilter === opt.value;
                    return (
                      <button
                        key={opt.value}
                        type="button"
                        role="tab"
                        aria-selected={active}
                        className={`${styles.segmentButton} ${active ? styles.segmentButtonActive : ''}`}
                        onClick={() => setLogLevelFilter(opt.value)}
                      >
                        {opt.label}
                        <span className={styles.segmentCount}>{counts[opt.value]}</span>
                      </button>
                    );
                  })}
                </div>
              </div>
            ) : <span />}
            <div className={styles.logToolbarRight}>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => void handleCopyLogs(logs)}
                disabled={logs.length === 0}
                aria-label={t('monitoring.server_codex_inspection_logs_copy')}
              >
                {t('monitoring.server_codex_inspection_logs_copy')}
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setLogsCollapsed((previous) => !previous)}
                disabled={logs.length === 0}
              >
                {logsCollapsed
                  ? t('monitoring.codex_inspection_expand_logs')
                  : t('monitoring.codex_inspection_fold_logs')}
              </Button>
            </div>
          </div>
        }
      >
        {!logsCollapsed ? (
          <div className={styles.logList}>
            {filtered.length > 0 ? (
              filtered.map((entry) => (
                <div
                  key={entry.id}
                  className={`${styles.logRow} ${logLevelClass[entry.level] ?? styles.logInfo}`}
                >
                  <span className={styles.logTime}>{formatTimestamp(entry.createdAtMs, i18n.language)}</span>
                  <span className={styles.logMessage}>
                    {entry.message}
                    {entry.detail ? (
                      <small className={styles.serverLogDetail}>
                        {typeof entry.detail === 'string'
                          ? entry.detail
                          : JSON.stringify(entry.detail)}
                      </small>
                    ) : null}
                  </span>
                </div>
              ))
            ) : (
              <div className={styles.emptyBlockSmall}>{t('monitoring.codex_inspection_logs_empty')}</div>
            )}
          </div>
        ) : (
          <div className={styles.logCollapsedBar}>
            <span>{t('monitoring.codex_inspection_logs_collapsed', { count: logs.length })}</span>
          </div>
        )}
      </Panel>
    );
  };

  return (
    <div className={styles.page}>
      <CodexInspectionModeTabs activeMode="server" />

      {error ? (
        <div className={styles.topErrorBar} role="alert" aria-live="polite">
          <span>{error}</span>
          <div className={styles.topErrorActions}>
            <Button variant="secondary" size="sm" onClick={() => void refreshRuns()} loading={loading}>
              {t('common.retry')}
            </Button>
          </div>
        </div>
      ) : null}
      {renderStatusPanel()}
      {renderConfigPanel()}
      <div className={styles.serverDetailGrid}>
        {renderRunsPanel()}
        <div className={styles.serverDetailPanels}>
          {detail?.run.error ? <div className={styles.serverError} role="alert">{detail.run.error}</div> : null}
          {renderResultsPanel(detail?.results ?? [])}
          {renderLogsPanel(detail?.logs ?? [])}
        </div>
      </div>
    </div>
  );
}
