import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CodexInspectionTasksPage } from './CodexInspectionTasksPage';

const { mocks } = vi.hoisted(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT =
    true;
  return {
    mocks: {
      showNotification: vi.fn(),
      showConfirmation: vi.fn(),
    },
  };
});

vi.mock('@/stores', () => ({
  useAuthStore: (selector: (state: { apiBase: string; managementKey: string }) => unknown) =>
    selector({ apiBase: 'http://localhost:3000', managementKey: 'mock-key' }),
  useUsageServiceStore: (selector: (state: { enabled: boolean; serviceBase: string }) => unknown) =>
    selector({ enabled: false, serviceBase: '' }),
  useNotificationStore: (
    selector: (state: {
      showNotification: typeof mocks.showNotification;
      showConfirmation: typeof mocks.showConfirmation;
    }) => unknown
  ) =>
    selector({
      showNotification: mocks.showNotification,
      showConfirmation: mocks.showConfirmation,
    }),
}));

describe('CodexInspectionTasksPage mock mode', () => {
  beforeEach(() => {
    mocks.showNotification.mockReset();
    mocks.showConfirmation.mockReset();
  });

  it('renders local mock task data when query flag is enabled', async () => {
    const html = renderToStaticMarkup(
      <MemoryRouter initialEntries={['/monitoring/codex-inspection-tasks?mockCodexInspection=1&mockCodexInspectionScenario=default']}>
        <Routes>
          <Route path="/monitoring/codex-inspection-tasks" element={<CodexInspectionTasksPage />} />
        </Routes>
      </MemoryRouter>
    );

    expect(html).toContain('Mock 数据模式已启用');
    expect(html).toContain('高风险账号自动处置');
    expect(html).toContain('策略风险提醒');
    expect(html).toContain('高风险');
    expect(html).toContain('最近执行记录');
    expect(html).toContain('当前自动处理策略为 高风险');
    expect(mocks.showNotification).not.toHaveBeenCalledWith(expect.stringContaining('Usage Service'), 'error');
  });

  it('renders empty state when empty scenario is enabled', async () => {
    const html = renderToStaticMarkup(
      <MemoryRouter initialEntries={['/monitoring/codex-inspection-tasks?mockCodexInspection=1&mockCodexInspectionScenario=empty']}>
        <Routes>
          <Route path="/monitoring/codex-inspection-tasks" element={<CodexInspectionTasksPage />} />
        </Routes>
      </MemoryRouter>
    );

    expect(html).toContain('Mock 数据模式已启用');
    expect(html).toContain('还没有巡检任务。');
    expect(html).toContain('选择一个任务查看详情。');
  });

  it('renders stress scenario with pagination and long text', async () => {
    const html = renderToStaticMarkup(
      <MemoryRouter initialEntries={['/monitoring/codex-inspection-tasks?mockCodexInspection=1&mockCodexInspectionScenario=stress']}>
        <Routes>
          <Route path="/monitoring/codex-inspection-tasks" element={<CodexInspectionTasksPage />} />
        </Routes>
      </MemoryRouter>
    );

    expect(html).toContain('跨区域高风险账号巡检任务');
    expect(html).toContain('platform-security-automation@example.com');
    expect(html).toContain('共 12 条');
    expect(html).toContain('共 14 条');
    expect(html).toContain('Telegram / 飞书 / 企业微信 / Webhook');
  });
});
