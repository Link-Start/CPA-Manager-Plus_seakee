import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import {
  usageServiceApi,
  type UsageArchivePreview,
  type UsageArchiveRunStatus,
  type UsageArchiveRunSummary,
  type UsageMaintenanceStatus,
} from '@/services/api/usageService';
import { useAuthStore, useNotificationStore } from '@/stores';
import { usePanelFeatureAvailability } from '@/hooks/usePanelFeatureAvailability';
import { formatDateTime, formatFileSize } from '@/utils/format';
import styles from './UsageMaintenancePage.module.scss';

const toLocalDateTimeValue = (timestamp: number) => {
  const date = new Date(timestamp);
  const offset = date.getTimezoneOffset() * 60_000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
};

const isUnsupportedError = (error: unknown) => {
  const candidate = error as { status?: number } | null;
  return candidate?.status === 404 || candidate?.status === 405;
};

const terminalArchiveStatuses = new Set(['completed', 'cancelled']);
const staticManualArchiveStatuses = new Set(['archived', 'verified']);
const runIsActive = (run: UsageArchiveRunSummary) =>
  !terminalArchiveStatuses.has(run.status) &&
  (run.mode === 'retention' || !staticManualArchiveStatuses.has(run.status));
const activeRefreshIntervalMs = 5_000;
const archiveProgressStatuses = new Set(['archiving', 'verifying', 'deleting']);
const archiveStatusTranslationValues = new Set([
  'previewed',
  'archiving',
  'archived',
  'verifying',
  'verified',
  'deleting',
  'completed',
  'failed',
  'cancelled',
]);
const archiveModeTranslationValues = new Set(['manual', 'retention']);
const migrationStatusTranslationValues = new Set([
  'discovering',
  'pending',
  'running',
  'applying',
  'clearing',
  'completed',
  'failed',
]);
const aggregateStatusTranslationValues = new Set([
  'pending',
  'backfilling',
  'catching_up',
  'clearing',
  'ready',
  'failed',
]);

type OperationToken = {
  generation: number;
  controller: AbortController;
  serviceBase: string;
  managementKey?: string;
};

type ConfirmationToken = {
  generation: number;
  serviceBase: string;
  managementKey?: string;
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value);

const hasString = (value: Record<string, unknown>, key: string) => typeof value[key] === 'string';

const hasNumber = (value: Record<string, unknown>, key: string) => {
  const candidate = value[key];
  return typeof candidate === 'number' && Number.isFinite(candidate);
};

const hasBoolean = (value: Record<string, unknown>, key: string) => typeof value[key] === 'boolean';

const hasOptionalNumber = (value: Record<string, unknown>, key: string) =>
  value[key] === undefined || hasNumber(value, key);

const hasOptionalString = (value: Record<string, unknown>, key: string) =>
  value[key] === undefined || hasString(value, key);

const isUsageArchivePreview = (value: unknown): value is UsageArchivePreview =>
  isRecord(value) &&
  hasNumber(value, 'cutoff_timestamp_ms') &&
  hasNumber(value, 'target_event_id') &&
  hasNumber(value, 'event_count') &&
  hasNumber(value, 'estimated_bytes') &&
  hasOptionalNumber(value, 'min_timestamp_ms') &&
  hasOptionalNumber(value, 'max_timestamp_ms');

const isUsageArchiveRunSummary = (value: unknown): value is UsageArchiveRunSummary => {
  if (!isRecord(value)) return false;
  return (
    hasString(value, 'id') &&
    hasString(value, 'mode') &&
    hasString(value, 'status') &&
    hasOptionalString(value, 'resume_status') &&
    hasNumber(value, 'cutoff_timestamp_ms') &&
    hasNumber(value, 'target_event_id') &&
    hasNumber(value, 'event_count') &&
    hasNumber(value, 'estimated_bytes') &&
    hasNumber(value, 'last_archived_event_id') &&
    hasNumber(value, 'archived_event_count') &&
    hasNumber(value, 'archived_uncompressed_bytes') &&
    hasNumber(value, 'archived_compressed_bytes') &&
    hasNumber(value, 'last_deleted_event_id') &&
    hasNumber(value, 'deleted_event_count') &&
    hasNumber(value, 'created_at_ms') &&
    hasNumber(value, 'updated_at_ms') &&
    hasOptionalNumber(value, 'started_at_ms') &&
    hasOptionalNumber(value, 'archived_at_ms') &&
    hasOptionalNumber(value, 'verified_at_ms') &&
    hasOptionalNumber(value, 'delete_started_at_ms') &&
    hasOptionalNumber(value, 'completed_at_ms') &&
    hasBoolean(value, 'has_error')
  );
};

const isUsageArchiveList = (value: unknown): value is { runs: UsageArchiveRunSummary[] } =>
  isRecord(value) && Array.isArray(value.runs) && value.runs.every(isUsageArchiveRunSummary);

const isUsageMaintenanceLock = (value: unknown) =>
  isRecord(value) &&
  hasString(value, 'run_id') &&
  hasString(value, 'operation') &&
  hasNumber(value, 'acquired_at_ms') &&
  hasNumber(value, 'updated_at_ms');

const isUsageMaintenanceStatus = (value: unknown): value is UsageMaintenanceStatus => {
  if (!isRecord(value)) return false;
  const migration = value.migration;
  const aggregate = value.hourly_aggregate;
  const readiness = value.readiness;
  const storage = value.storage;
  if (!isRecord(migration) || !isRecord(aggregate) || !isRecord(readiness) || !isRecord(storage)) {
    return false;
  }
  if (value.active_run !== undefined && !isUsageArchiveRunSummary(value.active_run)) return false;
  if (value.active_lock !== undefined && !isUsageMaintenanceLock(value.active_lock)) return false;
  return (
    hasNumber(value, 'raw_event_count') &&
    hasNumber(value, 'raw_deleted_event_count') &&
    hasString(migration, 'name') &&
    hasString(migration, 'status') &&
    hasNumber(migration, 'last_event_id') &&
    hasNumber(migration, 'target_event_id') &&
    hasNumber(migration, 'processed_rows') &&
    hasNumber(migration, 'changed_rows') &&
    hasNumber(migration, 'updated_at_ms') &&
    hasString(aggregate, 'name') &&
    hasString(aggregate, 'status') &&
    hasNumber(aggregate, 'schema_version') &&
    hasNumber(aggregate, 'coverage_event_id') &&
    hasNumber(aggregate, 'target_event_id') &&
    hasNumber(aggregate, 'updated_at_ms') &&
    hasBoolean(readiness, 'migration_ready') &&
    hasBoolean(readiness, 'hourly_aggregate_ready') &&
    hasBoolean(readiness, 'archive_delete_enabled') &&
    hasNumber(storage, 'page_size') &&
    hasNumber(storage, 'page_count') &&
    hasNumber(storage, 'freelist_count') &&
    hasNumber(storage, 'reclaimable_bytes') &&
    hasNumber(storage, 'database_bytes') &&
    hasNumber(storage, 'wal_bytes') &&
    hasNumber(storage, 'shm_bytes') &&
    hasNumber(storage, 'total_bytes') &&
    value.compact_requires_stopped_server === true
  );
};

const statusAction = (status: UsageArchiveRunStatus): 'resume' | 'verify' | 'delete' | null => {
  if (
    status === 'previewed' ||
    status === 'archiving' ||
    status === 'verifying' ||
    status === 'deleting' ||
    status === 'failed'
  ) {
    return 'resume';
  }
  if (status === 'archived') return 'verify';
  if (status === 'verified') return 'delete';
  return null;
};

const actionIsDestructive = (run: UsageArchiveRunSummary, action: 'resume' | 'verify' | 'delete') =>
  action === 'delete' ||
  (action === 'resume' &&
    (run.status === 'deleting' || (run.status === 'failed' && run.resume_status === 'deleting')));

export function UsageMaintenancePage() {
  const { t, i18n } = useTranslation();
  const availability = usePanelFeatureAvailability();
  const managementKey = useAuthStore((state) => state.managementKey);
  const { showConfirmation, showNotification } = useNotificationStore();
  const serviceBase = availability.managerServiceBase;
  const [maintenance, setMaintenance] = useState<UsageMaintenanceStatus | null>(null);
  const [archives, setArchives] = useState<UsageArchiveRunSummary[]>([]);
  const [preview, setPreview] = useState<UsageArchivePreview | null>(null);
  const [cutoff, setCutoff] = useState(() =>
    toLocalDateTimeValue(Date.now() - 30 * 24 * 60 * 60 * 1000)
  );
  const [loading, setLoading] = useState(true);
  const [working, setWorking] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [unsupported, setUnsupported] = useState(false);
  const mountedRef = useRef(false);
  const loadControllerRef = useRef<AbortController | null>(null);
  const loadGenerationRef = useRef(0);
  const capabilityContextRef = useRef<{ serviceBase: string; managementKey?: string } | null>(null);
  const operationControllerRef = useRef<AbortController | null>(null);
  const operationGenerationRef = useRef(0);
  const operationContextRef = useRef({ serviceBase, managementKey });
  const contextGenerationRef = useRef(0);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      loadGenerationRef.current += 1;
      loadControllerRef.current?.abort();
      loadControllerRef.current = null;
      operationGenerationRef.current += 1;
      operationControllerRef.current?.abort();
      operationControllerRef.current = null;
      contextGenerationRef.current += 1;
    };
  }, []);

  const invalidateOperation = useCallback((resetWorking = false) => {
    operationGenerationRef.current += 1;
    operationControllerRef.current?.abort();
    operationControllerRef.current = null;
    if (resetWorking && mountedRef.current) setWorking(false);
  }, []);

  const beginWorking = useCallback(
    (capturedServiceBase: string, capturedManagementKey?: string): OperationToken | null => {
      if (!mountedRef.current) return null;
      const currentContext = operationContextRef.current;
      if (
        currentContext.serviceBase !== capturedServiceBase ||
        currentContext.managementKey !== capturedManagementKey
      ) {
        return null;
      }
      operationControllerRef.current?.abort();
      const controller = new AbortController();
      const generation = ++operationGenerationRef.current;
      operationControllerRef.current = controller;
      loadGenerationRef.current += 1;
      loadControllerRef.current?.abort();
      loadControllerRef.current = null;
      setWorking(true);
      return {
        generation,
        controller,
        serviceBase: capturedServiceBase,
        managementKey: capturedManagementKey,
      };
    },
    []
  );

  const confirmationIsCurrent = useCallback((confirmation: ConfirmationToken) => {
    const currentContext = operationContextRef.current;
    return (
      mountedRef.current &&
      confirmation.generation === contextGenerationRef.current &&
      confirmation.serviceBase === currentContext.serviceBase &&
      confirmation.managementKey === currentContext.managementKey
    );
  }, []);

  const operationIsCurrent = useCallback((operation: OperationToken) => {
    const currentContext = operationContextRef.current;
    return (
      mountedRef.current &&
      !operation.controller.signal.aborted &&
      operation.generation === operationGenerationRef.current &&
      operation.controller === operationControllerRef.current &&
      operation.serviceBase === currentContext.serviceBase &&
      operation.managementKey === currentContext.managementKey
    );
  }, []);

  const finishOperation = useCallback(
    (operation: OperationToken) => {
      if (!operationIsCurrent(operation)) return;
      operationControllerRef.current = null;
      setWorking(false);
    },
    [operationIsCurrent]
  );

  useLayoutEffect(() => {
    contextGenerationRef.current += 1;
    operationContextRef.current = { serviceBase, managementKey };
    capabilityContextRef.current = null;
    invalidateOperation(true);
    setMaintenance(null);
    setArchives([]);
    setPreview(null);
    setError(null);
    setUnsupported(false);
    setLoading(Boolean(serviceBase));
    return () => invalidateOperation(false);
  }, [invalidateOperation, managementKey, serviceBase]);

  const load = useCallback(
    async ({ background = false }: { background?: boolean } = {}) => {
      if (!mountedRef.current) return;
      if (!serviceBase) {
        if (!background) setLoading(false);
        return;
      }

      const generation = ++loadGenerationRef.current;
      loadControllerRef.current?.abort();
      const controller = new AbortController();
      loadControllerRef.current = controller;
      if (!background) {
        setLoading(true);
        setError(null);
      }
      try {
        const capabilityContext = capabilityContextRef.current;
        if (
          capabilityContext?.serviceBase !== serviceBase ||
          capabilityContext.managementKey !== managementKey
        ) {
          await usageServiceApi.probeUsageMaintenance(
            serviceBase,
            managementKey,
            controller.signal
          );
          if (controller.signal.aborted || generation !== loadGenerationRef.current) return;
          capabilityContextRef.current = { serviceBase, managementKey };
        }
        const [maintenanceResult, archiveResult] = await Promise.all([
          usageServiceApi.getUsageMaintenance(serviceBase, managementKey, controller.signal),
          usageServiceApi.listUsageArchives(serviceBase, managementKey, 20, controller.signal),
        ]);
        if (controller.signal.aborted || generation !== loadGenerationRef.current) return;
        if (!isUsageMaintenanceStatus(maintenanceResult) || !isUsageArchiveList(archiveResult)) {
          setUnsupported(true);
          setMaintenance(null);
          setArchives([]);
          setError(null);
          return;
        }
        setMaintenance(maintenanceResult);
        setArchives(archiveResult.runs ?? []);
        setUnsupported(false);
        setError(null);
      } catch (cause) {
        if (generation !== loadGenerationRef.current || controller.signal.aborted) return;
        controller.abort();
        if (isUnsupportedError(cause)) {
          setUnsupported(true);
          setMaintenance(null);
          setArchives([]);
        } else {
          setUnsupported(false);
          setError(cause instanceof Error ? cause.message : String(cause));
        }
      } finally {
        if (generation === loadGenerationRef.current) {
          loadControllerRef.current = null;
          if (!background) setLoading(false);
        }
      }
    },
    [managementKey, serviceBase]
  );

  useEffect(() => {
    setPreview(null);
    void load();
    return () => {
      loadGenerationRef.current += 1;
      loadControllerRef.current?.abort();
      loadControllerRef.current = null;
    };
  }, [load]);

  const shouldPollMaintenance = Boolean(
    maintenance?.active_lock ||
    (maintenance?.active_run &&
      (maintenance.active_run.mode === 'retention' ||
        archiveProgressStatuses.has(maintenance.active_run.status)))
  );
  useEffect(() => {
    if (!shouldPollMaintenance || working || !serviceBase) return;
    const timer = setInterval(() => {
      if (!loadControllerRef.current) void load({ background: true });
    }, activeRefreshIntervalMs);
    return () => clearInterval(timer);
  }, [load, serviceBase, shouldPollMaintenance, working]);

  const cutoffTimestamp = useMemo(() => new Date(cutoff).getTime(), [cutoff]);

  const handlePreview = async () => {
    if (!Number.isFinite(cutoffTimestamp) || cutoffTimestamp <= 0) {
      showNotification(
        t('usage_maintenance.invalid_cutoff', { defaultValue: 'Choose a valid cutoff time.' }),
        'warning'
      );
      return;
    }
    const operation = beginWorking(serviceBase, managementKey);
    if (!operation) return;
    try {
      const result = await usageServiceApi.previewUsageArchive(
        serviceBase,
        cutoffTimestamp,
        managementKey,
        operation.controller.signal
      );
      if (!operationIsCurrent(operation)) return;
      if (!isUsageArchivePreview(result)) {
        setPreview(null);
        setUnsupported(true);
        return;
      }
      setPreview(result);
    } catch (cause) {
      if (operationIsCurrent(operation)) {
        if (isUnsupportedError(cause)) setUnsupported(true);
        else showNotification(cause instanceof Error ? cause.message : String(cause), 'error');
      }
    } finally {
      finishOperation(operation);
    }
  };

  const createArchive = async (
    previewCutoffTimestamp: number,
    confirmation?: ConfirmationToken
  ) => {
    if (confirmation && !confirmationIsCurrent(confirmation)) return;
    const operation = beginWorking(serviceBase, managementKey);
    if (!operation) return;
    try {
      await usageServiceApi.createUsageArchive(
        serviceBase,
        previewCutoffTimestamp,
        managementKey,
        operation.controller.signal
      );
      if (!operationIsCurrent(operation)) return;
      setPreview(null);
      showNotification(
        t('usage_maintenance.create_success', { defaultValue: 'Archive run created.' }),
        'success'
      );
      await load({ background: true });
    } catch (cause) {
      if (operationIsCurrent(operation)) {
        showNotification(cause instanceof Error ? cause.message : String(cause), 'error');
        await load({ background: true });
      }
    } finally {
      finishOperation(operation);
    }
  };

  const confirmCreate = () => {
    if (!preview) return;
    const previewCutoffTimestamp = preview.cutoff_timestamp_ms;
    const confirmation = {
      generation: contextGenerationRef.current,
      serviceBase,
      managementKey,
    };
    showConfirmation({
      title: t('usage_maintenance.create_confirm_title', { defaultValue: 'Create archive run?' }),
      message: t('usage_maintenance.create_confirm_message', {
        defaultValue:
          'This creates a resumable archive run without deleting raw data. Resume it afterward to write the archive files.',
      }),
      confirmText: t('usage_maintenance.create_confirm_button', { defaultValue: 'Create run' }),
      cancelText: t('common.cancel'),
      variant: 'primary',
      onConfirm: () => createArchive(previewCutoffTimestamp, confirmation),
    });
  };

  const runAction = async (
    run: UsageArchiveRunSummary,
    action: 'resume' | 'verify' | 'delete',
    confirmation?: ConfirmationToken
  ) => {
    if (confirmation && !confirmationIsCurrent(confirmation)) return;
    const method =
      action === 'resume'
        ? usageServiceApi.resumeUsageArchive
        : action === 'verify'
          ? usageServiceApi.verifyUsageArchive
          : usageServiceApi.deleteUsageArchive;
    const operation = beginWorking(serviceBase, managementKey);
    if (!operation) return;
    try {
      await method(serviceBase, run.id, managementKey, operation.controller.signal);
      if (!operationIsCurrent(operation)) return;
      const destructive = actionIsDestructive(run, action);
      showNotification(
        t(`usage_maintenance.${destructive ? 'delete' : action}_success`, {
          defaultValue: destructive ? 'Logical deletion completed.' : 'Archive run updated.',
        }),
        'success'
      );
      await load({ background: true });
    } catch (cause) {
      if (operationIsCurrent(operation)) {
        showNotification(cause instanceof Error ? cause.message : String(cause), 'error');
        await load({ background: true });
      }
    } finally {
      finishOperation(operation);
    }
  };

  const formatTime = (value?: number) =>
    value ? formatDateTime(new Date(value), i18n.language) : '-';
  const knownValueLabel = (prefix: string, value: string, knownValues: ReadonlySet<string>) =>
    knownValues.has(value)
      ? t(`usage_maintenance.${prefix}_${value}`, { defaultValue: value })
      : value;
  const archiveStatusLabel = (value: string) =>
    knownValueLabel('run_status', value, archiveStatusTranslationValues);
  const archiveModeLabel = (value: string) =>
    knownValueLabel('run_mode', value, archiveModeTranslationValues);
  const migrationStatusLabel = (value: string) =>
    knownValueLabel('migration_status', value, migrationStatusTranslationValues);
  const aggregateStatusLabel = (value: string) =>
    knownValueLabel('aggregate_status', value, aggregateStatusTranslationValues);

  const confirmAction = (run: UsageArchiveRunSummary, action: 'resume' | 'verify' | 'delete') => {
    if (!actionIsDestructive(run, action)) {
      void runAction(run, action);
      return;
    }
    const remainingEventCount = Math.max(0, run.event_count - run.deleted_event_count);
    const confirmation = {
      generation: contextGenerationRef.current,
      serviceBase,
      managementKey,
    };
    showConfirmation({
      title: t('usage_maintenance.delete_confirm_title', { defaultValue: 'Delete raw events?' }),
      message: t('usage_maintenance.delete_confirm_message', {
        defaultValue:
          'Run {{runId}} will delete up to {{remainingEventCount}} remaining raw events ({{totalEventCount}} total archived) before {{cutoff}}. Archive files remain untouched.',
        runId: run.id,
        remainingEventCount: remainingEventCount.toLocaleString(i18n.language),
        totalEventCount: run.event_count.toLocaleString(i18n.language),
        cutoff: formatTime(run.cutoff_timestamp_ms),
      }),
      confirmText: t('usage_maintenance.delete_confirm_button', {
        defaultValue: 'Delete raw rows',
      }),
      cancelText: t('common.cancel'),
      variant: 'danger',
      onConfirm: () => runAction(run, action, confirmation),
    });
  };

  const actionLabel = (action: ReturnType<typeof statusAction>) => {
    if (!action) return '';
    const fallback = action === 'resume' ? 'Resume' : action === 'verify' ? 'Verify' : 'Delete raw';
    return t(`usage_maintenance.action_${action}`, { defaultValue: fallback });
  };
  const deleteDisabled = maintenance?.readiness.archive_delete_enabled === false;
  const deleteReadinessHint = deleteDisabled
    ? t('usage_maintenance.delete_disabled', {
        defaultValue: 'Raw deletion is disabled until the server enables archive deletion.',
      })
    : maintenance &&
        (!maintenance.readiness.migration_ready || !maintenance.readiness.hourly_aggregate_ready)
      ? t('usage_maintenance.delete_readiness_pending', {
          defaultValue:
            'Global catch-up is still pending. The server will verify this run’s exact coverage before deletion.',
        })
      : '';

  if (availability.checking || loading) return <LoadingSpinner />;
  if (unsupported) {
    return (
      <div className={styles.page}>
        <section className={styles.unsupported}>
          <h1 className={styles.title}>
            {t('usage_maintenance.title', { defaultValue: 'Usage maintenance' })}
          </h1>
          <p className={styles.description}>
            {t('usage_maintenance.unsupported', {
              defaultValue:
                'This Manager Server is older than the usage maintenance API. Upgrade the server to manage archives here.',
            })}
          </p>
        </section>
      </div>
    );
  }

  return (
    <div className={styles.page}>
      <section className={styles.hero}>
        <div>
          <p className={styles.eyebrow}>
            {t('usage_maintenance.eyebrow', { defaultValue: 'Data lifecycle' })}
          </p>
          <h1 className={styles.title}>
            {t('usage_maintenance.title', { defaultValue: 'Usage maintenance' })}
          </h1>
          <p className={styles.description}>
            {t('usage_maintenance.subtitle', {
              defaultValue:
                'Review archive runs and reclaim SQLite space without mixing logical deletion with physical compaction.',
            })}
          </p>
        </div>
        <div className={styles.actions}>
          <Button variant="secondary" size="sm" onClick={() => void load()} disabled={working}>
            {t('common.refresh')}
          </Button>
        </div>
      </section>

      {error ? <div className={styles.error}>{error}</div> : null}

      {maintenance ? (
        <section
          className={styles.stats}
          aria-label={t('usage_maintenance.status_title', { defaultValue: 'Maintenance status' })}
        >
          <div className={styles.stat}>
            <span className={styles.statValue}>{maintenance.raw_event_count.toLocaleString()}</span>
            <span className={styles.statLabel}>
              {t('usage_maintenance.raw_events', { defaultValue: 'Raw events' })}
            </span>
          </div>
          <div className={styles.stat}>
            <span className={styles.statValue}>
              {maintenance.raw_deleted_event_count.toLocaleString()}
            </span>
            <span className={styles.statLabel}>
              {t('usage_maintenance.deleted_events', { defaultValue: 'Logically deleted' })}
            </span>
          </div>
          <div className={styles.stat}>
            <span className={styles.statValue}>
              {formatFileSize(maintenance.storage.reclaimable_bytes)}
            </span>
            <span className={styles.statLabel}>
              {t('usage_maintenance.reclaimable', { defaultValue: 'Reclaimable' })}
            </span>
          </div>
          <div className={styles.stat}>
            <span className={styles.statValue}>
              {migrationStatusLabel(maintenance.migration.status)}
            </span>
            <span className={styles.statLabel}>
              {t('usage_maintenance.migration', { defaultValue: 'Migration' })}
            </span>
          </div>
          <div className={styles.stat}>
            <span className={styles.statValue}>
              {aggregateStatusLabel(maintenance.hourly_aggregate.status)}
            </span>
            <span className={styles.statLabel}>
              {t('usage_maintenance.hourly_aggregate', { defaultValue: 'Hourly aggregate' })}
            </span>
          </div>
        </section>
      ) : null}

      <section className={styles.panel}>
        <div className={styles.panelHeader}>
          <h2 className={styles.panelTitle}>
            {t('usage_maintenance.archive_title', { defaultValue: 'Archive history' })}
          </h2>
          <span className={styles.muted}>
            {t('usage_maintenance.archive_hint', {
              defaultValue: 'Archive first, delete only after verification.',
            })}
          </span>
        </div>
        <div className={styles.toolbar}>
          <div className={styles.dateField}>
            <Input
              type="datetime-local"
              label={t('usage_maintenance.cutoff', { defaultValue: 'Archive events before' })}
              value={cutoff}
              disabled={working}
              onChange={(event) => {
                invalidateOperation(true);
                setCutoff(event.target.value);
                setPreview(null);
              }}
            />
          </div>
          <Button variant="secondary" onClick={() => void handlePreview()} loading={working}>
            {t('usage_maintenance.preview', { defaultValue: 'Preview' })}
          </Button>
        </div>
        {deleteReadinessHint ? <p className={styles.readinessHint}>{deleteReadinessHint}</p> : null}
        {preview ? (
          <div className={styles.preview}>
            <div className={styles.previewGrid}>
              <div className={styles.metric}>
                <span className={styles.metricLabel}>
                  {t('usage_maintenance.preview_events', { defaultValue: 'Eligible events' })}
                </span>
                <span className={styles.metricValue}>{preview.event_count.toLocaleString()}</span>
              </div>
              <div className={styles.metric}>
                <span className={styles.metricLabel}>
                  {t('usage_maintenance.preview_bytes', { defaultValue: 'Estimated size' })}
                </span>
                <span className={styles.metricValue}>
                  {formatFileSize(preview.estimated_bytes)}
                </span>
              </div>
              <div className={styles.metric}>
                <span className={styles.metricLabel}>
                  {t('usage_maintenance.preview_range', { defaultValue: 'Timestamp range' })}
                </span>
                <span className={styles.metricValue}>
                  {formatTime(preview.min_timestamp_ms)} – {formatTime(preview.max_timestamp_ms)}
                </span>
              </div>
            </div>
            <Button onClick={confirmCreate} disabled={working || preview.event_count <= 0}>
              {t('usage_maintenance.create', { defaultValue: 'Create archive run' })}
            </Button>
          </div>
        ) : null}
        <div className={styles.runList}>
          {archives.length === 0 ? (
            <p className={styles.muted}>
              {t('usage_maintenance.no_runs', { defaultValue: 'No archive runs yet.' })}
            </p>
          ) : null}
          {archives.map((run) => {
            const action = statusAction(run.status);
            const destructive = action ? actionIsDestructive(run, action) : false;
            return (
              <div className={styles.run} key={run.id}>
                <div className={styles.runMain}>
                  <div className={styles.runTitle}>
                    <span>{run.id}</span>
                    <span
                      className={`${styles.badge} ${runIsActive(run) ? styles.badgeActive : ''}`}
                    >
                      {archiveStatusLabel(run.status)}
                    </span>
                    <span className={styles.badge}>{archiveModeLabel(run.mode)}</span>
                  </div>
                  <div className={styles.runMeta}>
                    <span>
                      {run.event_count.toLocaleString()}{' '}
                      {t('usage_maintenance.events_suffix', { defaultValue: 'events' })}
                    </span>
                    <span>{formatFileSize(run.estimated_bytes)}</span>
                    <span>{formatTime(run.created_at_ms)}</span>
                  </div>
                </div>
                <div className={styles.runActions}>
                  {action ? (
                    <Button
                      size="xs"
                      variant={destructive ? 'danger' : 'secondary'}
                      disabled={working || (destructive && deleteDisabled)}
                      title={destructive ? deleteReadinessHint || undefined : undefined}
                      onClick={() => confirmAction(run, action)}
                    >
                      {actionLabel(action)}
                    </Button>
                  ) : null}
                </div>
              </div>
            );
          })}
        </div>
      </section>

      <section className={styles.notice}>
        <h2>
          {t('usage_maintenance.compact_title', { defaultValue: 'Physical compact (offline CLI)' })}
        </h2>
        <p>
          {t('usage_maintenance.compact_description', {
            defaultValue:
              'VACUUM is deliberately unavailable online. Stop Manager Server before compacting the SQLite file.',
          })}
        </p>
        <ul>
          <li>
            {t('usage_maintenance.compact_backup', {
              defaultValue:
                'Back up usage.sqlite, usage.sqlite-wal, usage.sqlite-shm, data.key, and the usage-archives directory together.',
            })}
          </li>
          <li>
            {t('usage_maintenance.compact_command', {
              defaultValue: 'Run: cpa-manager-plus compact-usage --db-path /path/to/usage.sqlite',
            })}
          </li>
          <li>
            {t('usage_maintenance.compact_restore', {
              defaultValue:
                'Restore the complete backup set before troubleshooting a failed checkpoint or integrity check.',
            })}
          </li>
        </ul>
      </section>
    </div>
  );
}
