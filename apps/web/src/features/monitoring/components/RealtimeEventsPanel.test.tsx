import { renderToStaticMarkup } from 'react-dom/server';
import type { TFunction } from 'i18next';
import { describe, expect, it, vi } from 'vitest';
import type { MonitoringEventRow } from '@/features/monitoring/hooks/useMonitoringData';
import { RealtimeEventsPanel } from './RealtimeEventsPanel';

const t = ((key: string, options?: Record<string, string>) => {
  const messages: Record<string, string> = {
    'common.loading': 'Loading',
    'common.copy': 'Copy',
    'monitoring.cache_creation_tokens_short': 'Create',
    'monitoring.cache_read_tokens_short': 'Read',
    'monitoring.column_latency': 'Latency',
    'monitoring.column_model': 'Model',
    'monitoring.column_success_rate': 'Success',
    'monitoring.column_time': 'Time',
    'monitoring.column_type': 'Type',
    'monitoring.fail_status_code_short': 'HTTP',
    'monitoring.filter_status_failed': 'Failed only',
    'monitoring.load_more_events': 'Load more',
    'monitoring.log_rows': 'Rows',
    'monitoring.no_more_events': 'No more events',
    'monitoring.reasoning_effort': 'Effort',
    'monitoring.reasoning_effort_short': 'Effort',
    'monitoring.recent_failures': 'Failures',
    'monitoring.recent_status': 'Recent',
    'monitoring.request_status': 'Status',
    'monitoring.result_failed': 'Failed',
    'monitoring.result_success': 'Success',
    'monitoring.this_call_cost': 'Cost',
    'monitoring.this_call_usage': 'Usage',
  };
  if (key === 'monitoring.resolved_model_label') {
    return `Resolved ${options?.model ?? ''}`.trim();
  }
  return messages[key] ?? key;
}) as unknown as TFunction;

const noop = vi.fn();

type PanelRow = MonitoringEventRow & {
  requestCount: number;
  successRate: number;
  streamKey: string;
  recentPattern: boolean[];
};

const baseRow = (overrides: Partial<PanelRow> = {}): PanelRow => ({
  id: 'row-1',
  timestamp: '2026-04-25T00:00:00Z',
  timestampMs: Date.UTC(2026, 3, 25),
  dayKey: '2026-04-25',
  hourLabel: '00:00',
  model: 'client-gpt',
  resolvedModel: 'gpt-5.4',
  endpoint: 'POST /v1/chat/completions',
  endpointMethod: 'POST',
  endpointPath: '/v1/chat/completions',
  sourceKey: 'source:user@example.com',
  source: 'user@example.com',
  sourceMasked: 'user@example.com',
  account: 'user@example.com',
  accountMasked: 'user@example.com',
  authIndex: '0',
  authIndexMasked: '0',
  authLabel: '0',
  projectId: '',
  apiKeyHash: '',
  apiKeyLabel: '-',
  apiKeyMasked: '-',
  provider: 'openai',
  planType: '-',
  channel: 'openai',
  channelHost: '-',
  channelDisabled: false,
  failed: false,
  statsIncluded: true,
  latencyMs: 1500,
  inputTokens: 10,
  outputTokens: 20,
  reasoningTokens: 3,
  cachedTokens: 5,
  cacheReadTokens: 0,
  cacheCreationTokens: 0,
  totalTokens: 33,
  totalCost: 0,
  taskKey: 'task-1',
  searchText: '',
  requestCount: 1,
  successRate: 1,
  streamKey: 'stream-1',
  recentPattern: [true],
  ...overrides,
});

const renderPanel = (row: PanelRow) =>
  renderToStaticMarkup(
    <RealtimeEventsPanel
      embedded
      rows={[row]}
      pagination={{
        currentPage: 1,
        totalPages: 1,
        pageItems: [row],
        startItem: 1,
        endItem: 1,
      }}
      pageSize={10}
      scopedFailureCount={row.failed ? 1 : 0}
      failedOnlyActive={false}
      eventsHasMore={false}
      eventsLoadingMore={false}
      overallLoading={false}
      hasPrices={false}
      locale="en-US"
      emptyState={<span>empty</span>}
      t={t}
      onToggleFailedOnly={noop}
      onPageChange={noop}
      onPageSizeChange={noop}
      onLoadMoreEvents={noop}
    />
  );

describe('RealtimeEventsPanel', () => {
  it('renders CPA v7.1.18 usage details for failed rows', () => {
    const markup = renderPanel(
      baseRow({
        failed: true,
        successRate: 0,
        reasoningEffort: 'medium',
        cacheReadTokens: 4,
        cacheCreationTokens: 1,
        failStatusCode: 429,
        failSummary: 'rate limit exceeded',
      })
    );

    expect(markup).toContain('<th>Effort</th>');
    expect(markup).toContain('medium');
    expect(markup).toContain('Failed');
    expect(markup).toContain('I 10 · O 20 · C 5');
    expect(markup).not.toContain('Read 4');
    expect(markup).not.toContain('Create 1');
    expect(markup).toContain('role="tooltip"');
    expect(markup).toContain('aria-describedby=');
    expect(markup).toContain('aria-label="HTTP 429 · rate limit exceeded"');
    expect(markup).toContain('aria-label="Copy"');
    expect(markup).toContain('HTTP 429');
    expect(markup).toContain('rate limit exceeded');
  });

  it('renders safe defaults when optional usage fields are missing', () => {
    const markup = renderPanel(baseRow());

    expect(markup).not.toContain('Effort -');
    expect(markup).toContain('<th>Effort</th>');
    expect(markup).toContain('Success');
    expect(markup).toContain('I 10 · O 20 · C 5');
    expect(markup).not.toContain('Read 0');
    expect(markup).not.toContain('Create 0');
    expect(markup).not.toContain('role="tooltip"');
    expect(markup).not.toContain('aria-describedby=');
    expect(markup).not.toContain('HTTP');
  });

  it('renders residual cached tokens even when they equal cache read tokens', () => {
    const markup = renderPanel(
      baseRow({
        cachedTokens: 4,
        cacheReadTokens: 4,
        cacheCreationTokens: 1,
      })
    );

    expect(markup).toContain('C 4');
    expect(markup).not.toContain('Read 4');
    expect(markup).not.toContain('Create 1');
  });
});
