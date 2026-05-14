import type {
  CodexInspectionAccountResult,
  CodexInspectionActionRecord,
  CodexInspectionNotificationRecord,
  CodexInspectionRun,
  CodexInspectionRunResponse,
  CodexInspectionSchedulerStatus,
  CodexInspectionTask,
} from '@/types/codexInspectionTask';

export const CODEX_INSPECTION_MOCK_QUERY_PARAM = 'mockCodexInspection';
export const CODEX_INSPECTION_MOCK_SCENARIO_QUERY_PARAM = 'mockCodexInspectionScenario';
export const MOCK_CODEX_INSPECTION_BASE = 'mock://codex-inspection';

export type CodexInspectionMockScenario = 'default' | 'empty' | 'stress' | 'missing-run';

export interface CodexInspectionMockDataset {
  tasks: CodexInspectionTask[];
  runs: CodexInspectionRun[];
  schedulerStatus: CodexInspectionSchedulerStatus;
  runDetails: Record<string, CodexInspectionRunResponse>;
  allowRunFallback: boolean;
}

interface CodexInspectionMockQueryOptions {
  dev?: boolean;
  href?: string;
  hostname?: string;
  mode?: string;
  routerSearch?: string;
  windowSearch?: string;
}

const mockModeAllowed = import.meta.env.DEV || import.meta.env.MODE === 'test';
const now = Date.UTC(2026, 4, 14, 9, 30, 0);

const taskHighRisk: CodexInspectionTask = {
  id: 'cit_mock_high_risk_001',
  name: '高风险账号自动处置',
  description: '每天多次检查高风险标签账号，发现失效或零额度后自动下线。',
  note: '删除动作已开启，请在工作日观察告警。',
  createdBy: 'alice',
  enabled: true,
  targetScope: {
    type: 'metadata_filter',
    query: 'high-risk,production',
    noteIncludes: 'vip',
    priorityMin: 80,
  },
  schedule: {
    type: 'daily_times',
    times: ['08:30', '14:30', '21:00'],
    timezone: 'Asia/Shanghai',
  },
  execution: {
    concurrency: 8,
    timeoutMs: 30_000,
    retries: 1,
  },
  autoAction: {
    dryRun: false,
    zeroQuotaAction: 'disable',
    fullQuotaAction: 'disable',
    invalidAction: 'delete',
    allowDelete: true,
    requireDeletePreview: true,
  },
  notification: {
    enabled: true,
    channels: ['telegram', 'webhook'],
    trigger: 'abnormal',
    channelConfigs: {
      telegram: {
        chatId: '-1001234567890',
      },
      webhook: {
        url: 'https://example.test/codex-inspection',
      },
    },
  },
  logRetention: {
    mode: 'days',
    days: 30,
  },
  saveLogs: true,
  dryRun: false,
  status: 'partial',
  lastRunId: 'cir_mock_partial_001',
  lastRunStatus: 'partial',
  lastRunAtMs: now - 2 * 60 * 60 * 1000,
  nextRunAtMs: now + 4 * 60 * 60 * 1000,
  createdAtMs: now - 20 * 24 * 60 * 60 * 1000,
  updatedAtMs: now - 45 * 60 * 1000,
};

const taskSafe: CodexInspectionTask = {
  id: 'cit_mock_safe_002',
  name: '全量只读巡检',
  description: '每 6 小时执行一次 dry-run，全量观察 Codex 账号状态变化。',
  note: '仅产出建议，不做真实动作。',
  createdBy: 'ops-bot',
  enabled: true,
  targetScope: {
    type: 'all_codex',
  },
  schedule: {
    type: 'interval',
    every: 6,
    unit: 'hour',
    timezone: 'UTC',
  },
  execution: {
    concurrency: 4,
    timeoutMs: 20_000,
    retries: 0,
  },
  autoAction: {
    dryRun: true,
    zeroQuotaAction: 'disable',
    fullQuotaAction: 'disable',
    invalidAction: 'none',
    allowDelete: false,
    requireDeletePreview: true,
  },
  notification: {
    enabled: false,
    channels: [],
    trigger: 'manual_required',
  },
  logRetention: {
    mode: 'latest',
    count: 200,
  },
  saveLogs: true,
  dryRun: true,
  status: 'success',
  lastRunId: 'cir_mock_success_002',
  lastRunStatus: 'success',
  lastRunAtMs: now - 6 * 60 * 60 * 1000,
  nextRunAtMs: now + 20 * 60 * 1000,
  createdAtMs: now - 15 * 24 * 60 * 60 * 1000,
  updatedAtMs: now - 6 * 60 * 60 * 1000,
};

const taskManual: CodexInspectionTask = {
  id: 'cit_mock_manual_003',
  name: '人工兜底复核',
  description: '仅针对财务和高优先级账号的人工复核任务。',
  note: '最近一次执行因上游 API 抖动失败。',
  createdBy: 'security',
  enabled: false,
  targetScope: {
    type: 'auth_indices',
    authIndices: ['finance@example.com', 'ceo@example.com', 'ops-oncall@example.com'],
  },
  schedule: {
    type: 'manual',
  },
  execution: {
    concurrency: 2,
    timeoutMs: 15_000,
    retries: 2,
  },
  autoAction: {
    dryRun: true,
    zeroQuotaAction: 'none',
    fullQuotaAction: 'disable',
    invalidAction: 'none',
    allowDelete: false,
    requireDeletePreview: true,
  },
  notification: {
    enabled: true,
    channels: ['wecom'],
    trigger: 'manual_required',
    channelConfigs: {
      wecom: {
        webhookUrl: 'https://qyapi.weixin.qq.com/mock',
      },
    },
  },
  logRetention: {
    mode: 'days',
    days: 7,
  },
  saveLogs: true,
  dryRun: true,
  status: 'failed',
  lastRunId: 'cir_mock_failed_003',
  lastRunStatus: 'failed',
  lastRunAtMs: now - 26 * 60 * 60 * 1000,
  createdAtMs: now - 12 * 24 * 60 * 60 * 1000,
  updatedAtMs: now - 26 * 60 * 60 * 1000,
};

const taskRunning: CodexInspectionTask = {
  id: 'cit_mock_running_004',
  name: '认证文件滚动巡检',
  description: '以认证文件为维度，每 30 分钟检查一轮运行态账号。',
  note: '适合观察短时额度波动。',
  createdBy: 'scheduler',
  enabled: true,
  targetScope: {
    type: 'files',
    fileNames: ['team-alpha.json', 'team-bravo.json', 'shared-ops.json'],
  },
  schedule: {
    type: 'interval',
    every: 30,
    unit: 'minute',
    timezone: 'Asia/Shanghai',
  },
  execution: {
    concurrency: 3,
    timeoutMs: 12_000,
    retries: 0,
  },
  autoAction: {
    dryRun: true,
    zeroQuotaAction: 'disable',
    fullQuotaAction: 'disable',
    invalidAction: 'disable',
    allowDelete: false,
    requireDeletePreview: true,
  },
  notification: {
    enabled: true,
    channels: ['telegram'],
    trigger: 'auto_action',
    channelConfigs: {
      telegram: {
        chatId: '-10099887766',
      },
    },
  },
  logRetention: {
    mode: 'latest',
    count: 100,
  },
  saveLogs: true,
  dryRun: true,
  status: 'running',
  lastRunId: 'cir_mock_running_004',
  lastRunStatus: 'running',
  lastRunAtMs: now - 20 * 60 * 1000,
  nextRunAtMs: now + 10 * 60 * 1000,
  createdAtMs: now - 6 * 24 * 60 * 60 * 1000,
  updatedAtMs: now - 10 * 60 * 1000,
};

const runPartial: CodexInspectionRun = {
  id: 'cir_mock_partial_001',
  taskId: taskHighRisk.id,
  batchId: 'cib_mock_partial_001',
  trigger: 'scheduled',
  status: 'partial',
  startedAtMs: now - 2 * 60 * 60 * 1000,
  endedAtMs: now - 118 * 60 * 1000,
  durationMs: 125_000,
  scheduleSnapshot: taskHighRisk.schedule,
  targetScopeSnapshot: taskHighRisk.targetScope,
  executionSnapshot: taskHighRisk.execution,
  autoActionSnapshot: taskHighRisk.autoAction,
  notificationSnapshot: taskHighRisk.notification,
  summary: {
    total: 45,
    healthy: 38,
    fullQuota: 4,
    zeroQuota: 0,
    invalid: 2,
    probeFailed: 1,
    disableCount: 2,
    enableCount: 0,
    deleteCount: 0,
  },
  error: '1 个账号在探测阶段超时，已进入人工复核队列。',
  createdAtMs: now - 2 * 60 * 60 * 1000,
};

const runHighRiskPrevious: CodexInspectionRun = {
  id: 'cir_mock_partial_000',
  taskId: taskHighRisk.id,
  batchId: 'cib_mock_partial_000',
  trigger: 'scheduled',
  status: 'success',
  startedAtMs: now - 26 * 60 * 60 * 1000,
  endedAtMs: now - 26 * 60 * 60 * 1000 + 88_000,
  durationMs: 88_000,
  scheduleSnapshot: taskHighRisk.schedule,
  targetScopeSnapshot: taskHighRisk.targetScope,
  executionSnapshot: taskHighRisk.execution,
  autoActionSnapshot: taskHighRisk.autoAction,
  notificationSnapshot: taskHighRisk.notification,
  summary: {
    total: 30,
    healthy: 27,
    fullQuota: 1,
    zeroQuota: 1,
    invalid: 1,
    probeFailed: 0,
    disableCount: 2,
    enableCount: 0,
    deleteCount: 0,
  },
  createdAtMs: now - 26 * 60 * 60 * 1000,
};

const runSuccess: CodexInspectionRun = {
  id: 'cir_mock_success_002',
  taskId: taskSafe.id,
  batchId: 'cib_mock_success_002',
  trigger: 'scheduled',
  status: 'success',
  startedAtMs: now - 6 * 60 * 60 * 1000,
  endedAtMs: now - 6 * 60 * 60 * 1000 + 94_000,
  durationMs: 94_000,
  scheduleSnapshot: taskSafe.schedule,
  targetScopeSnapshot: taskSafe.targetScope,
  executionSnapshot: taskSafe.execution,
  autoActionSnapshot: taskSafe.autoAction,
  notificationSnapshot: taskSafe.notification,
  summary: {
    total: 48,
    healthy: 45,
    fullQuota: 1,
    zeroQuota: 2,
    invalid: 0,
    probeFailed: 0,
    disableCount: 0,
    enableCount: 0,
    deleteCount: 0,
  },
  createdAtMs: now - 6 * 60 * 60 * 1000,
};

const runSuccessPrevious: CodexInspectionRun = {
  id: 'cir_mock_success_001',
  taskId: taskSafe.id,
  batchId: 'cib_mock_success_001',
  trigger: 'scheduled',
  status: 'success',
  startedAtMs: now - 12 * 60 * 60 * 1000,
  endedAtMs: now - 12 * 60 * 60 * 1000 + 91_000,
  durationMs: 91_000,
  scheduleSnapshot: taskSafe.schedule,
  targetScopeSnapshot: taskSafe.targetScope,
  executionSnapshot: taskSafe.execution,
  autoActionSnapshot: taskSafe.autoAction,
  notificationSnapshot: taskSafe.notification,
  summary: {
    total: 47,
    healthy: 44,
    fullQuota: 1,
    zeroQuota: 2,
    invalid: 0,
    probeFailed: 0,
    disableCount: 0,
    enableCount: 0,
    deleteCount: 0,
  },
  createdAtMs: now - 12 * 60 * 60 * 1000,
};

const runFailed: CodexInspectionRun = {
  id: 'cir_mock_failed_003',
  taskId: taskManual.id,
  batchId: 'cib_mock_failed_003',
  trigger: 'manual',
  status: 'failed',
  startedAtMs: now - 26 * 60 * 60 * 1000,
  endedAtMs: now - 26 * 60 * 60 * 1000 + 42_000,
  durationMs: 42_000,
  scheduleSnapshot: taskManual.schedule,
  targetScopeSnapshot: taskManual.targetScope,
  executionSnapshot: taskManual.execution,
  autoActionSnapshot: taskManual.autoAction,
  notificationSnapshot: taskManual.notification,
  summary: {
    total: 6,
    healthy: 2,
    fullQuota: 0,
    zeroQuota: 0,
    invalid: 0,
    probeFailed: 4,
    disableCount: 0,
    enableCount: 0,
    deleteCount: 0,
  },
  error: 'CPA Management API 在 42 秒后返回超时。',
  createdAtMs: now - 26 * 60 * 60 * 1000,
};

const runRunning: CodexInspectionRun = {
  id: 'cir_mock_running_004',
  taskId: taskRunning.id,
  batchId: 'cib_mock_running_004',
  trigger: 'scheduled',
  status: 'running',
  startedAtMs: now - 20 * 60 * 1000,
  scheduleSnapshot: taskRunning.schedule,
  targetScopeSnapshot: taskRunning.targetScope,
  executionSnapshot: taskRunning.execution,
  autoActionSnapshot: taskRunning.autoAction,
  notificationSnapshot: taskRunning.notification,
  summary: {
    total: 12,
    healthy: 5,
    fullQuota: 1,
    zeroQuota: 1,
    invalid: 1,
    probeFailed: 0,
    disableCount: 1,
    enableCount: 0,
    deleteCount: 0,
  },
  createdAtMs: now - 20 * 60 * 1000,
};

const runPartialAccounts: CodexInspectionAccountResult[] = [
  {
    id: 1,
    runId: runPartial.id,
    taskId: runPartial.taskId,
    fileName: 'vip-prod-01.json',
    authIndex: 'ops-primary@example.com',
    accountId: 'ops-primary@example.com',
    displayAccount: 'ops-primary@example.com',
    provider: 'codex',
    disabledBefore: false,
    status: 'success',
    statusCode: 200,
    usedPercent: 38,
    classification: 'healthy',
    recommendedAction: 'none',
    createdAtMs: runPartial.startedAtMs ?? runPartial.createdAtMs,
  },
  {
    id: 2,
    runId: runPartial.id,
    taskId: runPartial.taskId,
    fileName: 'vip-prod-02.json',
    authIndex: 'ops-backup@example.com',
    accountId: 'ops-backup@example.com',
    displayAccount: 'ops-backup@example.com',
    provider: 'codex',
    disabledBefore: false,
    status: 'success',
    statusCode: 200,
    usedPercent: 100,
    classification: 'full_quota',
    recommendedAction: 'disable',
    actionReason: '该账号配额已满，建议自动下线。',
    createdAtMs: runPartial.startedAtMs ?? runPartial.createdAtMs,
  },
  {
    id: 3,
    runId: runPartial.id,
    taskId: runPartial.taskId,
    fileName: 'vip-prod-03.json',
    authIndex: 'billing@example.com',
    accountId: 'billing@example.com',
    displayAccount: 'billing@example.com',
    provider: 'codex',
    disabledBefore: false,
    status: 'success',
    statusCode: 200,
    usedPercent: 12,
    classification: 'healthy',
    recommendedAction: 'none',
    createdAtMs: runPartial.startedAtMs ?? runPartial.createdAtMs,
  },
  {
    id: 4,
    runId: runPartial.id,
    taskId: runPartial.taskId,
    fileName: 'vip-prod-04.json',
    authIndex: 'legacy-invalid@example.com',
    accountId: 'legacy-invalid@example.com',
    displayAccount: 'legacy-invalid@example.com',
    provider: 'codex',
    disabledBefore: true,
    status: 'failed',
    statusCode: 401,
    classification: 'invalid',
    recommendedAction: 'delete',
    actionReason: '认证已失效且被标记为可删除。',
    createdAtMs: runPartial.startedAtMs ?? runPartial.createdAtMs,
  },
  {
    id: 5,
    runId: runPartial.id,
    taskId: runPartial.taskId,
    fileName: 'vip-prod-05.json',
    authIndex: 'timeout@example.com',
    accountId: 'timeout@example.com',
    displayAccount: 'timeout@example.com',
    provider: 'codex',
    disabledBefore: false,
    status: 'failed',
    classification: 'probe_failed',
    recommendedAction: 'none',
    error: '上游接口响应超时',
    createdAtMs: runPartial.startedAtMs ?? runPartial.createdAtMs,
  },
  ...Array.from({ length: 40 }, (_, idx) => {
    const seq = idx + 6;
    const seqStr = String(seq).padStart(2, '0');
    const isFull = idx >= 36 && idx <= 38;
    const isInvalid = idx === 39;
    const classification = isInvalid ? 'invalid' : isFull ? 'full_quota' : 'healthy';
    const usedPercent = isInvalid ? undefined : isFull ? 95 + (idx - 36) * 2 : 18 + (idx % 9) * 8;
    const statusCode = isInvalid ? 401 : 200;
    const recommendedAction = isInvalid ? 'delete' : isFull ? 'disable' : 'none';
    const aliases = [
      'aikkosasaki',
      'ops-team',
      'team-billing',
      'marketing',
      'analytics',
      'sales',
      'support',
      'engineer',
      'product',
      'design',
    ];
    const alias = aliases[idx % aliases.length];
    const account = `${alias}-${seqStr}@example.com`;
    return {
      id: seq,
      runId: runPartial.id,
      taskId: runPartial.taskId,
      fileName: `vip-prod-${seqStr}.json`,
      authIndex: account,
      accountId: account,
      displayAccount: account,
      provider: 'codex',
      disabledBefore: false,
      status: isInvalid ? 'failed' : 'success',
      statusCode,
      usedPercent,
      classification,
      recommendedAction,
      actionReason: isFull
        ? '该账号配额已满，建议自动下线。'
        : isInvalid
          ? '认证失败，建议下线后人工核实。'
          : undefined,
      error: isInvalid ? '认证已失效，HTTP 401' : undefined,
      createdAtMs: runPartial.startedAtMs ?? runPartial.createdAtMs,
    } as CodexInspectionAccountResult;
  }),
];

const runPartialActions: CodexInspectionActionRecord[] = [
  {
    id: 1,
    taskId: runPartial.taskId,
    runId: runPartial.id,
    fileName: 'vip-prod-02.json',
    authIndex: 'ops-backup@example.com',
    action: 'disable',
    triggerReason: 'full_quota',
    dryRun: false,
    success: true,
    createdAtMs: runPartial.endedAtMs ?? runPartial.createdAtMs,
  },
  {
    id: 2,
    taskId: runPartial.taskId,
    runId: runPartial.id,
    fileName: 'vip-prod-42.json',
    authIndex: 'team-billing-42@example.com',
    action: 'disable',
    triggerReason: 'full_quota',
    dryRun: false,
    success: true,
    createdAtMs: runPartial.endedAtMs ?? runPartial.createdAtMs,
  },
];

const runPartialNotifications: CodexInspectionNotificationRecord[] = [
  {
    id: 1,
    taskId: runPartial.taskId,
    runId: runPartial.id,
    channel: 'telegram',
    status: 'success',
    responseSummary: 'message_id=778899',
    createdAtMs: runPartial.endedAtMs ?? runPartial.createdAtMs,
  },
  {
    id: 2,
    taskId: runPartial.taskId,
    runId: runPartial.id,
    channel: 'wecom',
    status: 'success',
    responseSummary: 'trace_id=mock-wecom-2',
    createdAtMs: runPartial.endedAtMs ?? runPartial.createdAtMs,
  },
  {
    id: 3,
    taskId: runPartial.taskId,
    runId: runPartial.id,
    channel: 'webhook',
    status: 'failed',
    error: 'Webhook 返回 502 Bad Gateway',
    createdAtMs: runPartial.endedAtMs ?? runPartial.createdAtMs,
  },
];

const runFailedAccounts: CodexInspectionAccountResult[] = [
  {
    id: 11,
    runId: runFailed.id,
    taskId: runFailed.taskId,
    fileName: 'finance.json',
    authIndex: 'finance@example.com',
    accountId: 'finance@example.com',
    displayAccount: 'finance@example.com',
    provider: 'codex',
    disabledBefore: false,
    status: 'failed',
    classification: 'probe_failed',
    recommendedAction: 'none',
    error: '请求在 15 秒后超时',
    createdAtMs: runFailed.startedAtMs ?? runFailed.createdAtMs,
  },
  {
    id: 12,
    runId: runFailed.id,
    taskId: runFailed.taskId,
    fileName: 'ceo.json',
    authIndex: 'ceo@example.com',
    accountId: 'ceo@example.com',
    displayAccount: 'ceo@example.com',
    provider: 'codex',
    disabledBefore: false,
    status: 'success',
    statusCode: 200,
    usedPercent: 42,
    classification: 'healthy',
    recommendedAction: 'none',
    createdAtMs: runFailed.startedAtMs ?? runFailed.createdAtMs,
  },
];

const runFailedNotifications: CodexInspectionNotificationRecord[] = [
  {
    id: 21,
    taskId: runFailed.taskId,
    runId: runFailed.id,
    channel: 'wecom',
    status: 'success',
    responseSummary: 'trace_id=mock-wecom-1',
    createdAtMs: runFailed.endedAtMs ?? runFailed.createdAtMs,
  },
];

const runRunningAccounts: CodexInspectionAccountResult[] = [
  {
    id: 31,
    runId: runRunning.id,
    taskId: runRunning.taskId,
    fileName: 'team-alpha.json',
    authIndex: 'alpha-1@example.com',
    accountId: 'alpha-1@example.com',
    displayAccount: 'alpha-1@example.com',
    provider: 'codex',
    disabledBefore: false,
    status: 'success',
    statusCode: 200,
    usedPercent: 67,
    classification: 'healthy',
    recommendedAction: 'none',
    createdAtMs: runRunning.startedAtMs ?? runRunning.createdAtMs,
  },
];

const defaultMockDataset: CodexInspectionMockDataset = {
  tasks: [taskHighRisk, taskSafe, taskManual, taskRunning],
  runs: [runRunning, runPartial, runSuccess, runSuccessPrevious, runHighRiskPrevious, runFailed],
  schedulerStatus: {
    status: 'running',
    running: true,
    workerCount: 2,
    lastHeartbeatAtMs: now - 15_000,
    nextTickAtMs: now + 60_000,
  },
  runDetails: {
    [runPartial.id]: {
      run: runPartial,
      accounts: runPartialAccounts,
      actions: runPartialActions,
      notifications: runPartialNotifications,
    },
    [runFailed.id]: {
      run: runFailed,
      accounts: runFailedAccounts,
      actions: [],
      notifications: runFailedNotifications,
    },
    [runRunning.id]: {
      run: runRunning,
      accounts: runRunningAccounts,
      actions: [],
      notifications: [],
    },
  },
  allowRunFallback: true,
};

const stressPrimaryTask: CodexInspectionTask = {
  ...taskHighRisk,
  id: 'cit_mock_stress_001',
  name: '跨区域高风险账号巡检任务 - 用于验证超长标题、卡片换行、筛选布局以及多通知渠道展示是否稳定',
  description:
    '这是一条专门用于 UI 压测的巡检任务描述，覆盖超长标题、超长描述、密集通知渠道、分页执行记录以及复杂筛选标签的展示效果。',
  note: '备注：本任务用于前端压测，请重点观察任务详情中的长文本折行、风险提示、渠道卡片和分页栏是否在窄屏下依然可读。',
  createdBy: 'platform-security-automation@example.com',
  targetScope: {
    type: 'metadata_filter',
    query: 'critical,billing,production,weekend-oncall,cn-hz,cn-bj,night-shift,long-text-layout',
    noteIncludes: 'needs-human-review escalation-required handoff-checklist',
    priorityMin: 60,
    priorityMax: 100,
  },
  schedule: {
    type: 'daily_times',
    times: ['00:15', '06:45', '12:15', '18:45'],
    timezone: 'Asia/Shanghai',
  },
  execution: {
    concurrency: 12,
    timeoutMs: 45_000,
    retries: 2,
  },
  autoAction: {
    dryRun: false,
    zeroQuotaAction: 'disable',
    fullQuotaAction: 'disable',
    invalidAction: 'delete',
    allowDelete: true,
    requireDeletePreview: true,
  },
  notification: {
    enabled: true,
    channels: ['telegram', 'feishu', 'wecom', 'webhook'],
    trigger: 'always',
    channelConfigs: {
      telegram: { chatId: '-100000000001' },
      feishu: { webhookUrl: 'https://open.feishu.cn/mock/1' },
      wecom: { webhookUrl: 'https://qyapi.weixin.qq.com/mock/stress' },
      webhook: { url: 'https://example.test/stress-webhook' },
    },
  },
  logRetention: {
    mode: 'latest',
    count: 500,
  },
  saveLogs: true,
  dryRun: false,
  status: 'partial',
  lastRunId: 'cir_mock_stress_001_01',
  lastRunStatus: 'partial',
  lastRunAtMs: now - 30 * 60 * 1000,
  nextRunAtMs: now + 15 * 60 * 1000,
  createdAtMs: now - 45 * 24 * 60 * 60 * 1000,
  updatedAtMs: now - 3 * 60 * 1000,
};

const stressStatuses = ['partial', 'success', 'failed', 'interrupted'] as const;
const stressPrimaryRuns: CodexInspectionRun[] = Array.from({ length: 14 }, (_, index) => {
  const status = stressStatuses[index % stressStatuses.length];
  const startedAtMs = now - (index + 1) * 35 * 60 * 1000;
  const endedAtMs = status === 'interrupted' ? startedAtMs + 32_000 : startedAtMs + 96_000 + index * 1_000;
  return {
    id: `cir_mock_stress_001_${String(index + 1).padStart(2, '0')}`,
    taskId: stressPrimaryTask.id,
    batchId: `cib_mock_stress_001_${String(index + 1).padStart(2, '0')}`,
    trigger: index % 3 === 0 ? 'manual' : 'scheduled',
    status,
    startedAtMs,
    endedAtMs,
    durationMs: endedAtMs - startedAtMs,
    scheduleSnapshot: stressPrimaryTask.schedule,
    targetScopeSnapshot: stressPrimaryTask.targetScope,
    executionSnapshot: stressPrimaryTask.execution,
    autoActionSnapshot: stressPrimaryTask.autoAction,
    notificationSnapshot: stressPrimaryTask.notification,
    summary: {
      total: 80 - index,
      healthy: 48 - (index % 5),
      fullQuota: 7 + (index % 3),
      zeroQuota: 8 + (index % 4),
      invalid: 4 + (index % 2),
      probeFailed: 5 + (index % 3),
      disableCount: 6 + (index % 4),
      enableCount: index % 2,
      deleteCount: index % 3 === 0 ? 2 : 1,
    },
    error: status === 'failed' ? '压测场景：部分上游调用返回 503。' : status === 'interrupted' ? '压测场景：任务被人工中断。' : '',
    createdAtMs: startedAtMs,
  };
});

const stressRunDetail: CodexInspectionRunResponse = {
  run: stressPrimaryRuns[0],
  accounts: [
    {
      id: 101,
      runId: stressPrimaryRuns[0].id,
      taskId: stressPrimaryTask.id,
      fileName: 'critical-billing-handover-with-very-long-file-name-2026-05-14-production.json',
      authIndex: 'vip-asia-hz-primary@example.com',
      accountId: 'vip-asia-hz-primary@example.com',
      displayAccount: 'vip-asia-hz-primary@example.com',
      provider: 'codex',
      disabledBefore: false,
      status: 'success',
      statusCode: 200,
      usedPercent: 97,
      classification: 'full_quota',
      recommendedAction: 'disable',
      actionReason: '压测场景：账号即将打满额度。',
      createdAtMs: stressPrimaryRuns[0].startedAtMs ?? stressPrimaryRuns[0].createdAtMs,
    },
    {
      id: 102,
      runId: stressPrimaryRuns[0].id,
      taskId: stressPrimaryTask.id,
      fileName: 'critical-billing-handover-with-very-long-file-name-2026-05-14-secondary.json',
      authIndex: 'vip-asia-bj-dr@example.com',
      accountId: 'vip-asia-bj-dr@example.com',
      displayAccount: 'vip-asia-bj-dr@example.com',
      provider: 'codex',
      disabledBefore: false,
      status: 'failed',
      statusCode: 401,
      classification: 'invalid',
      recommendedAction: 'delete',
      actionReason: '压测场景：账号已失效且允许删除。',
      createdAtMs: stressPrimaryRuns[0].startedAtMs ?? stressPrimaryRuns[0].createdAtMs,
    },
  ],
  actions: [
    {
      id: 201,
      taskId: stressPrimaryTask.id,
      runId: stressPrimaryRuns[0].id,
      fileName: 'critical-billing-handover-with-very-long-file-name-2026-05-14-production.json',
      authIndex: 'vip-asia-hz-primary@example.com',
      action: 'disable',
      triggerReason: 'full_quota',
      dryRun: false,
      success: true,
      createdAtMs: stressPrimaryRuns[0].endedAtMs ?? stressPrimaryRuns[0].createdAtMs,
    },
    {
      id: 202,
      taskId: stressPrimaryTask.id,
      runId: stressPrimaryRuns[0].id,
      fileName: 'critical-billing-handover-with-very-long-file-name-2026-05-14-secondary.json',
      authIndex: 'vip-asia-bj-dr@example.com',
      action: 'delete',
      triggerReason: 'invalid',
      dryRun: false,
      success: false,
      error: '压测场景：删除前校验未通过。',
      createdAtMs: stressPrimaryRuns[0].endedAtMs ?? stressPrimaryRuns[0].createdAtMs,
    },
  ],
  notifications: [
    {
      id: 301,
      taskId: stressPrimaryTask.id,
      runId: stressPrimaryRuns[0].id,
      channel: 'telegram',
      status: 'success',
      responseSummary: 'stress-message-id=1',
      createdAtMs: stressPrimaryRuns[0].endedAtMs ?? stressPrimaryRuns[0].createdAtMs,
    },
    {
      id: 302,
      taskId: stressPrimaryTask.id,
      runId: stressPrimaryRuns[0].id,
      channel: 'webhook',
      status: 'failed',
      error: '压测场景：Webhook 连接超时。',
      createdAtMs: stressPrimaryRuns[0].endedAtMs ?? stressPrimaryRuns[0].createdAtMs,
    },
  ],
};

const stressAdditionalTasks: CodexInspectionTask[] = Array.from({ length: 11 }, (_, index) => {
  const taskNumber = index + 2;
  const baseTask = [taskSafe, taskManual, taskRunning, taskHighRisk][index % 4];
  const status = ['success', 'failed', 'running', 'partial'][index % 4] as CodexInspectionTask['status'];
  const lastRunId = `cir_mock_stress_task_${String(taskNumber).padStart(2, '0')}`;
  return {
    ...baseTask,
    id: `cit_mock_stress_${String(taskNumber).padStart(3, '0')}`,
    name: `压测任务 ${taskNumber}`,
    description: `用于分页和筛选压测的附加任务 ${taskNumber}`,
    note: taskNumber % 2 === 0 ? '压测附加任务，无特殊说明。' : '压测附加任务，带少量异常数据。',
    createdBy: `stress-bot-${taskNumber}`,
    enabled: taskNumber % 5 !== 0,
    status,
    lastRunId,
    lastRunStatus: status,
    lastRunAtMs: now - taskNumber * 45 * 60 * 1000,
    nextRunAtMs: status === 'running' ? now + taskNumber * 5 * 60 * 1000 : now + taskNumber * 20 * 60 * 1000,
    createdAtMs: now - taskNumber * 24 * 60 * 60 * 1000,
    updatedAtMs: now - taskNumber * 12 * 60 * 1000,
  };
});

const stressAdditionalRuns: CodexInspectionRun[] = stressAdditionalTasks.map((task, index) => {
  const status = task.lastRunStatus || task.status;
  const startedAtMs = task.lastRunAtMs || now - (index + 2) * 50 * 60 * 1000;
  const endedAtMs = status === 'running' ? undefined : startedAtMs + 55_000 + index * 1_500;
  return {
    id: task.lastRunId || `cir_mock_stress_task_${String(index + 2).padStart(2, '0')}`,
    taskId: task.id,
    batchId: `cib_mock_stress_task_${String(index + 2).padStart(2, '0')}`,
    trigger: index % 2 === 0 ? 'scheduled' : 'manual',
    status,
    startedAtMs,
    endedAtMs,
    durationMs: endedAtMs ? endedAtMs - startedAtMs : undefined,
    scheduleSnapshot: task.schedule,
    targetScopeSnapshot: task.targetScope,
    executionSnapshot: task.execution,
    autoActionSnapshot: task.autoAction,
    notificationSnapshot: task.notification,
    summary: {
      total: 10 + index,
      healthy: Math.max(2, 7 - (index % 3)),
      fullQuota: index % 3,
      zeroQuota: (index + 1) % 3,
      invalid: index % 2,
      probeFailed: status === 'failed' ? 3 : index % 2,
      disableCount: index % 4,
      enableCount: index % 3 === 0 ? 1 : 0,
      deleteCount: index % 5 === 0 ? 1 : 0,
    },
    error: status === 'failed' ? '压测附加任务失败。' : '',
    createdAtMs: startedAtMs,
  };
});

const stressMockDataset: CodexInspectionMockDataset = {
  tasks: [stressPrimaryTask, ...stressAdditionalTasks],
  runs: [...stressPrimaryRuns, ...stressAdditionalRuns],
  schedulerStatus: {
    status: 'running',
    running: true,
    workerCount: 5,
    lastHeartbeatAtMs: now - 4_000,
    nextTickAtMs: now + 20_000,
  },
  runDetails: {
    [stressPrimaryRuns[0].id]: stressRunDetail,
  },
  allowRunFallback: true,
};

const emptyMockDataset: CodexInspectionMockDataset = {
  tasks: [],
  runs: [],
  schedulerStatus: {
    status: 'idle',
    running: false,
    workerCount: 0,
  },
  runDetails: {},
  allowRunFallback: false,
};

const missingRunMockDataset: CodexInspectionMockDataset = {
  tasks: defaultMockDataset.tasks,
  runs: defaultMockDataset.runs,
  schedulerStatus: defaultMockDataset.schedulerStatus,
  runDetails: {},
  allowRunFallback: false,
};

export const mockCodexInspectionTasks = defaultMockDataset.tasks;
export const mockCodexInspectionRuns = defaultMockDataset.runs;
export const mockCodexInspectionSchedulerStatus = defaultMockDataset.schedulerStatus;

const normalizeSearch = (search: string) => (search.startsWith('?') ? search : search ? `?${search}` : '');

const getHashSearch = (href: string) => {
  if (!href) return '';
  try {
    const hash = new URL(href).hash.slice(1);
    if (!hash) return '';
    const queryIndex = hash.indexOf('?');
    if (queryIndex < 0) return '';
    const query = hash.slice(queryIndex);
    const nestedHashIndex = query.indexOf('#', 1);
    return nestedHashIndex >= 0 ? query.slice(0, nestedHashIndex) : query;
  } catch {
    return '';
  }
};

const mergeSearchParams = (target: URLSearchParams, search: string) => {
  const normalized = normalizeSearch(search);
  if (!normalized) return;
  const params = new URLSearchParams(normalized);
  params.forEach((value, key) => {
    target.set(key, value);
  });
};

const isLocalMockHost = (hostname: string) =>
  hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '::1';

export const shouldAllowCodexInspectionMock = (options: CodexInspectionMockQueryOptions = {}) => {
  const dev = options.dev ?? import.meta.env.DEV;
  const mode = options.mode ?? import.meta.env.MODE;
  const hostname =
    options.hostname ?? (typeof window !== 'undefined' ? window.location.hostname : '');
  return dev || mode === 'test' || isLocalMockHost(hostname);
};

const resolveCodexInspectionMockParams = (options: CodexInspectionMockQueryOptions = {}) => {
  const params = new URLSearchParams();
  const windowSearch =
    options.windowSearch ?? (typeof window !== 'undefined' ? window.location.search : '');
  const href = options.href ?? (typeof window !== 'undefined' ? window.location.href : '');
  mergeSearchParams(params, windowSearch);
  mergeSearchParams(params, getHashSearch(href));
  mergeSearchParams(params, options.routerSearch ?? '');
  return params;
};

export const getCodexInspectionMockSearch = (
  routerSearch: string,
  options: Omit<CodexInspectionMockQueryOptions, 'routerSearch'> = {}
) => {
  const params = resolveCodexInspectionMockParams({
    ...options,
    routerSearch,
  });
  const value = params.toString();
  return value ? `?${value}` : '';
};

export const isCodexInspectionMockEnabled = (
  search: string,
  options: Omit<CodexInspectionMockQueryOptions, 'routerSearch'> = {}
) => {
  if (!mockModeAllowed && !shouldAllowCodexInspectionMock(options)) return false;
  const value = resolveCodexInspectionMockParams({
    ...options,
    routerSearch: search,
  }).get(CODEX_INSPECTION_MOCK_QUERY_PARAM);
  return value === '1' || value === 'true' || value === 'yes';
};

export const getCodexInspectionMockScenario = (
  search: string,
  options: Omit<CodexInspectionMockQueryOptions, 'routerSearch'> = {}
): CodexInspectionMockScenario => {
  const value = resolveCodexInspectionMockParams({
    ...options,
    routerSearch: search,
  }).get(CODEX_INSPECTION_MOCK_SCENARIO_QUERY_PARAM);
  if (value === 'empty' || value === 'stress' || value === 'missing-run') return value;
  return 'default';
};

export const getCodexInspectionMockDataset = (scenario: CodexInspectionMockScenario): CodexInspectionMockDataset => {
  switch (scenario) {
    case 'empty':
      return emptyMockDataset;
    case 'stress':
      return stressMockDataset;
    case 'missing-run':
      return missingRunMockDataset;
    case 'default':
    default:
      return defaultMockDataset;
  }
};

export const findMockCodexInspectionRunDetail = (
  runId: string,
  scenario: CodexInspectionMockScenario = 'default'
): CodexInspectionRunResponse | null => {
  const dataset = getCodexInspectionMockDataset(scenario);
  if (dataset.runDetails[runId]) return dataset.runDetails[runId];
  if (!dataset.allowRunFallback) return null;
  const run = dataset.runs.find((item) => item.id === runId);
  return run ? { run, accounts: [], actions: [], notifications: [] } : null;
};
