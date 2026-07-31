import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { SelectionCheckbox } from '@/components/ui/SelectionCheckbox';
import {
  IconCheck,
  IconEye,
  IconEyeOff,
  IconInfo,
  IconLanguages,
  IconMoon,
  IconShield,
  IconSun,
} from '@/components/ui/icons';
import {
  useAuthStore,
  useLanguageStore,
  useNotificationStore,
  useThemeStore,
  useUsageServiceStore,
} from '@/stores';
import {
  LEGACY_USAGE_SERVICE_LAST_CPA_BASE_KEY,
  USAGE_SERVICE_LAST_CPA_BASE_KEY,
  getUsageServiceErrorCode,
  usageServiceApi,
  type UsageServiceInfo,
} from '@/services/api/usageService';
import {
  detectApiBaseFromLocation,
  normalizeApiBase,
  resolveDefaultCPAConnectionBase,
} from '@/utils/connection';
import { LANGUAGE_LABEL_KEYS, LANGUAGE_ORDER } from '@/utils/constants';
import { isSupportedLanguage } from '@/utils/language';
import { INLINE_LOGO_JPEG } from '@/assets/logoInline';
import type { ApiError } from '@/types';
import { resolveUsageServiceLoginMode } from './loginMode';
import { localizeSetupDocsUrl } from './setupDocs';
import styles from './LoginPage.module.scss';

type RedirectState = { from?: { pathname?: string } };
type UsageSetupStep = 'slim' | 'connection' | 'admin' | 'complete';
const CONFIG_TAB_STORAGE_KEY = 'config-management:tab';
const SETUP_STATE_STORAGE_KEY = 'cpa-manager-plus:setup-state-changed';

const readLocalStorage = (key: string): string => {
  try {
    return localStorage.getItem(key) || '';
  } catch {
    return '';
  }
};

const writeLocalStorage = (key: string, value: string) => {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Setup already succeeded server-side; unavailable browser storage must not roll it back in the UI.
  }
};

const publishSetupStateChanged = () => {
  writeLocalStorage(SETUP_STATE_STORAGE_KEY, `${Date.now()}:${Math.random()}`);
};

function getLocalizedErrorMessage(
  error: unknown,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  const usageServiceCode = getUsageServiceErrorCode(error);
  if (usageServiceCode) {
    return t(`usage_service_errors.${usageServiceCode}`, {
      defaultValue: t('usage_service_errors.request_failed'),
    });
  }

  const apiError = error as Partial<ApiError>;
  const status = typeof apiError.status === 'number' ? apiError.status : undefined;
  const code = typeof apiError.code === 'string' ? apiError.code : undefined;
  const message =
    error instanceof Error
      ? error.message
      : typeof apiError.message === 'string'
        ? apiError.message
        : typeof error === 'string'
          ? error
          : '';

  const withHttpStatus = (summary: string) => {
    if (!status) return summary;

    const genericAxiosMessage = `Request failed with status code ${status}`;
    const detail = message.trim();
    const backendDetail =
      detail && detail !== genericAxiosMessage
        ? ` (${t('login.error_backend_detail')}: ${detail})`
        : '';

    return `HTTP ${status}: ${summary}${backendDetail}`;
  };

  if (status === 401) return withHttpStatus(t('login.error_unauthorized'));
  if (status === 403) return withHttpStatus(t('login.error_forbidden'));
  if (status === 404) return withHttpStatus(t('login.error_not_found'));
  if (status && status >= 500) return withHttpStatus(t('login.error_server'));
  if (code === 'ECONNABORTED' || message.toLowerCase().includes('timeout')) {
    return t('login.error_timeout');
  }
  if (code === 'ERR_NETWORK' || message.toLowerCase().includes('network error')) {
    return t('login.error_network');
  }
  if (code === 'ERR_CERT_AUTHORITY_INVALID' || message.toLowerCase().includes('certificate')) {
    return t('login.error_ssl');
  }
  if (message.toLowerCase().includes('cors') || message.toLowerCase().includes('cross-origin')) {
    return t('login.error_cors');
  }

  return withHttpStatus(t('login.error_invalid'));
}

export function LoginPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const { showNotification } = useNotificationStore();
  const language = useLanguageStore((state) => state.language);
  const setLanguage = useLanguageStore((state) => state.setLanguage);
  const theme = useThemeStore((state) => state.theme);
  const cycleTheme = useThemeStore((state) => state.cycleTheme);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const login = useAuthStore((state) => state.login);
  const restoreSession = useAuthStore((state) => state.restoreSession);
  const storedBase = useAuthStore((state) => state.apiBase);
  const storedKey = useAuthStore((state) => state.managementKey);
  const storedRememberPassword = useAuthStore((state) => state.rememberPassword);
  const setUsageServiceConfig = useUsageServiceStore((state) => state.setUsageServiceConfig);
  const languageMenuRef = useRef<HTMLDivElement | null>(null);
  const mountedRef = useRef(true);
  const actionInFlightRef = useRef(false);
  const refreshGenerationRef = useRef(0);
  const autoLoginTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [apiBase, setApiBase] = useState('');
  const [adminKey, setAdminKey] = useState('');
  const [cpaManagementKey, setCPAManagementKey] = useState('');
  const [showCustomBase, setShowCustomBase] = useState(false);
  const [showAdminKey, setShowAdminKey] = useState(false);
  const [showCPAManagementKey, setShowCPAManagementKey] = useState(false);
  const [rememberCredential, setRememberCredential] = useState(false);
  const [bootstrapToken, setBootstrapToken] = useState(
    () => new URLSearchParams(location.search).get('bootstrap')?.trim() || ''
  );
  const [deploymentMode, setDeploymentMode] = useState('external');
  const [bootstrapRequired, setBootstrapRequired] = useState(false);
  const [adminReady, setAdminReady] = useState(false);
  const [setupIncludesCPA, setSetupIncludesCPA] = useState(true);
  const [errorDocsUrl, setErrorDocsUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [autoLoading, setAutoLoading] = useState(true);
  const [autoLoginSuccess, setAutoLoginSuccess] = useState(false);
  const [error, setError] = useState('');
  const [hostedByUsageService, setHostedByUsageService] = useState(false);
  const [usageServiceNeedsSetup, setUsageServiceNeedsSetup] = useState(false);
  const [languageMenuOpen, setLanguageMenuOpen] = useState(false);
  const [hasHistoricalData, setHasHistoricalData] = useState(false);
  const [migrationStatus, setMigrationStatus] = useState('');
  const [usageSetupStep, setUsageSetupStep] = useState<UsageSetupStep>('connection');

  const detectedBase = useMemo(() => detectApiBaseFromLocation(), []);
  const isManagerServerMode = hostedByUsageService;
  const loginCredential = isManagerServerMode ? adminKey : cpaManagementKey;
  const loginCredentialLabel = isManagerServerMode
    ? t('login.admin_key_label')
    : t('login.cpa_management_key_label');
  const loginCredentialPlaceholder = isManagerServerMode
    ? t('login.admin_key_placeholder')
    : t('login.cpa_management_key_placeholder');
  const loginCredentialHint = isManagerServerMode
    ? t('login.admin_key_hint')
    : t('login.cpa_management_key_hint');

  const usageSetupSteps = useMemo<UsageSetupStep[]>(() => {
    const steps: UsageSetupStep[] = [];
    if (deploymentMode === 'slim' && setupIncludesCPA) steps.push('slim');
    if (setupIncludesCPA) steps.push('connection');
    if (!adminReady) steps.push('admin');
    steps.push('complete');
    return steps;
  }, [adminReady, deploymentMode, setupIncludesCPA]);
  const usageSetupStepIndex = Math.max(0, usageSetupSteps.indexOf(usageSetupStep));
  const usageSetupIsFirstStep = usageSetupStepIndex <= 0;
  const usageSetupIsLastStep = usageSetupStep === 'complete';
  const usageSetupStepLabels = useMemo<Record<UsageSetupStep, string>>(
    () => ({
      slim: t('login.step_cpa_source'),
      admin: t('login.step_admin_key'),
      connection: t('login.step_connection'),
      complete: t('login.step_complete'),
    }),
    [t]
  );
  const toggleLanguageMenu = useCallback(() => {
    setLanguageMenuOpen((prev) => !prev);
  }, []);

  const beginAction = useCallback(() => {
    if (!mountedRef.current || actionInFlightRef.current) return false;
    actionInFlightRef.current = true;
    refreshGenerationRef.current += 1;
    if (mountedRef.current) setLoading(true);
    return true;
  }, []);

  const finishAction = useCallback(() => {
    actionInFlightRef.current = false;
    if (mountedRef.current) setLoading(false);
  }, []);

  const applyUsageServiceInfo = useCallback(
    (info: UsageServiceInfo, options: { preserveLocalStep?: boolean } = {}) => {
      const mode = resolveUsageServiceLoginMode(info);
      const infoAdminReady = Boolean(info.adminReady);
      const infoProjectInitialized = Boolean(info.projectInitialized ?? info.configured);
      const infoDeploymentMode = info.deploymentMode || 'external';
      const includesCPA = !infoProjectInitialized;

      setHostedByUsageService(mode.hostedByUsageService);
      setUsageServiceNeedsSetup(mode.usageServiceNeedsSetup);
      setHasHistoricalData(Boolean(info.hasHistoricalData));
      setMigrationStatus(info.migrationStatus || '');
      setDeploymentMode(infoDeploymentMode);
      setBootstrapRequired(Boolean(info.bootstrapRequired));
      setAdminReady(infoAdminReady);
      setSetupIncludesCPA(includesCPA);

      if (!options.preserveLocalStep || !mode.usageServiceNeedsSetup || infoProjectInitialized) {
        if (infoDeploymentMode === 'slim' && includesCPA) {
          setUsageSetupStep('slim');
        } else if (includesCPA) {
          setUsageSetupStep('connection');
        } else if (!infoAdminReady) {
          setUsageSetupStep('admin');
        } else {
          setUsageSetupStep('complete');
        }
      }

      return {
        hostedByUsageService: mode.hostedByUsageService,
        usageServiceNeedsSetup: mode.usageServiceNeedsSetup,
        configured: mode.hostedByUsageService && !mode.usageServiceNeedsSetup,
        setupIncludesCPA: includesCPA,
      };
    },
    []
  );

  const handleLanguageSelect = useCallback(
    (selectedLanguage: string) => {
      if (!isSupportedLanguage(selectedLanguage)) {
        return;
      }

      setLanguage(selectedLanguage);
      setLanguageMenuOpen(false);
    },
    [setLanguage]
  );

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (autoLoginTimeoutRef.current !== null) {
        clearTimeout(autoLoginTimeoutRef.current);
        autoLoginTimeoutRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    if (!languageMenuOpen) {
      return;
    }

    const handlePointerDown = (event: MouseEvent) => {
      if (!languageMenuRef.current?.contains(event.target as Node)) {
        setLanguageMenuOpen(false);
      }
    };

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setLanguageMenuOpen(false);
      }
    };

    document.addEventListener('mousedown', handlePointerDown);
    document.addEventListener('keydown', handleEscape);

    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
      document.removeEventListener('keydown', handleEscape);
    };
  }, [languageMenuOpen]);

  useEffect(() => {
    const searchParams = new URLSearchParams(location.search);
    if (!searchParams.has('bootstrap')) return;
    searchParams.delete('bootstrap');
    const remainingSearch = searchParams.toString();
    navigate(
      {
        pathname: location.pathname,
        search: remainingSearch ? `?${remainingSearch}` : '',
        hash: location.hash,
      },
      { replace: true, state: location.state }
    );
    // The token remains only in component memory and is removed from browser history.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    let cancelled = false;
    const init = async () => {
      try {
        let detectedUsageService = false;
        let detectedUsageServiceConfigured = false;
        let detectedSetupIncludesCPA = false;
        try {
          const info = await usageServiceApi.getInfo(detectedBase);
          if (cancelled || !mountedRef.current) return;
          const detection = applyUsageServiceInfo(info);
          detectedUsageService = detection.hostedByUsageService;
          detectedUsageServiceConfigured = detection.configured;
          detectedSetupIncludesCPA = detection.setupIncludesCPA;
        } catch {
          if (cancelled || !mountedRef.current) return;
          detectedUsageService = false;
          detectedUsageServiceConfigured = false;
          setHostedByUsageService(false);
          setUsageServiceNeedsSetup(false);
          setHasHistoricalData(false);
          setMigrationStatus('');
          setDeploymentMode('external');
          setBootstrapRequired(false);
          setAdminReady(false);
          setSetupIncludesCPA(true);
          detectedSetupIncludesCPA = true;
        }

        const hostedManagementPage =
          typeof window !== 'undefined' && /\/management\.html$/i.test(window.location.pathname);
        const autoLoginExpectedPanelBase =
          detectedUsageService || hostedManagementPage ? detectedBase : undefined;
        const autoLoggedIn = await restoreSession({
          expectedMode: detectedUsageService ? 'manager_embedded' : 'external_panel',
          expectedPanelBase: autoLoginExpectedPanelBase,
        });
        if (cancelled || !mountedRef.current) return;
        if (detectedUsageService) {
          setUsageServiceConfig(
            { enabled: true, serviceBase: detectedBase },
            { panelBase: detectedBase, panelHostMode: 'manager_embedded' }
          );
        }
        if (autoLoggedIn) {
          setAutoLoginSuccess(true);
          autoLoginTimeoutRef.current = setTimeout(() => {
            if (!mountedRef.current) return;
            const redirect =
              autoLoggedIn.recoveryMode === 'manager_config'
                ? '/config'
                : (location.state as RedirectState | null)?.from?.pathname || '/';
            if (autoLoggedIn.recoveryMode === 'manager_config') {
              writeLocalStorage(CONFIG_TAB_STORAGE_KEY, 'manager');
            }
            navigate(redirect, { replace: true });
          }, 1500);
          return;
        }

        const lastCPAForUsageService =
          readLocalStorage(USAGE_SERVICE_LAST_CPA_BASE_KEY) ||
          readLocalStorage(LEGACY_USAGE_SERVICE_LAST_CPA_BASE_KEY);
        const defaultCPAConnectionBase = resolveDefaultCPAConnectionBase({
          hostedByUsageService: detectedUsageService,
          currentBase: detectedBase,
        });
        setApiBase(
          detectedUsageService
            ? detectedUsageServiceConfigured
              ? detectedBase
              : lastCPAForUsageService || defaultCPAConnectionBase
            : storedBase || detectedBase
        );
        setShowCustomBase(
          detectedUsageService && !detectedUsageServiceConfigured && detectedSetupIncludesCPA
        );
        if (detectedUsageService) {
          setAdminKey(storedKey || '');
          setCPAManagementKey('');
        } else {
          setAdminKey('');
          setCPAManagementKey(storedKey || '');
        }
        setRememberCredential(storedRememberPassword || Boolean(storedKey));
      } finally {
        if (!cancelled && mountedRef.current) setAutoLoading(false);
      }
    };

    init();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const refreshUsageServiceInfo = useCallback(async () => {
    if (autoLoading || actionInFlightRef.current) return;
    const generation = refreshGenerationRef.current + 1;
    refreshGenerationRef.current = generation;
    try {
      const info = await usageServiceApi.getInfo(detectedBase);
      if (
        !mountedRef.current ||
        actionInFlightRef.current ||
        refreshGenerationRef.current !== generation
      ) {
        return;
      }
      const detection = applyUsageServiceInfo(info, { preserveLocalStep: true });
      if (detection.hostedByUsageService) {
        setUsageServiceConfig(
          { enabled: true, serviceBase: detectedBase },
          { panelBase: detectedBase, panelHostMode: 'manager_embedded' }
        );
      }
      if (!detection.usageServiceNeedsSetup) {
        setApiBase(detectedBase);
        setShowCustomBase(false);
      }
    } catch {
      // Focus/storage synchronization is best-effort; the visible flow keeps its current state.
    }
  }, [applyUsageServiceInfo, autoLoading, detectedBase, setUsageServiceConfig]);

  useEffect(() => {
    const handleFocus = () => void refreshUsageServiceInfo();
    const handleStorage = (event: StorageEvent) => {
      if (event.key === SETUP_STATE_STORAGE_KEY) handleFocus();
    };
    const handleVisibilityChange = () => {
      if (typeof document !== 'undefined' && document.visibilityState === 'visible') handleFocus();
    };

    window.addEventListener?.('focus', handleFocus);
    window.addEventListener?.('storage', handleStorage);
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', handleVisibilityChange);
    }
    return () => {
      window.removeEventListener?.('focus', handleFocus);
      window.removeEventListener?.('storage', handleStorage);
      if (typeof document !== 'undefined') {
        document.removeEventListener('visibilitychange', handleVisibilityChange);
      }
    };
  }, [refreshUsageServiceInfo]);

  useEffect(() => {
    if (!usageSetupSteps.includes(usageSetupStep)) {
      setUsageSetupStep(usageSetupSteps[0] || 'complete');
    }
  }, [usageSetupStep, usageSetupSteps]);

  const validateUsageSetupStep = useCallback(
    (step: UsageSetupStep) => {
      setErrorDocsUrl('');
      if (bootstrapRequired && step !== 'complete' && !bootstrapToken.trim()) {
        setError(t('login.bootstrap_token_required'));
        return false;
      }
      if ((step === 'slim' || step === 'connection') && adminReady && !adminKey.trim()) {
        setError(t('login.admin_key_required'));
        return false;
      }
      if (step === 'connection') {
        if (!apiBase.trim()) {
          setError(t('login.cpa_address_required'));
          return false;
        }
        if (!cpaManagementKey.trim()) {
          setError(t('login.cpa_management_key_required'));
          return false;
        }
      }
      if (step === 'admin') {
        const value = adminKey.trim();
        if (!value) {
          setError(t('login.admin_key_required'));
          return false;
        }
        const classes = [
          /[A-Z]/.test(value),
          /[a-z]/.test(value),
          /[0-9]/.test(value),
          /[^A-Za-z0-9]/.test(value),
        ].filter(Boolean).length;
        if (Array.from(value).length < 16 || classes < 3) {
          setError(t('login.admin_key_policy'));
          return false;
        }
      }
      setError('');
      return true;
    },
    [adminKey, adminReady, apiBase, bootstrapRequired, bootstrapToken, cpaManagementKey, t]
  );

  const setupAuth = useMemo(
    () =>
      bootstrapRequired ? { bootstrapToken: bootstrapToken.trim() } : { adminKey: adminKey.trim() },
    [adminKey, bootstrapRequired, bootstrapToken]
  );

  const handleSetupError = useCallback(
    (err: unknown) => {
      if (!mountedRef.current) return;
      const message = getLocalizedErrorMessage(err, t);
      const docsUrl =
        typeof (err as { docsUrl?: unknown })?.docsUrl === 'string'
          ? ((err as { docsUrl: string }).docsUrl ?? '')
          : '';
      setError(message);
      setErrorDocsUrl(localizeSetupDocsUrl(docsUrl, language));
      showNotification(`${t('login.initialization_failed')}: ${message}`, 'error');
    },
    [language, showNotification, t]
  );

  const handleGenerateAdminKey = useCallback(async () => {
    if (bootstrapRequired && !bootstrapToken.trim()) {
      setError(t('login.bootstrap_token_required'));
      return;
    }
    if (!beginAction()) return;
    setError('');
    setErrorDocsUrl('');
    try {
      const result = await usageServiceApi.generateAdminKey(detectedBase, setupAuth);
      if (!mountedRef.current) return;
      setAdminKey(result.adminKey);
      setShowAdminKey(true);
    } catch (err) {
      handleSetupError(err);
    } finally {
      finishAction();
    }
  }, [
    beginAction,
    bootstrapRequired,
    bootstrapToken,
    detectedBase,
    finishAction,
    handleSetupError,
    setupAuth,
    t,
  ]);

  const handleSlimCPAChoice = useCallback(
    async (action: 'download_latest' | 'use_existing') => {
      if (!validateUsageSetupStep('slim')) return;
      if (!beginAction()) return;
      setError('');
      setErrorDocsUrl('');
      try {
        const result = await usageServiceApi.selectSlimCPA(detectedBase, action, setupAuth);
        publishSetupStateChanged();
        if (!mountedRef.current) return;
        if (action === 'download_latest') {
          setDeploymentMode('integrated');
          setSetupIncludesCPA(false);
          setUsageSetupStep(adminReady ? 'complete' : 'admin');
          showNotification(
            result.installed?.version
              ? t('login.slim_download_complete_version', {
                  version: result.installed.version,
                })
              : t('login.slim_download_complete'),
            'success'
          );
          return;
        }
        setUsageSetupStep('connection');
      } catch (err) {
        handleSetupError(err);
      } finally {
        finishAction();
      }
    },
    [
      adminReady,
      beginAction,
      detectedBase,
      finishAction,
      handleSetupError,
      setupAuth,
      showNotification,
      t,
      validateUsageSetupStep,
    ]
  );

  const handleUsageSetupNext = useCallback(async () => {
    if (!validateUsageSetupStep(usageSetupStep)) return;
    if (!beginAction()) return;
    setError('');
    setErrorDocsUrl('');
    try {
      if (usageSetupStep === 'slim') {
        return;
      }
      if (usageSetupStep === 'connection') {
        const baseToUse = normalizeApiBase(apiBase);
        await usageServiceApi.setup(
          detectedBase,
          {
            cpaBaseUrl: baseToUse,
            cpaManagementKey: cpaManagementKey.trim(),
            requestMonitoringEnabled: true,
            ensureUsageStatisticsEnabled: true,
          },
          setupAuth
        );
        writeLocalStorage(USAGE_SERVICE_LAST_CPA_BASE_KEY, baseToUse);
        publishSetupStateChanged();
        if (!mountedRef.current) return;
        setUsageSetupStep(adminReady ? 'complete' : 'admin');
        return;
      }
      if (usageSetupStep === 'admin') {
        await usageServiceApi.initializeAdminKey(detectedBase, adminKey.trim(), setupAuth);
        publishSetupStateChanged();
        if (!mountedRef.current) return;
        setAdminReady(true);
        setBootstrapRequired(false);
        setBootstrapToken('');
        setUsageSetupStep('complete');
      }
    } catch (err) {
      handleSetupError(err);
    } finally {
      finishAction();
    }
  }, [
    adminKey,
    adminReady,
    apiBase,
    beginAction,
    cpaManagementKey,
    detectedBase,
    finishAction,
    handleSetupError,
    setupAuth,
    usageSetupStep,
    validateUsageSetupStep,
  ]);

  const handleUsageSetupBack = useCallback(() => {
    if (actionInFlightRef.current) return;
    setError('');
    setErrorDocsUrl('');
    const currentIndex = usageSetupSteps.indexOf(usageSetupStep);
    const previousStep = usageSetupSteps[Math.max(currentIndex - 1, 0)];
    setUsageSetupStep(previousStep);
  }, [usageSetupStep, usageSetupSteps]);

  const handleSubmit = useCallback(async () => {
    if (usageServiceNeedsSetup && !usageSetupIsLastStep) {
      await handleUsageSetupNext();
      return;
    }

    const trimmedAdminKey = adminKey.trim();
    const trimmedCPAKey = cpaManagementKey.trim();
    const baseToUse = apiBase ? normalizeApiBase(apiBase) : detectedBase;

    if (usageServiceNeedsSetup || isManagerServerMode) {
      if (!trimmedAdminKey) {
        setError(t('login.admin_key_required'));
        return;
      }
    } else if (!trimmedCPAKey) {
      setError(t('login.cpa_management_key_required'));
      return;
    }

    if (!beginAction()) return;
    setError('');
    setErrorDocsUrl('');
    try {
      if (usageServiceNeedsSetup) {
        setUsageServiceConfig(
          { enabled: true, serviceBase: detectedBase },
          { panelBase: detectedBase, panelHostMode: 'manager_embedded' }
        );
      } else if (isManagerServerMode) {
        setUsageServiceConfig(
          { enabled: true, serviceBase: detectedBase },
          { panelBase: detectedBase, panelHostMode: 'manager_embedded' }
        );
      }

      const loginResult = await login({
        apiBase: isManagerServerMode ? detectedBase : baseToUse,
        managementKey: isManagerServerMode ? trimmedAdminKey : trimmedCPAKey,
        rememberPassword: rememberCredential,
        sessionMode: isManagerServerMode ? 'manager_embedded' : 'external_panel',
        sessionPanelBase: detectedBase,
      });
      if (!mountedRef.current) return;
      showNotification(t('common.connected_status'), 'success');
      if (loginResult.recoveryMode === 'manager_config') {
        writeLocalStorage(CONFIG_TAB_STORAGE_KEY, 'manager');
        navigate('/config', { replace: true });
      } else {
        navigate('/', { replace: true });
      }
    } catch (err: unknown) {
      if (!mountedRef.current) return;
      const message = getLocalizedErrorMessage(err, t);
      setError(message);
      setErrorDocsUrl(
        typeof (err as { docsUrl?: unknown })?.docsUrl === 'string'
          ? localizeSetupDocsUrl((err as { docsUrl: string }).docsUrl ?? '', language)
          : ''
      );
      showNotification(`${t('notification.login_failed')}: ${message}`, 'error');
    } finally {
      finishAction();
    }
  }, [
    adminKey,
    apiBase,
    beginAction,
    cpaManagementKey,
    detectedBase,
    finishAction,
    handleUsageSetupNext,
    isManagerServerMode,
    language,
    login,
    navigate,
    rememberCredential,
    setUsageServiceConfig,
    showNotification,
    t,
    usageServiceNeedsSetup,
    usageSetupIsLastStep,
  ]);

  const handleSubmitKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Enter' && !loading) {
        event.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit, loading]
  );

  if (isAuthenticated && !autoLoading && !autoLoginSuccess) {
    const redirect = (location.state as RedirectState | null)?.from?.pathname || '/';
    return <Navigate to={redirect} replace />;
  }

  const showSplash = autoLoading || autoLoginSuccess;

  const renderKeyToggle = (visible: boolean, toggle: () => void) => (
    <button
      type="button"
      className="btn btn-ghost btn-xs btn-icon-only"
      onClick={toggle}
      aria-label={visible ? t('login.hide_key') : t('login.show_key')}
      title={visible ? t('login.hide_key') : t('login.show_key')}
    >
      {visible ? <IconEyeOff size={16} /> : <IconEye size={16} />}
    </button>
  );

  return (
    <div className={styles.container}>
      <div className={styles.toolBar}>
        <button
          type="button"
          className={styles.toolButton}
          onClick={cycleTheme}
          aria-label={t('theme.switch')}
          title={t('theme.switch')}
        >
          {theme === 'dark' ? <IconMoon size={17} /> : <IconSun size={17} />}
        </button>
        <div className={styles.languageMenu} ref={languageMenuRef}>
          <button
            type="button"
            className={styles.toolButton}
            onClick={toggleLanguageMenu}
            aria-label={t('language.switch')}
            title={t('language.switch')}
            aria-haspopup="menu"
            aria-expanded={languageMenuOpen}
          >
            <IconLanguages size={17} />
          </button>
          {languageMenuOpen && (
            <div className={styles.languagePopover} role="menu" aria-label={t('language.switch')}>
              {LANGUAGE_ORDER.map((lang) => (
                <button
                  key={lang}
                  type="button"
                  className={`${styles.languageOption} ${
                    language === lang ? styles.languageOptionActive : ''
                  }`}
                  onClick={() => handleLanguageSelect(lang)}
                  role="menuitemradio"
                  aria-checked={language === lang}
                >
                  {t(LANGUAGE_LABEL_KEYS[lang])}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className={styles.formPanel}>
        {showSplash ? (
          <div className={styles.splashContent}>
            <img src={INLINE_LOGO_JPEG} alt="CPAMP" className={styles.splashLogo} />
            <h1 className={styles.splashTitle}>{t('splash.title')}</h1>
            <p className={styles.splashSubtitle}>{t('splash.subtitle')}</p>
            <div className={styles.splashLoader}>
              <div className={styles.splashLoaderBar} />
            </div>
          </div>
        ) : (
          <div
            className={`${styles.formContent} ${
              usageServiceNeedsSetup ? styles.setupFormContent : ''
            }`}
          >
            <div
              className={`${styles.loginCard} ${usageServiceNeedsSetup ? styles.setupCard : ''}`}
            >
              <div className={styles.cardBranding}>
                <img src={INLINE_LOGO_JPEG} alt="CPA Manager Plus" className={styles.logo} />
                <h1>CPA Manager Plus</h1>
                <p>
                  {usageServiceNeedsSetup
                    ? t('login.docker_setup_subtitle')
                    : isManagerServerMode
                      ? t('login.docker_login_subtitle')
                      : t('login.subtitle')}
                </p>
              </div>

              {usageServiceNeedsSetup && (
                <div className={styles.setupFlow}>
                  <div className={styles.stepper} role="list" aria-label={t('login.setup_steps')}>
                    {usageSetupSteps.map((step, index) => {
                      const isActive = index === usageSetupStepIndex;
                      const isDone = index < usageSetupStepIndex;
                      return (
                        <div
                          key={step}
                          className={`${styles.stepItem} ${isActive ? styles.stepItemActive : ''} ${
                            isDone ? styles.stepItemDone : ''
                          }`}
                          role="listitem"
                          aria-current={isActive ? 'step' : undefined}
                        >
                          <span className={styles.stepIndex}>
                            {isDone ? <IconCheck size={18} /> : index + 1}
                          </span>
                          <span className={styles.stepLabel}>{usageSetupStepLabels[step]}</span>
                        </div>
                      );
                    })}
                  </div>

                  <div className={styles.stepPanel}>
                    <div className={styles.stepHeader}>
                      <span className={styles.stepEyebrow}>
                        {t('login.step_count', {
                          current: usageSetupStepIndex + 1,
                          total: usageSetupSteps.length,
                        })}
                      </span>
                      <h2>{usageSetupStepLabels[usageSetupStep]}</h2>
                    </div>

                    {bootstrapRequired && usageSetupStep !== 'complete' && (
                      <div className={styles.stepFields}>
                        <Input
                          label={t('login.bootstrap_token_label')}
                          placeholder={t('login.bootstrap_token_placeholder')}
                          type="password"
                          value={bootstrapToken}
                          onChange={(event) => setBootstrapToken(event.target.value)}
                          hint={t('login.bootstrap_token_hint')}
                        />
                      </div>
                    )}

                    {usageSetupStep === 'slim' && (
                      <div className={styles.stepFields}>
                        {adminReady && (
                          <Input
                            autoFocus
                            label={t('login.admin_key_label')}
                            placeholder={t('login.admin_key_placeholder')}
                            type={showAdminKey ? 'text' : 'password'}
                            value={adminKey}
                            onChange={(event) => setAdminKey(event.target.value)}
                            hint={t('login.existing_admin_key_hint')}
                            rightElement={renderKeyToggle(showAdminKey, () =>
                              setShowAdminKey((prev) => !prev)
                            )}
                          />
                        )}
                        <div className={styles.connectionBox}>
                          <div className={styles.connectionIcon}>
                            <IconInfo size={18} />
                          </div>
                          <div className={styles.connectionCopy}>
                            <div className={styles.label}>{t('login.slim_choice_title')}</div>
                            <div className={styles.hint}>{t('login.slim_choice_hint')}</div>
                          </div>
                        </div>
                        <Button
                          fullWidth
                          onClick={() => void handleSlimCPAChoice('download_latest')}
                          loading={loading}
                        >
                          {loading
                            ? t('login.slim_downloading_latest')
                            : t('login.slim_download_latest')}
                        </Button>
                        <Button
                          fullWidth
                          variant="secondary"
                          onClick={() => void handleSlimCPAChoice('use_existing')}
                          disabled={loading}
                        >
                          {t('login.slim_use_existing')}
                        </Button>
                        <p className={styles.slimSecurityHint}>
                          {t('login.slim_download_security_hint')}
                        </p>
                      </div>
                    )}

                    {usageSetupStep === 'connection' && (
                      <div className={styles.stepFields}>
                        {adminReady && (
                          <Input
                            label={t('login.admin_key_label')}
                            placeholder={t('login.admin_key_placeholder')}
                            type={showAdminKey ? 'text' : 'password'}
                            value={adminKey}
                            onChange={(event) => setAdminKey(event.target.value)}
                            hint={t('login.existing_admin_key_hint')}
                            rightElement={renderKeyToggle(showAdminKey, () =>
                              setShowAdminKey((prev) => !prev)
                            )}
                          />
                        )}
                        <Input
                          autoFocus={!adminReady}
                          label={t('login.cpa_connection_label')}
                          placeholder={t('login.cpa_connection_placeholder')}
                          value={apiBase}
                          onChange={(event) => setApiBase(event.target.value)}
                          onKeyDown={handleSubmitKeyDown}
                          hint={t('login.cpa_connection_hint')}
                        />
                        <Input
                          label={t('login.cpa_management_key_label')}
                          placeholder={t('login.cpa_management_key_placeholder')}
                          type={showCPAManagementKey ? 'text' : 'password'}
                          value={cpaManagementKey}
                          onChange={(event) => setCPAManagementKey(event.target.value)}
                          onKeyDown={handleSubmitKeyDown}
                          hint={t('login.cpa_management_key_hint')}
                          rightElement={renderKeyToggle(showCPAManagementKey, () =>
                            setShowCPAManagementKey((prev) => !prev)
                          )}
                        />
                      </div>
                    )}

                    {usageSetupStep === 'admin' && (
                      <div className={styles.stepFields}>
                        <div className={styles.connectionBox}>
                          <div className={styles.connectionIcon}>
                            <IconShield size={18} />
                          </div>
                          <div className={styles.connectionCopy}>
                            <div className={styles.label}>{t('login.usage_service_address')}</div>
                            <div className={styles.value}>{detectedBase}</div>
                            <div className={styles.hint}>
                              {hasHistoricalData || migrationStatus
                                ? t('login.migration_detected_hint')
                                : t('login.admin_key_setup_hint')}
                            </div>
                          </div>
                        </div>
                        <Input
                          autoFocus
                          label={t('login.admin_key_label')}
                          placeholder={t('login.admin_key_placeholder')}
                          type={showAdminKey ? 'text' : 'password'}
                          value={adminKey}
                          onChange={(event) => setAdminKey(event.target.value)}
                          onKeyDown={handleSubmitKeyDown}
                          hint={t('login.admin_key_policy')}
                          rightElement={renderKeyToggle(showAdminKey, () =>
                            setShowAdminKey((prev) => !prev)
                          )}
                        />
                        <Button
                          variant="secondary"
                          onClick={handleGenerateAdminKey}
                          disabled={loading}
                        >
                          {t('login.generate_admin_key')}
                        </Button>
                      </div>
                    )}

                    {usageSetupStep === 'complete' && (
                      <div className={styles.stepFields}>
                        <div className={styles.connectionBox}>
                          <div className={styles.connectionIcon}>
                            <IconCheck size={18} />
                          </div>
                          <div className={styles.connectionCopy}>
                            <div className={styles.label}>{t('login.setup_complete_title')}</div>
                            <div className={styles.hint}>{t('login.setup_complete_hint')}</div>
                          </div>
                        </div>
                        <div className={styles.optionBox}>
                          <SelectionCheckbox
                            checked={rememberCredential}
                            onChange={setRememberCredential}
                            ariaLabel={t('login.remember_credential_label')}
                            label={t('login.remember_credential_label')}
                            labelClassName={styles.toggleLabel}
                          />
                        </div>
                      </div>
                    )}
                  </div>

                  {error && (
                    <div className={styles.errorBox} role="alert">
                      <div>{error}</div>
                      {errorDocsUrl && (
                        <a href={errorDocsUrl} target="_blank" rel="noreferrer">
                          {t('login.open_solution_docs')}
                        </a>
                      )}
                    </div>
                  )}

                  <div className={styles.stepActions}>
                    <Button
                      variant="secondary"
                      className={styles.setupBackButton}
                      onClick={handleUsageSetupBack}
                      disabled={usageSetupIsFirstStep || loading}
                    >
                      {t('common.previous')}
                    </Button>
                    {usageSetupStep === 'slim' ? null : usageSetupIsLastStep ? (
                      <Button
                        className={styles.setupNextButton}
                        onClick={handleSubmit}
                        loading={loading}
                      >
                        {loading ? t('login.initializing') : t('login.enter_management')}
                      </Button>
                    ) : (
                      <Button
                        className={styles.setupNextButton}
                        onClick={handleUsageSetupNext}
                        disabled={loading}
                      >
                        {t('common.next')}
                      </Button>
                    )}
                  </div>
                </div>
              )}

              {!usageServiceNeedsSetup && (
                <div className={styles.loginForm}>
                  <div className={styles.connectionBox}>
                    <div className={styles.label}>{t('login.connection_current')}</div>
                    <div className={styles.value}>{apiBase || detectedBase}</div>
                    <div className={styles.hint}>
                      {isManagerServerMode
                        ? t('login.usage_service_configured_hint')
                        : t('login.connection_auto_hint')}
                    </div>
                  </div>

                  {!isManagerServerMode && (
                    <>
                      <div className={styles.toggleAdvanced}>
                        <SelectionCheckbox
                          checked={showCustomBase}
                          onChange={setShowCustomBase}
                          ariaLabel={t('login.custom_connection_label')}
                          label={t('login.custom_connection_label')}
                          labelClassName={styles.toggleLabel}
                        />
                      </div>

                      {showCustomBase && (
                        <Input
                          label={t('login.custom_connection_label')}
                          placeholder={t('login.custom_connection_placeholder')}
                          value={apiBase}
                          onChange={(event) => setApiBase(event.target.value)}
                          hint={t('login.custom_connection_hint')}
                        />
                      )}
                    </>
                  )}

                  <Input
                    autoFocus
                    label={loginCredentialLabel}
                    placeholder={loginCredentialPlaceholder}
                    type={
                      (isManagerServerMode ? showAdminKey : showCPAManagementKey)
                        ? 'text'
                        : 'password'
                    }
                    value={loginCredential}
                    onChange={(event) =>
                      isManagerServerMode
                        ? setAdminKey(event.target.value)
                        : setCPAManagementKey(event.target.value)
                    }
                    onKeyDown={handleSubmitKeyDown}
                    hint={loginCredentialHint}
                    rightElement={renderKeyToggle(
                      isManagerServerMode ? showAdminKey : showCPAManagementKey,
                      () =>
                        isManagerServerMode
                          ? setShowAdminKey((prev) => !prev)
                          : setShowCPAManagementKey((prev) => !prev)
                    )}
                  />

                  <div className={styles.toggleAdvanced}>
                    <SelectionCheckbox
                      checked={rememberCredential}
                      onChange={setRememberCredential}
                      ariaLabel={t('login.remember_credential_label')}
                      label={t('login.remember_credential_label')}
                      labelClassName={styles.toggleLabel}
                    />
                  </div>

                  <Button fullWidth onClick={handleSubmit} loading={loading}>
                    {loading ? t('login.submitting') : t('login.submit_button')}
                  </Button>

                  {error && (
                    <div className={styles.errorBox} role="alert">
                      {error}
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
