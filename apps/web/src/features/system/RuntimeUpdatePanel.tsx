import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import { IconExternalLink, IconRefreshCw, IconSettings } from '@/components/ui/icons';
import { useAuthStore, useNotificationStore } from '@/stores';
import {
  getUsageServiceErrorCode,
  usageServiceApi,
  type RuntimeStatusResult,
  type RuntimeUpdateCheckResult,
  type RuntimeUpdateOperation,
} from '@/services/api/usageService';
import {
  clearRuntimeUpdateCheckCache,
  loadRuntimeUpdateCheck,
  readRuntimeUpdateCheckCache,
  runtimeUpdateCheckScope,
} from '@/services/runtimeUpdateCheckCache';
import styles from './RuntimeUpdatePanel.module.scss';

type UpdateTarget = 'cpamp' | 'cpa' | 'all';

const TERMINAL_OPERATION_STATES = new Set(['succeeded', 'failed', 'rolled_back']);

const OPERATION_STATE_RANK: Record<string, number> = {
  queued: 0,
  running: 1,
  handoff_pending: 2,
  rolling_back: 3,
  succeeded: 4,
  failed: 4,
  rolled_back: 4,
};

const OPERATION_MESSAGE_KEYS: Record<string, string> = {
  'update queued': 'system_info.runtime_operation_message_queued',
  'verifying and installing release assets': 'system_info.runtime_operation_message_verifying',
  'update failed': 'system_info.runtime_operation_message_failed',
  'update completed': 'system_info.runtime_operation_message_completed',
  'restarting runtime to complete update': 'system_info.runtime_operation_message_handoff_pending',
  'update failed; rolling back': 'system_info.runtime_operation_message_rolling_back',
  'update interrupted by runtime restart': 'system_info.runtime_operation_message_interrupted',
  'interrupted update rolled back during runtime startup':
    'system_info.runtime_operation_message_interrupted_rolled_back',
};

const COMPONENT_RESULT_STATUS_KEYS: Record<string, string> = {
  succeeded: 'system_info.runtime_result_succeeded',
  failed: 'system_info.runtime_result_failed',
  rolled_back: 'system_info.runtime_result_rolled_back',
  rollback_failed: 'system_info.runtime_result_rollback_failed',
};

const componentLabelKey = (name: string) =>
  name === 'cpa' ? 'system_info.runtime_component_cpa' : 'system_info.runtime_component_cpamp';

export function RuntimeUpdatePanel() {
  const { t } = useTranslation();
  const apiBase = useAuthStore((state) => state.apiBase);
  const adminKey = useAuthStore((state) => state.managementKey);
  const sessionMode = useAuthStore((state) => state.sessionMode);
  const { showNotification, showConfirmation } = useNotificationStore();
  const [status, setStatus] = useState<RuntimeStatusResult | null>(null);
  const [statusLoading, setStatusLoading] = useState(false);
  const [statusError, setStatusError] = useState('');
  const [supported, setSupported] = useState(true);
  const [basePathDraft, setBasePathDraft] = useState('');
  const [basePathSaving, setBasePathSaving] = useState(false);
  const [checkingUpdates, setCheckingUpdates] = useState(false);
  const [updateCheck, setUpdateCheck] = useState<RuntimeUpdateCheckResult | null>(null);
  const [selectedTarget, setSelectedTarget] = useState<UpdateTarget | null>(null);
  const [startingUpdate, setStartingUpdate] = useState(false);
  const [operation, setOperation] = useState<RuntimeUpdateOperation | null>(null);
  const [operationToken, setOperationToken] = useState('');
  const operationRef = useRef<RuntimeUpdateOperation | null>(null);
  const terminalNotificationRef = useRef('');
  const sessionGenerationRef = useRef(0);
  const statusRequestRef = useRef(0);
  const checkingUpdatesRef = useRef(false);
  const startingUpdateRef = useRef(false);
  const basePathSavingRef = useRef(false);
  const operationId = operation?.id || '';
  const operationStatus = operation?.status || '';
  const runtimeCheckScope = useMemo(
    () => runtimeUpdateCheckScope(apiBase, adminKey),
    [adminKey, apiBase]
  );

  useEffect(() => {
    sessionGenerationRef.current += 1;
    statusRequestRef.current += 1;
    setStatus(null);
    setStatusLoading(false);
    setStatusError('');
    setSupported(true);
    setBasePathDraft('');
    setBasePathSaving(false);
    basePathSavingRef.current = false;
    setCheckingUpdates(false);
    checkingUpdatesRef.current = false;
    setUpdateCheck(readRuntimeUpdateCheckCache(runtimeCheckScope));
    setSelectedTarget(null);
    setStartingUpdate(false);
    startingUpdateRef.current = false;
    setOperation(null);
    operationRef.current = null;
    setOperationToken('');
    terminalNotificationRef.current = '';
  }, [adminKey, apiBase, runtimeCheckScope, sessionMode]);

  useEffect(
    () => () => {
      sessionGenerationRef.current += 1;
      statusRequestRef.current += 1;
      checkingUpdatesRef.current = false;
      startingUpdateRef.current = false;
      basePathSavingRef.current = false;
    },
    []
  );

  const acceptOperation = useCallback((next: RuntimeUpdateOperation) => {
    const current = operationRef.current;
    if (current?.id === next.id) {
      const currentUpdatedAt = current.updatedAtMs;
      const nextUpdatedAt = next.updatedAtMs;
      if (Number.isFinite(currentUpdatedAt) && Number.isFinite(nextUpdatedAt)) {
        if (currentUpdatedAt > nextUpdatedAt) return false;
        if (currentUpdatedAt === nextUpdatedAt) {
          const currentTerminal = TERMINAL_OPERATION_STATES.has(current.status);
          const nextTerminal = TERMINAL_OPERATION_STATES.has(next.status);
          if (currentTerminal !== nextTerminal) {
            if (currentTerminal) return false;
          } else {
            const currentRank = OPERATION_STATE_RANK[current.status] ?? 0;
            const nextRank = OPERATION_STATE_RANK[next.status] ?? 0;
            if (
              currentRank > nextRank ||
              (currentRank === nextRank && current.progress >= next.progress)
            ) {
              return false;
            }
          }
        }
      }
    }
    operationRef.current = next;
    setOperation(next);
    return true;
  }, []);

  const loadStatus = useCallback(async () => {
    const requestId = statusRequestRef.current + 1;
    statusRequestRef.current = requestId;
    if (sessionMode !== 'manager_embedded' || !apiBase || !adminKey) return;
    const sessionGeneration = sessionGenerationRef.current;
    setStatusLoading(true);
    setStatusError('');
    try {
      const result = await usageServiceApi.getRuntimeStatus(apiBase, adminKey);
      if (
        requestId !== statusRequestRef.current ||
        sessionGeneration !== sessionGenerationRef.current
      ) {
        return;
      }
      setStatus(result);
      if (!result.runtimeAvailable || !result.updateCapabilities.all) {
        clearRuntimeUpdateCheckCache();
        setUpdateCheck(null);
      }
      setBasePathDraft(result.deployment.panelBasePath || '/management.html');
      const latestOperationId = result.runtime?.latestOperationId;
      const latestOperation = latestOperationId
        ? result.runtime?.operations?.[latestOperationId]
        : undefined;
      if (latestOperation) {
        acceptOperation(latestOperation);
      }
      setSupported(true);
    } catch (error) {
      if (
        requestId !== statusRequestRef.current ||
        sessionGeneration !== sessionGenerationRef.current
      ) {
        return;
      }
      clearRuntimeUpdateCheckCache();
      setStatus(null);
      setUpdateCheck(null);
      setSelectedTarget(null);
      setBasePathDraft('');
      const typed = error as { status?: number; message?: string };
      if (typed.status === 404) {
        setSupported(false);
        return;
      }
      setStatusError(typed.message || t('system_info.runtime_status_failed'));
    } finally {
      if (
        requestId === statusRequestRef.current &&
        sessionGeneration === sessionGenerationRef.current
      ) {
        setStatusLoading(false);
      }
    }
  }, [acceptOperation, adminKey, apiBase, sessionMode, t]);

  const finalizeTerminalOperation = useCallback(
    (next: RuntimeUpdateOperation) => {
      setOperationToken('');
      clearRuntimeUpdateCheckCache();
      setUpdateCheck(null);
      void loadStatus();
      if (terminalNotificationRef.current === next.id) return;
      terminalNotificationRef.current = next.id;
      showNotification(
        next.status === 'succeeded'
          ? t('system_info.runtime_update_succeeded')
          : next.status === 'rolled_back'
            ? t('system_info.runtime_update_rolled_back')
            : t('system_info.runtime_update_failed'),
        next.status === 'succeeded' ? 'success' : 'error'
      );
    },
    [loadStatus, showNotification, t]
  );

  useEffect(() => {
    void loadStatus();
    return () => {
      statusRequestRef.current += 1;
    };
  }, [loadStatus]);

  useEffect(() => {
    if (!operationId || TERMINAL_OPERATION_STATES.has(operationStatus)) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const sessionGeneration = sessionGenerationRef.current;
    const poll = async () => {
      let next: RuntimeUpdateOperation | null = null;
      try {
        next = await usageServiceApi.getRuntimeOperation(apiBase, adminKey, operationId);
      } catch {
        if (operationToken) {
          try {
            next = await usageServiceApi.getRuntimeOperationDirect(
              apiBase,
              operationId,
              operationToken
            );
          } catch {
            // The gateway and Manager can both be briefly unavailable during handoff.
          }
        }
      }
      if (cancelled || sessionGeneration !== sessionGenerationRef.current) return;
      if (next) {
        const accepted = acceptOperation(next);
        const current = operationRef.current;
        if (accepted && TERMINAL_OPERATION_STATES.has(next.status)) {
          finalizeTerminalOperation(next);
          return;
        }
        if (!accepted && current && TERMINAL_OPERATION_STATES.has(current.status)) {
          return;
        }
      }
      timer = setTimeout(poll, 1200);
    };
    timer = setTimeout(poll, 600);
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [
    acceptOperation,
    adminKey,
    apiBase,
    finalizeTerminalOperation,
    operationId,
    operationStatus,
    operationToken,
  ]);

  const handleCheckUpdates = useCallback(async () => {
    if (checkingUpdatesRef.current) return;
    const sessionGeneration = sessionGenerationRef.current;
    const statusRequest = statusRequestRef.current;
    checkingUpdatesRef.current = true;
    setCheckingUpdates(true);
    try {
      const result = await loadRuntimeUpdateCheck(
        runtimeCheckScope,
        () => usageServiceApi.checkRuntimeUpdates(apiBase, adminKey),
        true
      );
      if (
        sessionGeneration !== sessionGenerationRef.current ||
        statusRequest !== statusRequestRef.current
      ) {
        return;
      }
      setUpdateCheck(result);
      const available = Object.values(result.components).filter(
        (component) => component.updateAvailable
      ).length;
      showNotification(
        available > 0
          ? t('system_info.runtime_updates_found', { count: available })
          : t('system_info.runtime_updates_current'),
        available > 0 ? 'warning' : 'success'
      );
    } catch (error) {
      if (
        sessionGeneration !== sessionGenerationRef.current ||
        statusRequest !== statusRequestRef.current
      ) {
        return;
      }
      clearRuntimeUpdateCheckCache();
      setUpdateCheck(null);
      const code = getUsageServiceErrorCode(error);
      showNotification(t(`usage_service_errors.${code || 'runtime_update_check_failed'}`), 'error');
    } finally {
      if (sessionGeneration === sessionGenerationRef.current) {
        checkingUpdatesRef.current = false;
        setCheckingUpdates(false);
      }
    }
  }, [adminKey, apiBase, runtimeCheckScope, showNotification, t]);

  const handleStartUpdate = useCallback(async () => {
    if (!selectedTarget || startingUpdateRef.current) return;
    const sessionGeneration = sessionGenerationRef.current;
    startingUpdateRef.current = true;
    setStartingUpdate(true);
    try {
      const result = await usageServiceApi.startRuntimeUpdate(apiBase, adminKey, selectedTarget);
      if (sessionGeneration !== sessionGenerationRef.current) return;
      terminalNotificationRef.current = '';
      acceptOperation(result.operation);
      clearRuntimeUpdateCheckCache();
      setUpdateCheck(null);
      setSelectedTarget(null);
      if (TERMINAL_OPERATION_STATES.has(result.operation.status)) {
        finalizeTerminalOperation(result.operation);
      } else {
        setOperationToken(result.operationToken);
        showNotification(t('system_info.runtime_update_started'), 'info');
      }
    } catch (error) {
      if (sessionGeneration !== sessionGenerationRef.current) return;
      const code = getUsageServiceErrorCode(error);
      showNotification(t(`usage_service_errors.${code || 'runtime_update_start_failed'}`), 'error');
    } finally {
      if (sessionGeneration === sessionGenerationRef.current) {
        startingUpdateRef.current = false;
        setStartingUpdate(false);
      }
    }
  }, [
    acceptOperation,
    adminKey,
    apiBase,
    finalizeTerminalOperation,
    selectedTarget,
    showNotification,
    t,
  ]);

  const saveBasePath = useCallback(async () => {
    if (basePathSavingRef.current) return;
    const sessionGeneration = sessionGenerationRef.current;
    basePathSavingRef.current = true;
    setBasePathSaving(true);
    try {
      const result = await usageServiceApi.updateRuntimePanelBasePath(
        apiBase,
        adminKey,
        basePathDraft
      );
      if (sessionGeneration !== sessionGenerationRef.current) return;
      setStatus((current) => (current ? { ...current, deployment: result.deployment } : current));
      showNotification(t('system_info.runtime_base_path_saved'), 'success');
      if (typeof window !== 'undefined') {
        const search = window.location.search || '';
        const hash = window.location.hash || '#/system';
        window.location.assign(`${result.panelPath}${search}${hash}`);
      }
    } catch (error) {
      if (sessionGeneration !== sessionGenerationRef.current) return;
      const code = getUsageServiceErrorCode(error);
      showNotification(t(`usage_service_errors.${code || 'panel_base_path_invalid'}`), 'error');
    } finally {
      if (sessionGeneration === sessionGenerationRef.current) {
        basePathSavingRef.current = false;
        setBasePathSaving(false);
      }
    }
  }, [adminKey, apiBase, basePathDraft, showNotification, t]);

  const confirmBasePathSave = useCallback(() => {
    showConfirmation({
      title: t('system_info.runtime_base_path_confirm_title'),
      message: t('system_info.runtime_base_path_confirm_message', {
        path: basePathDraft || '/',
      }),
      confirmText: t('common.confirm'),
      variant: 'primary',
      onConfirm: saveBasePath,
    });
  }, [basePathDraft, saveBasePath, showConfirmation, t]);

  const selectedComponents = useMemo(() => {
    if (!selectedTarget || !updateCheck) return [];
    const names = selectedTarget === 'all' ? ['cpamp', 'cpa'] : [selectedTarget];
    return names
      .map((name) => updateCheck.components[name])
      .filter((component) => component?.updateAvailable);
  }, [selectedTarget, updateCheck]);

  const hasCPAMPUpdate = Boolean(updateCheck?.components.cpamp?.updateAvailable);
  const hasCPAUpdate = Boolean(updateCheck?.components.cpa?.updateAvailable);
  const cpaStandaloneUpdateAllowed = updateCheck?.components.cpa?.standaloneUpdateAllowed !== false;
  const cpaCombinedUpdateAllowed = updateCheck?.components.cpa?.combinedUpdateAllowed !== false;
  const cpaRequiresCPAMPUpdate =
    hasCPAUpdate && !cpaStandaloneUpdateAllowed && cpaCombinedUpdateAllowed;
  const cpaUpdateBlocked = hasCPAUpdate && !cpaCombinedUpdateAllowed;
  const operationActive = Boolean(operation && !TERMINAL_OPERATION_STATES.has(operation.status));
  const runtimeControlUnavailable = Boolean(
    status?.deployment.runtimeManaged && !status.runtimeAvailable
  );
  const updatesManaged = Boolean(status?.updateCapabilities.all && !runtimeControlUnavailable);
  const basePathEnvManaged = ['env', 'environment'].includes(
    status?.deployment.panelBasePathSource || ''
  );

  const operationMessage = useMemo(() => {
    if (!operation) return '';
    const message = operation.message?.trim() || '';
    if (!message) return `${operation.progress}%`;
    const key = OPERATION_MESSAGE_KEYS[message];
    if (key) return t(key);
    if (message.startsWith('updating ')) {
      const component = message.slice('updating '.length).trim();
      return t('system_info.runtime_operation_message_updating_component', {
        component: t(componentLabelKey(component)),
      });
    }
    return message;
  }, [operation, t]);

  if (sessionMode !== 'manager_embedded' || !supported) return null;

  return (
    <>
      <Card
        title={t('system_info.runtime_title')}
        extra={
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void loadStatus()}
            loading={statusLoading}
          >
            {t('common.refresh')}
          </Button>
        }
      >
        <p className={styles.description}>{t('system_info.runtime_description')}</p>
        {statusError && (
          <div className="error-box" role="alert">
            {statusError}
          </div>
        )}
        {status && (
          <div className={styles.sections}>
            <section className={styles.section}>
              <div className={styles.sectionHeader}>
                <div>
                  <h3>{t('system_info.runtime_versions_title')}</h3>
                  <p>{t('system_info.runtime_versions_hint')}</p>
                </div>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => void handleCheckUpdates()}
                  loading={checkingUpdates}
                  disabled={!updatesManaged || operationActive}
                >
                  <IconRefreshCw size={14} /> {t('system_info.runtime_check_updates')}
                </Button>
              </div>

              <div className={styles.versionGrid}>
                {['cpamp', 'cpa'].map((name) => {
                  const runtimeComponent = status.runtime?.components?.[name];
                  const release = updateCheck?.components[name];
                  return (
                    <div key={name} className={styles.versionCard}>
                      <div className={styles.versionName}>{t(componentLabelKey(name))}</div>
                      <div className={styles.versionValue}>
                        {runtimeComponent?.version ||
                          release?.currentVersion ||
                          t('dashboard.version_unknown')}
                      </div>
                      {release?.updateAvailable ? (
                        <span className={styles.updateBadge}>
                          {t('system_info.runtime_update_available', {
                            version: release.availableVersion,
                          })}
                        </span>
                      ) : updateCheck ? (
                        <span className={styles.currentBadge}>
                          {t('system_info.runtime_component_current')}
                        </span>
                      ) : null}
                    </div>
                  );
                })}
              </div>

              <div className={styles.updateActions}>
                <Button
                  variant="secondary"
                  onClick={() => setSelectedTarget('cpamp')}
                  disabled={!updatesManaged || !hasCPAMPUpdate || operationActive}
                >
                  {t('system_info.runtime_update_cpamp')}
                </Button>
                <Button
                  variant="secondary"
                  onClick={() => setSelectedTarget('cpa')}
                  disabled={
                    !updatesManaged ||
                    !hasCPAUpdate ||
                    !cpaStandaloneUpdateAllowed ||
                    operationActive
                  }
                >
                  {t('system_info.runtime_update_cpa')}
                </Button>
                <Button
                  onClick={() => setSelectedTarget('all')}
                  disabled={
                    !updatesManaged ||
                    (!hasCPAMPUpdate && !hasCPAUpdate) ||
                    cpaUpdateBlocked ||
                    operationActive
                  }
                >
                  {t('system_info.runtime_update_all')}
                </Button>
              </div>

              {cpaRequiresCPAMPUpdate && (
                <div className={styles.notice} role="status">
                  {t('system_info.runtime_cpa_requires_cpamp_update', {
                    cpaVersion: updateCheck?.components.cpa?.availableVersion || '-',
                    cpampVersion: updateCheck?.components.cpa?.minCpampVersion || '-',
                  })}
                </div>
              )}
              {cpaUpdateBlocked && (
                <div className={styles.notice} role="alert">
                  {t('system_info.runtime_cpa_update_incompatible', {
                    cpaVersion: updateCheck?.components.cpa?.availableVersion || '-',
                    cpampVersion: updateCheck?.components.cpa?.minCpampVersion || '-',
                  })}
                </div>
              )}

              {!status.updateCapabilities.all && (
                <div className={styles.notice}>{t('system_info.runtime_updates_readonly')}</div>
              )}
              {runtimeControlUnavailable && (
                <div className={styles.notice}>
                  {t('usage_service_errors.runtime_control_unavailable')}
                </div>
              )}

              {operation && (
                <div className={styles.operationBox} aria-live="polite">
                  <div className={styles.operationHeader}>
                    <strong>{t('system_info.runtime_operation_title')}</strong>
                    <span>{t(`system_info.runtime_operation_${operation.status}`)}</span>
                  </div>
                  <div
                    className={styles.progressTrack}
                    role="progressbar"
                    aria-label={t('system_info.runtime_operation_title')}
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={Math.max(0, Math.min(100, operation.progress))}
                  >
                    <div
                      className={styles.progressBar}
                      style={{ width: `${Math.max(0, Math.min(100, operation.progress))}%` }}
                    />
                  </div>
                  <div className={styles.operationMessage}>{operationMessage}</div>
                  {operation.error && (
                    <div className="error-box" role="alert">
                      {operation.error}
                    </div>
                  )}
                  {operation.results && (
                    <div className={styles.operationResults}>
                      {Object.values(operation.results).map((result) => {
                        const resultStatusKey = COMPONENT_RESULT_STATUS_KEYS[result.status];
                        return (
                          <div key={result.name}>
                            <span>{t(componentLabelKey(result.name))}</span>
                            <strong>{resultStatusKey ? t(resultStatusKey) : result.status}</strong>
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              )}
            </section>

            <section className={styles.section}>
              <div className={styles.sectionHeader}>
                <div>
                  <h3>{t('system_info.runtime_base_path_title')}</h3>
                  <p>{t('system_info.runtime_base_path_hint')}</p>
                </div>
                <IconSettings size={20} />
              </div>
              <div className={styles.basePathRow}>
                <Input
                  label={t('system_info.runtime_base_path_label')}
                  value={basePathDraft}
                  onChange={(event) => setBasePathDraft(event.target.value)}
                  placeholder="/management.html"
                  disabled={basePathEnvManaged || runtimeControlUnavailable || basePathSaving}
                />
                <Button
                  onClick={confirmBasePathSave}
                  loading={basePathSaving}
                  disabled={
                    basePathEnvManaged ||
                    runtimeControlUnavailable ||
                    !basePathDraft.trim() ||
                    basePathDraft.trim() === status.deployment.panelBasePath
                  }
                >
                  {t('common.save')}
                </Button>
              </div>
              <div className={styles.notice}>{t('system_info.runtime_base_path_security')}</div>
              {basePathEnvManaged && (
                <div className={styles.notice}>
                  {t('system_info.runtime_base_path_env_managed')}
                </div>
              )}
            </section>
          </div>
        )}
      </Card>

      <Modal
        open={selectedTarget !== null}
        onClose={() => setSelectedTarget(null)}
        title={t('system_info.runtime_update_confirm_title')}
        width={680}
        closeDisabled={startingUpdate}
        footer={
          <>
            <Button
              variant="secondary"
              onClick={() => setSelectedTarget(null)}
              disabled={startingUpdate}
            >
              {t('common.cancel')}
            </Button>
            <Button
              onClick={() => void handleStartUpdate()}
              loading={startingUpdate}
              disabled={!updatesManaged || selectedComponents.length === 0}
            >
              {t('system_info.runtime_update_confirm_action')}
            </Button>
          </>
        }
      >
        <div className={styles.confirmContent}>
          <p>{t('system_info.runtime_update_confirm_intro')}</p>
          <div className={styles.confirmComponents}>
            {selectedComponents.map((component) => (
              <div key={component.name} className={styles.confirmComponent}>
                <div>
                  <strong>{t(componentLabelKey(component.name))}</strong>
                  <span>
                    {component.currentVersion || '-'} → {component.availableVersion || '-'}
                  </span>
                </div>
                {component.releaseUrl && (
                  <a href={component.releaseUrl} target="_blank" rel="noreferrer">
                    {t('system_info.runtime_release_page')} <IconExternalLink size={13} />
                  </a>
                )}
                {component.releaseNotes && (
                  <p className={styles.releaseNotes}>{component.releaseNotes}</p>
                )}
                {component.minCpampVersion && (
                  <p>
                    {t('system_info.runtime_compatibility_requirement', {
                      version: component.minCpampVersion,
                    })}
                  </p>
                )}
              </div>
            ))}
          </div>
          <div className={styles.confirmNotice}>
            <strong>{t('system_info.runtime_update_impact_title')}</strong>
            <p>{t('system_info.runtime_update_impact')}</p>
          </div>
          <div className={styles.confirmNotice}>
            <strong>{t('system_info.runtime_update_rollback_title')}</strong>
            <p>{t('system_info.runtime_update_rollback')}</p>
          </div>
        </div>
      </Modal>
    </>
  );
}
