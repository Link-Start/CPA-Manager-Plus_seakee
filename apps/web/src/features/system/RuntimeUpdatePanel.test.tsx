import type { ReactNode } from 'react';
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import type { RuntimeStatusResult, RuntimeUpdateCheckResult } from '@/services/api/usageService';
import {
  clearRuntimeUpdateCheckCache,
  readRuntimeUpdateCheckCache,
  runtimeUpdateCheckScope,
  storeRuntimeUpdateCheckCache,
} from '@/services/runtimeUpdateCheckCache';
import { RuntimeUpdatePanel } from './RuntimeUpdatePanel';

const mocks = vi.hoisted(() => ({
  authState: {
    apiBase: 'http://manager.local',
    managementKey: 'cpamp-admin-key',
    sessionMode: 'manager_embedded',
  },
  getRuntimeStatus: vi.fn(),
  checkRuntimeUpdates: vi.fn(),
  startRuntimeUpdate: vi.fn(),
  getRuntimeOperation: vi.fn(),
  getRuntimeOperationDirect: vi.fn(),
  updateRuntimePanelBasePath: vi.fn(),
  showNotification: vi.fn(),
  showConfirmation: vi.fn(),
  locationAssign: vi.fn(),
  t: (key: string, options?: Record<string, unknown>) =>
    options?.version ? `${key}:${String(options.version)}` : key,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: mocks.t,
  }),
}));

vi.mock('@/components/ui/Modal', () => ({
  Modal: ({
    open,
    title,
    footer,
    children,
  }: {
    open: boolean;
    title?: ReactNode;
    footer?: ReactNode;
    children?: ReactNode;
  }) =>
    open ? (
      <section data-modal="true">
        {title}
        {children}
        {footer}
      </section>
    ) : null,
}));

vi.mock('@/stores', () => ({
  useAuthStore: (selector: (state: typeof mocks.authState) => unknown) => selector(mocks.authState),
  useNotificationStore: () => ({
    showNotification: mocks.showNotification,
    showConfirmation: mocks.showConfirmation,
  }),
}));

vi.mock('@/services/api/usageService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/api/usageService')>();
  return {
    ...actual,
    usageServiceApi: {
      ...actual.usageServiceApi,
      getRuntimeStatus: mocks.getRuntimeStatus,
      checkRuntimeUpdates: mocks.checkRuntimeUpdates,
      startRuntimeUpdate: mocks.startRuntimeUpdate,
      getRuntimeOperation: mocks.getRuntimeOperation,
      getRuntimeOperationDirect: mocks.getRuntimeOperationDirect,
      updateRuntimePanelBasePath: mocks.updateRuntimePanelBasePath,
    },
  };
});

const runtimeStatus = (
  overrides: Partial<RuntimeStatusResult['deployment']> = {},
  updateManaged = true
): RuntimeStatusResult => ({
  deployment: {
    schemaVersion: 1,
    mode: updateManaged ? 'integrated' : 'external',
    panelBasePath: '/management.html',
    panelBasePathSource: 'runtime',
    runtimeManaged: updateManaged,
    cpampUpdatesManaged: updateManaged,
    cpaUpdatesManaged: updateManaged,
    ...overrides,
  },
  runtimeAvailable: updateManaged,
  runtime: updateManaged
    ? {
        schemaVersion: 2,
        deployment: {
          schemaVersion: 1,
          mode: 'integrated',
          panelBasePath: '/management.html',
          runtimeManaged: true,
          cpampUpdatesManaged: true,
          cpaUpdatesManaged: true,
        },
        components: {
          cpamp: { name: 'cpamp', status: 'running', version: 'v1.11.9' },
          cpa: { name: 'cpa', status: 'running', version: 'v7.1.18' },
        },
        updatedAtMs: 1,
      }
    : undefined,
  updateCapabilities: {
    cpamp: updateManaged,
    cpa: updateManaged,
    all: updateManaged,
  },
});

const updateCheck: RuntimeUpdateCheckResult = {
  checkedAtMs: 1,
  deployment: runtimeStatus().deployment,
  components: {
    cpamp: {
      name: 'cpamp',
      currentVersion: 'v1.11.9',
      availableVersion: 'v1.12.0',
      updateAvailable: true,
      releaseUrl: 'https://example.com/cpamp',
      releaseNotes: 'CPAMP release notes',
    },
    cpa: {
      name: 'cpa',
      currentVersion: 'v7.1.18',
      availableVersion: 'v7.2.0',
      updateAvailable: true,
      releaseUrl: 'https://example.com/cpa',
      releaseNotes: 'CPA release notes',
      minCpampVersion: 'v1.12.0',
      standaloneUpdateAllowed: false,
      combinedUpdateAllowed: true,
    },
  },
};

const textContent = (node: ReactTestInstance): string =>
  node.children.map((child) => (typeof child === 'string' ? child : textContent(child))).join('');

const findButton = (root: ReactTestInstance, label: string) => {
  const buttons = root.findAllByType('button');
  return (
    buttons.find((button) => textContent(button) === label) ||
    buttons.find((button) => textContent(button).includes(label))
  );
};

const flush = async () => {
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
};

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return { promise, resolve };
};

describe('RuntimeUpdatePanel', () => {
  beforeAll(() => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  });

  beforeEach(() => {
    vi.clearAllMocks();
    clearRuntimeUpdateCheckCache();
    mocks.authState.apiBase = 'http://manager.local';
    mocks.authState.managementKey = 'cpamp-admin-key';
    mocks.authState.sessionMode = 'manager_embedded';
    mocks.getRuntimeStatus.mockResolvedValue(runtimeStatus());
    mocks.checkRuntimeUpdates.mockResolvedValue(updateCheck);
    mocks.startRuntimeUpdate.mockResolvedValue({
      operationToken: 'op-token',
      operation: {
        id: 'update-1',
        kind: 'all',
        status: 'succeeded',
        progress: 100,
        components: ['cpamp', 'cpa'],
        startedAtMs: 1,
        updatedAtMs: 2,
        completedAtMs: 2,
      },
    });
    mocks.showConfirmation.mockImplementation((options: { onConfirm: () => void }) => {
      options.onConfirm();
    });
    vi.stubGlobal('window', {
      location: { search: '?locale=zh-CN', hash: '#/system', assign: mocks.locationAssign },
    });
  });

  it('keeps managed update controls out of external panel sessions', async () => {
    mocks.authState.sessionMode = 'external_panel';
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    expect(renderer.toJSON()).toBeNull();
    expect(mocks.getRuntimeStatus).not.toHaveBeenCalled();
    renderer.unmount();
  });

  it('reuses the Dashboard update check and keeps the System check button as a force refresh', async () => {
    const scope = runtimeUpdateCheckScope(mocks.authState.apiBase, mocks.authState.managementKey);
    storeRuntimeUpdateCheckCache(scope, updateCheck);
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    expect(mocks.checkRuntimeUpdates).not.toHaveBeenCalled();
    expect(findButton(renderer.root, 'system_info.runtime_update_all')?.props.disabled).toBe(false);

    await act(async () => {
      findButton(renderer.root, 'system_info.runtime_check_updates')?.props.onClick();
      await flush();
    });
    expect(mocks.checkRuntimeUpdates).toHaveBeenCalledTimes(1);
    expect(readRuntimeUpdateCheckCache(scope)).toEqual(updateCheck);
    renderer.unmount();
  });

  it('clears stale signed results when a forced update check fails', async () => {
    const scope = runtimeUpdateCheckScope(mocks.authState.apiBase, mocks.authState.managementKey);
    storeRuntimeUpdateCheckCache(scope, updateCheck);
    mocks.checkRuntimeUpdates.mockRejectedValueOnce(
      Object.assign(new Error('manifest unavailable'), { code: 'runtime_update_check_failed' })
    );

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });
    expect(findButton(renderer.root, 'system_info.runtime_update_all')?.props.disabled).toBe(false);

    await act(async () => {
      findButton(renderer.root, 'system_info.runtime_check_updates')?.props.onClick();
      await flush();
    });

    expect(readRuntimeUpdateCheckCache(scope)).toBeNull();
    expect(findButton(renderer.root, 'system_info.runtime_update_all')?.props.disabled).toBe(true);
    renderer.unmount();
  });

  it('fails closed and clears cached updates when runtime status refresh fails', async () => {
    const scope = runtimeUpdateCheckScope(mocks.authState.apiBase, mocks.authState.managementKey);
    storeRuntimeUpdateCheckCache(scope, updateCheck);
    mocks.getRuntimeStatus
      .mockResolvedValueOnce(runtimeStatus())
      .mockRejectedValueOnce(new Error('runtime status unavailable'));

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });
    expect(findButton(renderer.root, 'system_info.runtime_update_all')?.props.disabled).toBe(false);

    await act(async () => {
      findButton(renderer.root, 'common.refresh')?.props.onClick();
      await flush();
    });

    expect(readRuntimeUpdateCheckCache(scope)).toBeNull();
    expect(findButton(renderer.root, 'system_info.runtime_update_all')).toBeUndefined();
    expect(findButton(renderer.root, 'common.save')).toBeUndefined();
    expect(textContent(renderer.root)).toContain('runtime status unavailable');
    renderer.unmount();
  });

  it('ignores an update check that completes after runtime status becomes unavailable', async () => {
    const pendingCheck = deferred<RuntimeUpdateCheckResult>();
    mocks.checkRuntimeUpdates.mockReturnValue(pendingCheck.promise);
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    await act(async () => {
      findButton(renderer.root, 'system_info.runtime_check_updates')?.props.onClick();
      await Promise.resolve();
    });

    const unavailable = runtimeStatus();
    unavailable.runtimeAvailable = false;
    unavailable.runtimeUnavailableReason = 'runtime_control_unreachable';
    unavailable.runtime = undefined;
    mocks.getRuntimeStatus.mockResolvedValueOnce(unavailable);
    await act(async () => {
      findButton(renderer.root, 'common.refresh')?.props.onClick();
      await flush();
    });

    await act(async () => {
      pendingCheck.resolve(updateCheck);
      await flush();
    });

    expect(textContent(renderer.root)).not.toContain('system_info.runtime_update_available');
    expect(mocks.showNotification).not.toHaveBeenCalledWith(
      'system_info.runtime_updates_found',
      'warning'
    );
    expect(
      readRuntimeUpdateCheckCache(
        runtimeUpdateCheckScope(mocks.authState.apiBase, mocks.authState.managementKey)
      )
    ).toBeNull();
    renderer.unmount();
  });

  it('checks both components and starts the confirmed managed update from System', async () => {
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    const checkButton = findButton(renderer.root, 'system_info.runtime_check_updates');
    expect(checkButton).toBeDefined();
    await act(async () => {
      checkButton?.props.onClick();
      await flush();
    });
    expect(mocks.checkRuntimeUpdates).toHaveBeenCalledWith(
      'http://manager.local',
      'cpamp-admin-key'
    );

    const updateAllButton = findButton(renderer.root, 'system_info.runtime_update_all');
    expect(updateAllButton?.props.disabled).toBe(false);
    await act(async () => {
      updateAllButton?.props.onClick();
      await flush();
    });
    expect(textContent(renderer.root)).toContain('CPAMP release notes');
    expect(textContent(renderer.root)).toContain('CPA release notes');
    expect(textContent(renderer.root)).toContain('system_info.runtime_update_rollback');

    const confirmButton = findButton(renderer.root, 'system_info.runtime_update_confirm_action');
    await act(async () => {
      confirmButton?.props.onClick();
      await flush();
    });
    expect(mocks.startRuntimeUpdate).toHaveBeenCalledWith(
      'http://manager.local',
      'cpamp-admin-key',
      'all'
    );
    expect(
      readRuntimeUpdateCheckCache(
        runtimeUpdateCheckScope('http://manager.local', 'cpamp-admin-key')
      )
    ).toBeNull();
    expect(textContent(renderer.root)).toContain('system_info.runtime_operation_succeeded');
    expect(mocks.showNotification).toHaveBeenCalledWith(
      'system_info.runtime_update_succeeded',
      'success'
    );
    expect(mocks.showNotification).not.toHaveBeenCalledWith(
      'system_info.runtime_update_started',
      'info'
    );
    renderer.unmount();
  });

  it('blocks incompatible CPA-only updates and guides users to a compatible target', async () => {
    const scope = runtimeUpdateCheckScope(mocks.authState.apiBase, mocks.authState.managementKey);
    storeRuntimeUpdateCheckCache(scope, updateCheck);
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });
    expect(textContent(renderer.root)).toContain('system_info.runtime_cpa_requires_cpamp_update');
    expect(findButton(renderer.root, 'system_info.runtime_update_cpa')?.props.disabled).toBe(true);
    expect(findButton(renderer.root, 'system_info.runtime_update_all')?.props.disabled).toBe(false);
    renderer.unmount();

    const incompatibleUpdateCheck = {
      ...updateCheck,
      components: {
        ...updateCheck.components,
        cpa: {
          ...updateCheck.components.cpa,
          combinedUpdateAllowed: false,
        },
      },
    };
    clearRuntimeUpdateCheckCache();
    storeRuntimeUpdateCheckCache(scope, incompatibleUpdateCheck);
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });
    expect(findButton(renderer.root, 'system_info.runtime_update_cpa')?.props.disabled).toBe(true);
    expect(findButton(renderer.root, 'system_info.runtime_update_all')?.props.disabled).toBe(true);
    expect(textContent(renderer.root)).toContain('system_info.runtime_cpa_update_incompatible');
    renderer.unmount();
  });

  it('prevents rapid duplicate update starts and ignores completion after unmount', async () => {
    const pendingStart = deferred<{
      operationToken: string;
      operation: {
        id: string;
        kind: string;
        status: 'succeeded';
        progress: number;
        components: string[];
        startedAtMs: number;
        updatedAtMs: number;
        completedAtMs: number;
      };
    }>();
    mocks.startRuntimeUpdate.mockImplementationOnce(() => pendingStart.promise);
    storeRuntimeUpdateCheckCache(
      runtimeUpdateCheckScope(mocks.authState.apiBase, mocks.authState.managementKey),
      updateCheck
    );

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });
    await act(async () => {
      findButton(renderer.root, 'system_info.runtime_update_all')?.props.onClick();
      await flush();
    });

    const confirmButton = findButton(renderer.root, 'system_info.runtime_update_confirm_action');
    await act(async () => {
      confirmButton?.props.onClick();
      confirmButton?.props.onClick();
      await Promise.resolve();
    });
    expect(mocks.startRuntimeUpdate).toHaveBeenCalledTimes(1);

    await act(async () => {
      renderer.unmount();
    });
    pendingStart.resolve({
      operationToken: 'op-token',
      operation: {
        id: 'update-after-unmount',
        kind: 'all',
        status: 'succeeded',
        progress: 100,
        components: ['cpamp', 'cpa'],
        startedAtMs: 1,
        updatedAtMs: 2,
        completedAtMs: 2,
      },
    });
    await act(async () => {
      await flush();
    });

    expect(mocks.showNotification).not.toHaveBeenCalled();
  });

  it('does not let an older poll overwrite a newer terminal operation from status refresh', async () => {
    const runningOperation = {
      id: 'update-racing',
      kind: 'all',
      status: 'running',
      progress: 40,
      message: 'updating cpa',
      components: ['cpamp', 'cpa'],
      startedAtMs: 1,
      updatedAtMs: 2,
    };
    const terminalOperation = {
      ...runningOperation,
      status: 'succeeded',
      progress: 100,
      message: 'update completed',
      updatedAtMs: 4,
      completedAtMs: 4,
    };
    const initialStatus = runtimeStatus();
    const terminalStatus = runtimeStatus();
    if (!initialStatus.runtime || !terminalStatus.runtime) {
      throw new Error('runtime fixture is unavailable');
    }
    initialStatus.runtime.latestOperationId = runningOperation.id;
    initialStatus.runtime.operations = { [runningOperation.id]: runningOperation };
    terminalStatus.runtime.latestOperationId = terminalOperation.id;
    terminalStatus.runtime.operations = { [terminalOperation.id]: terminalOperation };
    mocks.getRuntimeStatus
      .mockResolvedValueOnce(initialStatus)
      .mockResolvedValueOnce(terminalStatus);
    const stalePoll = deferred<typeof runningOperation>();
    mocks.getRuntimeOperation.mockImplementationOnce(() => stalePoll.promise);

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 700));
    });
    expect(mocks.getRuntimeOperation).toHaveBeenCalledTimes(1);

    await act(async () => {
      findButton(renderer.root, 'common.refresh')?.props.onClick();
      await flush();
    });
    expect(textContent(renderer.root)).toContain('system_info.runtime_operation_succeeded');

    await act(async () => {
      stalePoll.resolve({ ...runningOperation, progress: 55, updatedAtMs: 3 });
      await flush();
    });

    expect(textContent(renderer.root)).toContain('system_info.runtime_operation_succeeded');
    expect(textContent(renderer.root)).not.toContain('system_info.runtime_operation_running');
    renderer.unmount();
  });

  it('uses the direct operation token while the Manager is unavailable during handoff', async () => {
    const runningOperation = {
      id: 'update-direct-fallback',
      kind: 'cpamp',
      status: 'handoff_pending',
      progress: 95,
      message: 'restarting runtime to complete update',
      components: ['cpamp'],
      startedAtMs: 1,
      updatedAtMs: 2,
    };
    const terminalOperation = {
      ...runningOperation,
      status: 'succeeded',
      progress: 100,
      message: 'update completed',
      updatedAtMs: 3,
      completedAtMs: 3,
    };
    storeRuntimeUpdateCheckCache(
      runtimeUpdateCheckScope(mocks.authState.apiBase, mocks.authState.managementKey),
      updateCheck
    );
    mocks.startRuntimeUpdate.mockResolvedValueOnce({
      operationToken: 'direct-operation-token',
      operation: runningOperation,
    });
    mocks.getRuntimeOperation.mockRejectedValueOnce(new Error('manager restarting'));
    mocks.getRuntimeOperationDirect.mockResolvedValueOnce(terminalOperation);

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });
    await act(async () => {
      findButton(renderer.root, 'system_info.runtime_update_cpamp')?.props.onClick();
      await flush();
    });
    await act(async () => {
      findButton(renderer.root, 'system_info.runtime_update_confirm_action')?.props.onClick();
      await flush();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 700));
      await flush();
    });

    expect(mocks.getRuntimeOperation).toHaveBeenCalledWith(
      'http://manager.local',
      'cpamp-admin-key',
      runningOperation.id
    );
    expect(mocks.getRuntimeOperationDirect).toHaveBeenCalledWith(
      'http://manager.local',
      runningOperation.id,
      'direct-operation-token'
    );
    expect(textContent(renderer.root)).toContain('system_info.runtime_operation_succeeded');
    expect(mocks.showNotification).toHaveBeenCalledWith(
      'system_info.runtime_update_succeeded',
      'success'
    );
    renderer.unmount();
  });

  it('accepts a terminal poll that shares a millisecond timestamp with the running state', async () => {
    const runningOperation = {
      id: 'update-same-millisecond',
      kind: 'all',
      status: 'running',
      progress: 40,
      message: 'updating cpa',
      components: ['cpamp', 'cpa'],
      startedAtMs: 1,
      updatedAtMs: 2,
    };
    const terminalOperation = {
      ...runningOperation,
      status: 'succeeded',
      progress: 100,
      message: 'update completed',
      completedAtMs: 2,
    };
    const runningStatus = runtimeStatus();
    const terminalStatus = runtimeStatus();
    if (!runningStatus.runtime || !terminalStatus.runtime) {
      throw new Error('runtime fixture is unavailable');
    }
    runningStatus.runtime.latestOperationId = runningOperation.id;
    runningStatus.runtime.operations = { [runningOperation.id]: runningOperation };
    terminalStatus.runtime.latestOperationId = terminalOperation.id;
    terminalStatus.runtime.operations = { [terminalOperation.id]: terminalOperation };
    mocks.getRuntimeStatus.mockResolvedValueOnce(runningStatus).mockResolvedValue(terminalStatus);
    mocks.getRuntimeOperation.mockResolvedValue(terminalOperation);

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 700));
      await flush();
    });

    expect(textContent(renderer.root)).toContain('system_info.runtime_operation_succeeded');
    expect(mocks.showNotification).toHaveBeenCalledWith(
      'system_info.runtime_update_succeeded',
      'success'
    );
    renderer.unmount();
  });

  it('keeps polling while the replacement runtime is awaiting handoff confirmation', async () => {
    const runningOperation = {
      id: 'update-handoff',
      kind: 'cpamp',
      status: 'running',
      progress: 80,
      message: 'updating cpamp',
      components: ['cpamp'],
      startedAtMs: 1,
      updatedAtMs: 2,
    };
    const handoffOperation = {
      ...runningOperation,
      status: 'handoff_pending',
      progress: 95,
      message: 'restarting runtime to complete update',
    };
    const completedOperation = {
      ...handoffOperation,
      status: 'succeeded',
      progress: 100,
      message: 'update completed',
      updatedAtMs: 3,
      completedAtMs: 3,
    };
    const status = runtimeStatus();
    if (!status.runtime) throw new Error('runtime fixture is unavailable');
    status.runtime.latestOperationId = runningOperation.id;
    status.runtime.operations = { [runningOperation.id]: runningOperation };
    mocks.getRuntimeStatus.mockResolvedValue(status);
    mocks.getRuntimeOperation
      .mockResolvedValueOnce(handoffOperation)
      .mockResolvedValue(completedOperation);

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 700));
      await flush();
    });

    const handoffContent = textContent(renderer.root);
    expect(handoffContent).toContain('system_info.runtime_operation_handoff_pending');
    expect(handoffContent).toContain('system_info.runtime_operation_message_handoff_pending');
    expect(mocks.showNotification).not.toHaveBeenCalledWith(
      'system_info.runtime_update_succeeded',
      'success'
    );

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 700));
      await flush();
    });

    expect(textContent(renderer.root)).toContain('system_info.runtime_operation_succeeded');
    expect(mocks.getRuntimeOperation).toHaveBeenCalledTimes(2);
    expect(mocks.showNotification).toHaveBeenCalledWith(
      'system_info.runtime_update_succeeded',
      'success'
    );
    renderer.unmount();
  });

  it('restores and keeps polling the latest non-terminal runtime operation', async () => {
    const runningOperation = {
      id: 'update-running',
      kind: 'all',
      status: 'running',
      progress: 45,
      message: 'updating cpa',
      components: ['cpamp', 'cpa'],
      startedAtMs: 1,
      updatedAtMs: 2,
    };
    const status = runtimeStatus();
    if (!status.runtime) throw new Error('runtime fixture is unavailable');
    status.runtime.latestOperationId = runningOperation.id;
    status.runtime.operations = { [runningOperation.id]: runningOperation };
    mocks.getRuntimeStatus.mockResolvedValue(status);
    mocks.getRuntimeOperation.mockResolvedValue({
      ...runningOperation,
      progress: 55,
      updatedAtMs: 3,
    });

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    expect(textContent(renderer.root)).toContain('system_info.runtime_operation_running');
    expect(textContent(renderer.root)).toContain(
      'system_info.runtime_operation_message_updating_component'
    );
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 700));
    });
    expect(mocks.getRuntimeOperation).toHaveBeenCalledWith(
      'http://manager.local',
      'cpamp-admin-key',
      'update-running'
    );
    expect(mocks.getRuntimeOperation).toHaveBeenCalledTimes(1);
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 700));
    });
    expect(mocks.getRuntimeOperation).toHaveBeenCalledTimes(1);
    renderer.unmount();
  });

  it('translates known operation messages and component result statuses', async () => {
    const completedOperation = {
      id: 'update-completed',
      kind: 'all',
      status: 'succeeded',
      progress: 100,
      message: 'update completed',
      components: ['cpamp', 'cpa'],
      results: {
        cpamp: { name: 'cpamp', status: 'succeeded' },
        cpa: { name: 'cpa', status: 'rollback_failed' },
      },
      startedAtMs: 1,
      updatedAtMs: 4,
      completedAtMs: 4,
    };
    const status = runtimeStatus();
    if (!status.runtime) throw new Error('runtime fixture is unavailable');
    status.runtime.latestOperationId = completedOperation.id;
    status.runtime.operations = { [completedOperation.id]: completedOperation };
    mocks.getRuntimeStatus.mockResolvedValue(status);

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    const content = textContent(renderer.root);
    expect(content).toContain('system_info.runtime_operation_message_completed');
    expect(content).toContain('system_info.runtime_result_succeeded');
    expect(content).toContain('system_info.runtime_result_rollback_failed');
    renderer.unmount();
  });

  it('restores the latest terminal runtime operation after a refresh', async () => {
    const failedOperation = {
      id: 'update-failed',
      kind: 'all',
      status: 'failed',
      progress: 80,
      message: 'rollback needs attention',
      error: 'CPA health check failed',
      components: ['cpamp', 'cpa'],
      startedAtMs: 1,
      updatedAtMs: 4,
      completedAtMs: 4,
    };
    const status = runtimeStatus();
    if (!status.runtime) throw new Error('runtime fixture is unavailable');
    status.runtime.latestOperationId = failedOperation.id;
    status.runtime.operations = { [failedOperation.id]: failedOperation };
    mocks.getRuntimeStatus.mockResolvedValue(status);

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    expect(textContent(renderer.root)).toContain('system_info.runtime_operation_failed');
    expect(textContent(renderer.root)).toContain('rollback needs attention');
    expect(textContent(renderer.root)).toContain('CPA health check failed');
    expect(mocks.getRuntimeOperation).not.toHaveBeenCalled();
    renderer.unmount();
  });

  it('ignores a stale runtime status response after the Manager connection changes', async () => {
    const firstStatus = deferred<RuntimeStatusResult>();
    const nextStatus = runtimeStatus({ panelBasePath: '/new-manager' });
    mocks.getRuntimeStatus
      .mockImplementationOnce(() => firstStatus.promise)
      .mockResolvedValueOnce(nextStatus);

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    mocks.authState.apiBase = 'http://new-manager.local';
    await act(async () => {
      renderer.update(<RuntimeUpdatePanel />);
      await flush();
    });
    expect(renderer.root.findByType('input').props.value).toBe('/new-manager');

    await act(async () => {
      firstStatus.resolve(runtimeStatus({ panelBasePath: '/stale-manager' }));
      await flush();
    });
    expect(renderer.root.findByType('input').props.value).toBe('/new-manager');
    renderer.unmount();
  });

  it('disables runtime-managed actions while runtime control is unavailable', async () => {
    storeRuntimeUpdateCheckCache(
      runtimeUpdateCheckScope(mocks.authState.apiBase, mocks.authState.managementKey),
      updateCheck
    );
    const unavailable = runtimeStatus();
    unavailable.runtimeAvailable = false;
    unavailable.runtimeUnavailableReason = 'runtime_control_unreachable';
    unavailable.runtime = undefined;
    mocks.getRuntimeStatus.mockResolvedValue(unavailable);

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    expect(findButton(renderer.root, 'system_info.runtime_check_updates')?.props.disabled).toBe(
      true
    );
    expect(findButton(renderer.root, 'system_info.runtime_update_cpamp')?.props.disabled).toBe(
      true
    );
    expect(findButton(renderer.root, 'system_info.runtime_update_cpa')?.props.disabled).toBe(true);
    expect(findButton(renderer.root, 'system_info.runtime_update_all')?.props.disabled).toBe(true);
    expect(
      readRuntimeUpdateCheckCache(
        runtimeUpdateCheckScope(mocks.authState.apiBase, mocks.authState.managementKey)
      )
    ).toBeNull();
    expect(findButton(renderer.root, 'common.save')?.props.disabled).toBe(true);
    expect(renderer.root.findByType('input').props.disabled).toBe(true);
    expect(textContent(renderer.root)).toContain(
      'usage_service_errors.runtime_control_unavailable'
    );
    renderer.unmount();
  });

  it('treats environment-managed Base Paths as read-only', async () => {
    mocks.getRuntimeStatus.mockResolvedValue(
      runtimeStatus({ panelBasePath: '/admin', panelBasePathSource: 'environment' }, false)
    );
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    const input = renderer.root.findByType('input');
    const saveButton = findButton(renderer.root, 'common.save');
    expect(input.props.disabled).toBe(true);
    expect(saveButton?.props.disabled).toBe(true);
    expect(textContent(renderer.root)).toContain('system_info.runtime_base_path_env_managed');
    renderer.unmount();
  });

  it('updates a dynamic Base Path and redirects to the new entry', async () => {
    mocks.updateRuntimePanelBasePath.mockResolvedValue({
      ok: true,
      panelPath: '/admin',
      deployment: runtimeStatus({ panelBasePath: '/admin' }).deployment,
    });
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    await act(async () => {
      renderer.root.findByType('input').props.onChange({ target: { value: '/admin' } });
      await flush();
    });
    const saveButton = findButton(renderer.root, 'common.save');
    expect(saveButton?.props.disabled).toBe(false);
    await act(async () => {
      saveButton?.props.onClick();
      await flush();
    });

    expect(mocks.showConfirmation).toHaveBeenCalled();
    expect(mocks.updateRuntimePanelBasePath).toHaveBeenCalledWith(
      'http://manager.local',
      'cpamp-admin-key',
      '/admin'
    );
    expect(mocks.locationAssign).toHaveBeenCalledWith('/admin?locale=zh-CN#/system');
    renderer.unmount();
  });

  it('preserves the query and hash when the panel Base Path is the root', async () => {
    mocks.getRuntimeStatus.mockResolvedValue(runtimeStatus({ panelBasePath: '/admin' }));
    mocks.updateRuntimePanelBasePath.mockResolvedValue({
      ok: true,
      panelPath: '/',
      deployment: runtimeStatus({ panelBasePath: '/' }).deployment,
    });
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<RuntimeUpdatePanel />);
      await flush();
    });

    await act(async () => {
      renderer.root.findByType('input').props.onChange({ target: { value: '/' } });
      await flush();
    });
    await act(async () => {
      findButton(renderer.root, 'common.save')?.props.onClick();
      await flush();
    });

    expect(mocks.locationAssign).toHaveBeenCalledWith('/?locale=zh-CN#/system');
    renderer.unmount();
  });
});
