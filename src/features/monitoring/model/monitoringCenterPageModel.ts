import type { TFunction } from 'i18next';
import { requestCodexUsagePayload } from '@/services/api';
import type { AuthFileItem, CodexUsagePayload } from '@/types';
import type {
  MonitoringAccountRow,
  MonitoringEventRow,
  MonitoringSummary,
} from '@/features/monitoring/hooks/useMonitoringData';
import type { AccountSortKey } from '@/features/monitoring/accountOverviewState';
import type {
  AccountQuotaEntry,
  AccountQuotaWindow,
} from '@/features/monitoring/components/accountOverviewPresentation';
import { formatPercent } from '@/features/monitoring/components/accountOverviewPresentation';
import type { SummaryCardProps } from '@/features/monitoring/components/MonitoringShared';
import type { MonitoringAccountQuotaTarget } from '@/features/monitoring/accountOverviewQuotaTargets';
import { formatStatusWindowLabel } from '@/features/monitoring/model/statusWindow';
import { buildCodexQuotaWindowInfos, normalizePlanType } from '@/utils/quota';
import {
  formatCompactNumber,
  formatDurationMs,
  formatUsd,
  normalizeAuthIndex,
  type ModelPrice,
} from '@/utils/usage';

export type StatusFilter = 'all' | 'success' | 'failed';

export type FocusSnapshot = {
  searchInput: string;
  selectedAccount: string;
  selectedProvider: string;
  selectedModel: string;
  selectedChannel: string;
  selectedApiKeyHash: string;
  selectedStatus: StatusFilter;
};

export type PriceDraft = {
  prompt: string;
  completion: string;
  cache: string;
};

export type RealtimeLogRow = MonitoringEventRow & {
  requestCount: number;
  successRate: number;
  streamKey: string;
  recentPattern: boolean[];
};

export type AccountOverviewColumn = {
  key: string;
  label: string;
  sortKey?: AccountSortKey;
};

export type MonitoringOption = {
  value: string;
  label: string;
};

export type PaginationState<T> = {
  currentPage: number;
  totalPages: number;
  pageItems: T[];
  startItem: number;
  endItem: number;
};

const padDateUnit = (value: number) => String(value).padStart(2, '0');

export const formatDateTimeLocalValue = (date: Date) =>
  `${date.getFullYear()}-${padDateUnit(date.getMonth() + 1)}-${padDateUnit(date.getDate())}T${padDateUnit(date.getHours())}:${padDateUnit(date.getMinutes())}`;

export const getTodayStartInputValue = () => {
  const date = new Date();
  date.setHours(0, 0, 0, 0);
  return formatDateTimeLocalValue(date);
};

export const getCurrentInputValue = () => formatDateTimeLocalValue(new Date());

export const parseDateTimeLocalValue = (value: string) => {
  if (!value) return null;
  const timestamp = new Date(value).getTime();
  return Number.isFinite(timestamp) ? timestamp : null;
};

export const ensureSelectedOption = <T extends { value: string; label: string }>(
  options: T[],
  value: string,
  label = value
): T[] => {
  if (!value || value === 'all' || options.some((option) => option.value === value)) {
    return options;
  }
  return [...options, { value, label } as T];
};

const buildSortedValueOptions = (values: string[]): MonitoringOption[] =>
  Array.from(new Set(values))
    .filter(Boolean)
    .sort((left, right) => left.localeCompare(right))
    .map((value) => ({ value, label: value }));

export const buildProviderOptions = (
  rows: MonitoringEventRow[],
  selectedProvider: string,
  t: TFunction
) =>
  ensureSelectedOption(
    [
      { value: 'all', label: t('monitoring.filter_all_providers') },
      ...buildSortedValueOptions(rows.map((row) => row.provider)),
    ],
    selectedProvider
  );

export const buildAccountOptions = (
  rows: MonitoringAccountRow[],
  selectedAccount: string,
  t: TFunction
) =>
  ensureSelectedOption(
    [
      { value: 'all', label: t('monitoring.filter_all_accounts') },
      ...Array.from(
        new Map(rows.map((row) => [row.account, buildAccountOptionLabel(row)])).entries()
      )
        .sort((left, right) => left[1].localeCompare(right[1]))
        .map(([value, label]) => ({ value, label })),
    ],
    selectedAccount
  );

export const buildModelOptions = (
  rows: MonitoringEventRow[],
  selectedModel: string,
  t: TFunction
) =>
  ensureSelectedOption(
    [
      { value: 'all', label: t('monitoring.filter_all_models') },
      ...buildSortedValueOptions(rows.map((row) => row.model)),
    ],
    selectedModel
  );

export const buildChannelOptions = (
  rows: MonitoringEventRow[],
  selectedChannel: string,
  t: TFunction
) =>
  ensureSelectedOption(
    [
      { value: 'all', label: t('monitoring.filter_all_channels') },
      ...buildSortedValueOptions(rows.map((row) => row.channel)),
    ],
    selectedChannel
  );

export const buildApiKeyOptions = (
  rows: MonitoringEventRow[],
  selectedApiKeyHash: string,
  t: TFunction
) => {
  const optionMap = new Map<string, string>();
  rows.forEach((row) => {
    if (!row.apiKeyHash || optionMap.has(row.apiKeyHash)) return;
    optionMap.set(row.apiKeyHash, row.apiKeyLabel || row.apiKeyMasked || row.apiKeyHash);
  });

  return ensureSelectedOption(
    [
      { value: 'all', label: t('monitoring.filter_all_api_keys') },
      ...Array.from(optionMap.entries())
        .sort((left, right) => left[1].localeCompare(right[1]))
        .map(([value, label]) => ({ value, label })),
    ],
    selectedApiKeyHash,
    selectedApiKeyHash
  );
};

export const buildStatusOptions = (t: TFunction): MonitoringOption[] => [
  { value: 'all', label: t('monitoring.filter_all_statuses') },
  { value: 'success', label: t('monitoring.filter_status_success') },
  { value: 'failed', label: t('monitoring.filter_status_failed') },
];

export const buildSyncPriceModels = (
  rows: MonitoringEventRow[],
  modelPrices: Record<string, ModelPrice>
) =>
  Array.from(new Set([...rows.map((row) => row.model), ...Object.keys(modelPrices)]))
    .filter(Boolean)
    .sort((left, right) => left.localeCompare(right));

export const buildPriceModelOptions = (models: string[], t: TFunction): MonitoringOption[] => [
  { value: '', label: t('usage_stats.model_price_select_placeholder') },
  ...models.map((value) => ({ value, label: value })),
];

export const buildAuthFilesByAuthIndex = (authFiles: AuthFileItem[]) => {
  const map = new Map<string, AuthFileItem>();
  authFiles.forEach((file) => {
    const authIndex = normalizeAuthIndex(file['auth_index'] ?? file.authIndex);
    if (!authIndex || map.has(authIndex)) return;
    map.set(authIndex, file);
  });
  return map;
};

export const buildAccountOverviewColumns = (t: TFunction): AccountOverviewColumn[] => [
  { key: 'account', label: t('monitoring.account_overview_col_account') },
  { key: 'status', label: t('monitoring.column_status') },
  { key: 'total-calls', label: t('monitoring.total_calls'), sortKey: 'totalCalls' },
  {
    key: 'success-calls',
    label: t('monitoring.account_overview_col_success'),
    sortKey: 'successCalls',
  },
  {
    key: 'failure-calls',
    label: t('monitoring.account_overview_col_failure'),
    sortKey: 'failureCalls',
  },
  { key: 'success-rate', label: t('monitoring.column_success_rate'), sortKey: 'successRate' },
  { key: 'total-tokens', label: t('monitoring.total_tokens'), sortKey: 'totalTokens' },
  {
    key: 'estimated-cost',
    label: t('monitoring.account_overview_col_cost'),
    sortKey: 'totalCost',
  },
  {
    key: 'latest-request-time',
    label: t('monitoring.latest_request_time'),
    sortKey: 'lastSeenAt',
  },
  { key: 'action', label: t('common.action') },
];

export const buildApiKeyOverviewColumns = (t: TFunction): AccountOverviewColumn[] => [
  { key: 'api-key', label: t('monitoring.api_key_summary_col_key') },
  { key: 'total-calls', label: t('monitoring.total_calls') },
  { key: 'success-calls', label: t('monitoring.account_overview_col_success') },
  { key: 'failure-calls', label: t('monitoring.account_overview_col_failure') },
  { key: 'total-tokens', label: t('monitoring.total_tokens') },
  { key: 'estimated-cost', label: t('monitoring.account_overview_col_cost') },
  { key: 'latest-request-time', label: t('monitoring.latest_request_time') },
];

export const buildAccountSortOptions = (
  columns: AccountOverviewColumn[],
  t: TFunction
): MonitoringOption[] => {
  const prefix = t('monitoring.account_overview_sort_prefix');
  return columns
    .filter((column): column is AccountOverviewColumn & { sortKey: AccountSortKey } =>
      Boolean(column.sortKey)
    )
    .map((column) => ({
      value: column.sortKey,
      label: `${prefix}${column.label}`,
    }));
};

export const buildPrimarySummaryCards = ({
  summary,
  accountCount,
  failedGroupCount,
  hasPrices,
  locale,
  t,
}: {
  summary: MonitoringSummary;
  accountCount: number;
  failedGroupCount: number;
  hasPrices: boolean;
  locale: string;
  t: TFunction;
}): SummaryCardProps[] => [
  {
    label: t('monitoring.total_calls'),
    value: formatCompactNumber(summary.totalCalls),
    meta: `${accountCount} ${t('monitoring.accounts_suffix')}`,
  },
  {
    label: t('monitoring.call_success_rate'),
    value: formatPercent(summary.successRate),
    meta: formatDurationMs(summary.averageLatencyMs, { locale }),
    tone: summary.successRate >= 0.95 ? 'good' : summary.successRate >= 0.85 ? 'warn' : 'bad',
  },
  {
    label: t('monitoring.failure_calls'),
    value: formatCompactNumber(summary.failureCalls),
    meta: `${failedGroupCount} ${t('monitoring.groups_suffix')}`,
    tone: summary.failureCalls > 0 ? 'bad' : 'good',
  },
  {
    label: t('monitoring.estimated_cost'),
    value: hasPrices ? formatUsd(summary.totalCost) : '--',
    meta: hasPrices ? t('monitoring.estimated_cost_hint') : t('monitoring.estimated_cost_missing'),
    tone: hasPrices ? undefined : 'warn',
  },
];

export const buildSecondarySummaryCards = (
  summary: MonitoringSummary,
  t: TFunction
): SummaryCardProps[] => [
  {
    label: t('monitoring.total_tokens'),
    value: formatCompactNumber(summary.totalTokens),
    meta: `${t('monitoring.reasoning_tokens')} ${formatCompactNumber(summary.reasoningTokens)}`,
    variant: 'secondary',
  },
  {
    label: t('monitoring.input_tokens'),
    value: formatCompactNumber(summary.inputTokens),
    meta: `${t('monitoring.of_token_mix')} ${formatPercent(summary.totalTokens > 0 ? summary.inputTokens / summary.totalTokens : 0)}`,
    variant: 'secondary',
  },
  {
    label: t('monitoring.output_tokens'),
    value: formatCompactNumber(summary.outputTokens),
    meta: `${t('monitoring.of_token_mix')} ${formatPercent(summary.totalTokens > 0 ? summary.outputTokens / summary.totalTokens : 0)}`,
    variant: 'secondary',
  },
  {
    label: t('monitoring.cached_tokens'),
    value: formatCompactNumber(summary.cachedTokens),
    meta: `${t('monitoring.of_input_tokens')} ${formatPercent(summary.inputTokens > 0 ? summary.cachedTokens / summary.inputTokens : 0)}`,
    variant: 'secondary',
  },
];

export const isUsageImportFile = (file: File) => {
  const normalizedName = file.name.toLowerCase();
  const normalizedType = file.type.toLowerCase();
  return (
    /\.(json|jsonl|ndjson|txt)$/.test(normalizedName) ||
    normalizedType === 'application/json' ||
    normalizedType === 'application/x-ndjson' ||
    normalizedType === 'text/plain'
  );
};

export const buildPaginationState = <T,>(
  items: readonly T[],
  page: number,
  pageSize: number
): PaginationState<T> => {
  const safePageSize = Math.max(1, pageSize);
  const totalPages = Math.max(1, Math.ceil(items.length / safePageSize));
  const currentPage = Math.min(Math.max(1, page), totalPages);
  const startIndex = (currentPage - 1) * safePageSize;
  const endIndex = Math.min(startIndex + safePageSize, items.length);

  return {
    currentPage,
    totalPages,
    pageItems: items.slice(startIndex, endIndex),
    startItem: items.length > 0 ? startIndex + 1 : 0,
    endItem: endIndex,
  };
};

export const createPriceDraft = (price?: ModelPrice): PriceDraft => ({
  prompt: price ? String(price.prompt) : '',
  completion: price ? String(price.completion) : '',
  cache: price ? String(price.cache) : '',
});

export const parsePriceValue = (value: string) => {
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : 0;
};

export const buildAccountOptionLabel = (row: MonitoringAccountRow) => {
  if (!row.displayAccount || row.displayAccount === row.account) {
    return row.account;
  }
  return `${row.displayAccount} / ${row.account}`;
};

export const buildAccountQuotaWindows = (
  payload: CodexUsagePayload,
  t: TFunction
): AccountQuotaWindow[] =>
  buildCodexQuotaWindowInfos(payload).map((window) => {
    const clampedUsed =
      window.usedPercent === null ? null : Math.max(0, Math.min(100, window.usedPercent));
    const remainingPercent = clampedUsed === null ? null : Math.max(0, 100 - clampedUsed);
    let usageLabel: string | null = null;

    if (
      window.limitWindowSeconds !== null &&
      window.limitWindowSeconds > 0 &&
      clampedUsed !== null
    ) {
      const totalHours = window.limitWindowSeconds / 3600;
      const usedHours = (totalHours * clampedUsed) / 100;
      const formatHours = (value: number) =>
        Number.isInteger(value) ? value.toFixed(0) : value.toFixed(1);
      usageLabel = t('codex_quota.window_usage', {
        used: formatHours(usedHours),
        total: formatHours(totalHours),
      });
    }

    return {
      id: window.id,
      label: t(window.labelKey, window.labelParams),
      remainingPercent,
      resetLabel: window.resetLabel,
      usageLabel,
    };
  });

export const requestAccountQuota = async (
  target: MonitoringAccountQuotaTarget,
  t: TFunction
): Promise<AccountQuotaEntry> => {
  const payload = await requestCodexUsagePayload(
    {
      authIndex: target.authIndex,
      accountId: target.accountId,
    },
    { emptyMessage: t('codex_quota.empty_windows') }
  );

  return {
    key: target.key,
    authLabel: target.authLabel,
    fileName: target.fileName,
    planType: normalizePlanType(payload.plan_type ?? payload.planType) ?? target.planType,
    windows: buildAccountQuotaWindows(payload, t),
  };
};

export const buildRealtimeLogRows = (rows: MonitoringEventRow[]): RealtimeLogRow[] => {
  const sortedAsc = [...rows].sort(
    (left, right) => left.timestampMs - right.timestampMs || left.id.localeCompare(right.id)
  );
  const metricsByStream = new Map<string, { total: number; success: number; pattern: boolean[] }>();

  const enriched = sortedAsc.map((row) => {
    const streamKey = [row.account, row.provider, row.model, row.channel].join('::');
    const previous = metricsByStream.get(streamKey) ?? { total: 0, success: 0, pattern: [] };
    const nextPattern = [...previous.pattern, !row.failed].slice(-10);
    const next = {
      total: previous.total + (row.statsIncluded ? 1 : 0),
      success: previous.success + (row.statsIncluded && !row.failed ? 1 : 0),
      pattern: nextPattern,
    };
    metricsByStream.set(streamKey, next);

    return {
      ...row,
      streamKey,
      requestCount: next.total,
      successRate: next.total > 0 ? next.success / next.total : 1,
      recentPattern: nextPattern,
    } satisfies RealtimeLogRow;
  });

  return enriched.sort(
    (left, right) =>
      right.timestampMs - left.timestampMs ||
      right.requestCount - left.requestCount ||
      right.id.localeCompare(left.id)
  );
};

export const formatAccountOverviewScopeText = (
  bounds: { startMs: number; endMs: number } | null,
  locale: string,
  t: TFunction
) => {
  if (!bounds) {
    return t('monitoring.account_overview_scope_current_filters');
  }

  const rangeLabel =
    Number.isFinite(bounds.startMs) && Number.isFinite(bounds.endMs)
      ? formatStatusWindowLabel(bounds.startMs, bounds.endMs, locale)
      : t('monitoring.range_all');

  return t('monitoring.account_overview_scope_range', { range: rangeLabel });
};
