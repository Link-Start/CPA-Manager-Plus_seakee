import { act } from 'react';
import { create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type {
  UsageArchiveList,
  UsageArchivePreview,
  UsageArchiveRunSummary,
  UsageMaintenanceStatus,
} from '@/services/api/usageService';
import { formatDateTime } from '@/utils/format';
import { UsageMaintenancePage } from './UsageMaintenancePage';

const { mocks } = vi.hoisted(() => {
  (
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true;
  return {
    mocks: {
      availability: {
        checking: false,
        managerServiceBase: 'http://manager-a.local:18317',
      },
      managementKey: 'management-key-a',
      showNotification: vi.fn(),
      showConfirmation: vi.fn(),
      probeUsageMaintenance: vi.fn(),
      getUsageMaintenance: vi.fn(),
      listUsageArchives: vi.fn(),
      previewUsageArchive: vi.fn(),
      createUsageArchive: vi.fn(),
      resumeUsageArchive: vi.fn(),
      verifyUsageArchive: vi.fn(),
      deleteUsageArchive: vi.fn(),
      t: vi.fn((key: string, options?: Record<string, unknown>) => {
        if (
          [
            'usage_maintenance.run_status_',
            'usage_maintenance.run_mode_',
            'usage_maintenance.migration_status_',
            'usage_maintenance.aggregate_status_',
          ].some((prefix) => key.startsWith(prefix))
        ) {
          return `translated:${key}`;
        }
        let value = typeof options?.defaultValue === 'string' ? options.defaultValue : key;
        for (const [name, replacement] of Object.entries(options ?? {})) {
          if (name !== 'defaultValue') {
            value = value.split(`{{${name}}}`).join(String(replacement));
          }
        }
        return value;
      }),
    },
  };
});

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    i18n: { language: 'en' },
    t: mocks.t,
  }),
}));

vi.mock('@/hooks/usePanelFeatureAvailability', () => ({
  usePanelFeatureAvailability: () => mocks.availability,
}));

vi.mock('@/stores', () => ({
  useAuthStore: (selector: (state: { managementKey: string }) => unknown) =>
    selector({ managementKey: mocks.managementKey }),
  useNotificationStore: () => ({
    showNotification: mocks.showNotification,
    showConfirmation: mocks.showConfirmation,
  }),
}));

vi.mock('@/services/api/usageService', () => ({
  usageServiceApi: {
    probeUsageMaintenance: mocks.probeUsageMaintenance,
    getUsageMaintenance: mocks.getUsageMaintenance,
    listUsageArchives: mocks.listUsageArchives,
    previewUsageArchive: mocks.previewUsageArchive,
    createUsageArchive: mocks.createUsageArchive,
    resumeUsageArchive: mocks.resumeUsageArchive,
    verifyUsageArchive: mocks.verifyUsageArchive,
    deleteUsageArchive: mocks.deleteUsageArchive,
  },
}));

vi.mock('@/components/ui/LoadingSpinner', () => ({
  LoadingSpinner: () => <div>full-screen-loading</div>,
}));

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
};

const archive = (
  status: UsageArchiveRunSummary['status'],
  id = `run-${status}`
): UsageArchiveRunSummary => ({
  id,
  mode: 'manual',
  status,
  cutoff_timestamp_ms: 1_700_000_000_000,
  target_event_id: 100,
  event_count: 10,
  estimated_bytes: 1_024,
  last_archived_event_id: status === 'previewed' ? 0 : 100,
  archived_event_count: status === 'previewed' ? 0 : 10,
  archived_uncompressed_bytes: 1_024,
  archived_compressed_bytes: 256,
  last_deleted_event_id: status === 'completed' ? 100 : 0,
  deleted_event_count: status === 'completed' ? 10 : 0,
  created_at_ms: 1_700_000_000_000,
  updated_at_ms: 1_700_000_001_000,
  has_error: status === 'failed',
});

const maintenance = (overrides: Partial<UsageMaintenanceStatus> = {}): UsageMaintenanceStatus => ({
  raw_event_count: 10,
  raw_deleted_event_count: 2,
  migration: {
    name: 'usage_cache_accounting_v2',
    status: 'completed',
    last_event_id: 100,
    target_event_id: 100,
    processed_rows: 100,
    changed_rows: 2,
    updated_at_ms: 1_700_000_000_000,
  },
  hourly_aggregate: {
    name: 'hourly_core',
    schema_version: 1,
    status: 'ready',
    coverage_event_id: 100,
    target_event_id: 100,
    updated_at_ms: 1_700_000_000_000,
  },
  readiness: {
    migration_ready: true,
    hourly_aggregate_ready: true,
    archive_delete_enabled: true,
  },
  storage: {
    page_size: 4_096,
    page_count: 20,
    freelist_count: 1,
    reclaimable_bytes: 4_096,
    database_bytes: 81_920,
    wal_bytes: 0,
    shm_bytes: 0,
    total_bytes: 81_920,
  },
  compact_requires_stopped_server: true,
  ...overrides,
});

const getText = (node: ReactTestInstance): string =>
  node.children
    .map((child) => {
      if (typeof child === 'string' || typeof child === 'number') return String(child);
      return getText(child);
    })
    .join('');

const findButtons = (renderer: ReactTestRenderer, text: string) =>
  renderer.root.findAllByType('button').filter((button) => getText(button).includes(text));

const renderResolvedPage = async (status = maintenance(), runs: UsageArchiveRunSummary[] = []) => {
  mocks.getUsageMaintenance.mockResolvedValue(status);
  mocks.listUsageArchives.mockResolvedValue({ runs } satisfies UsageArchiveList);
  let renderer!: ReactTestRenderer;
  await act(async () => {
    renderer = create(<UsageMaintenancePage />);
  });
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
  return renderer;
};

beforeEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
  mocks.availability.checking = false;
  mocks.availability.managerServiceBase = 'http://manager-a.local:18317';
  mocks.managementKey = 'management-key-a';
  mocks.probeUsageMaintenance.mockResolvedValue(undefined);
  mocks.createUsageArchive.mockResolvedValue({});
  mocks.resumeUsageArchive.mockResolvedValue({});
  mocks.verifyUsageArchive.mockResolvedValue({});
  mocks.deleteUsageArchive.mockResolvedValue({});
});

afterEach(() => {
  vi.useRealTimers();
});

describe('UsageMaintenancePage', () => {
  it('invalidates a preview when cutoff changes and creates with the preview cutoff', async () => {
    const renderer = await renderResolvedPage();
    const firstCutoff = '2026-07-01T12:00';
    const secondCutoff = '2026-07-02T12:00';
    const expectedCutoff = new Date(firstCutoff).getTime();
    mocks.previewUsageArchive.mockResolvedValue({
      cutoff_timestamp_ms: expectedCutoff,
      target_event_id: 100,
      event_count: 7,
      estimated_bytes: 2_048,
    });

    const input = renderer.root.findByType('input');
    act(() => input.props.onChange({ target: { value: firstCutoff } }));
    await act(async () => {
      findButtons(renderer, 'Preview')[0].props.onClick();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(findButtons(renderer, 'Create archive run')).toHaveLength(1);

    act(() => findButtons(renderer, 'Create archive run')[0].props.onClick());
    const confirmation = mocks.showConfirmation.mock.calls[0][0] as {
      onConfirm: () => Promise<void>;
    };
    act(() => input.props.onChange({ target: { value: secondCutoff } }));
    expect(findButtons(renderer, 'Create archive run')).toHaveLength(0);

    await act(async () => {
      await confirmation.onConfirm();
    });
    expect(mocks.createUsageArchive).toHaveBeenCalledWith(
      'http://manager-a.local:18317',
      expectedCutoff,
      'management-key-a',
      expect.any(AbortSignal)
    );
    act(() => renderer.unmount());
  });

  it('reloads persisted previewed state without polling when the create response is lost', async () => {
    vi.useFakeTimers();
    const renderer = await renderResolvedPage();
    const previewCutoff = 1_700_000_000_000;
    mocks.previewUsageArchive.mockResolvedValue({
      cutoff_timestamp_ms: previewCutoff,
      target_event_id: 100,
      event_count: 7,
      estimated_bytes: 2_048,
    });
    const active = archive('previewed', 'persisted-after-timeout');
    mocks.createUsageArchive.mockRejectedValueOnce(new Error('create response lost'));
    mocks.getUsageMaintenance.mockResolvedValueOnce(maintenance({ active_run: active }));
    mocks.listUsageArchives.mockResolvedValueOnce({ runs: [active] });

    await act(async () => {
      findButtons(renderer, 'Preview')[0].props.onClick();
      await Promise.resolve();
      await Promise.resolve();
    });
    act(() => findButtons(renderer, 'Create archive run')[0].props.onClick());
    const confirmation = mocks.showConfirmation.mock.calls[0][0] as {
      onConfirm: () => Promise<void>;
    };
    await act(async () => {
      await confirmation.onConfirm();
    });

    expect(mocks.showNotification).toHaveBeenCalledWith('create response lost', 'error');
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(2);
    expect(getText(renderer.root)).toContain('persisted-after-timeout');

    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(2);
    act(() => renderer.unmount());
  });

  it('shows the unsupported state for legacy method-not-allowed responses', async () => {
    mocks.probeUsageMaintenance.mockRejectedValueOnce({ status: 405 });
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<UsageMaintenancePage />);
    });
    expect(getText(renderer.root)).toContain('older than the usage maintenance API');
    expect(getText(renderer.root)).not.toContain('full-screen-loading');
    expect(mocks.probeUsageMaintenance).toHaveBeenCalledWith(
      'http://manager-a.local:18317',
      'management-key-a',
      expect.any(AbortSignal)
    );
    expect(mocks.getUsageMaintenance).not.toHaveBeenCalled();
    expect(mocks.listUsageArchives).not.toHaveBeenCalled();
    act(() => renderer.unmount());
  });

  it('shows archive configuration failures instead of the legacy-server state', async () => {
    mocks.getUsageMaintenance.mockRejectedValueOnce(
      Object.assign(new Error('usage archive is unavailable'), {
        status: 503,
        code: 'usage_archive_unavailable',
      })
    );
    mocks.listUsageArchives.mockResolvedValueOnce({ runs: [] });

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<UsageMaintenancePage />);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(getText(renderer.root)).toContain('usage archive is unavailable');
    expect(getText(renderer.root)).not.toContain('older than the usage maintenance API');
    act(() => renderer.unmount());
  });

  it('aborts the sibling load when one maintenance request fails', async () => {
    let siblingSignal: AbortSignal | undefined;
    mocks.getUsageMaintenance.mockRejectedValueOnce({ status: 404 });
    mocks.listUsageArchives.mockImplementationOnce(
      (_base: string, _key: string, _limit: number, signal: AbortSignal) => {
        siblingSignal = signal;
        return new Promise<UsageArchiveList>(() => {});
      }
    );

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<UsageMaintenancePage />);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(siblingSignal?.aborted).toBe(true);
    expect(getText(renderer.root)).toContain('older than the usage maintenance API');
    act(() => renderer.unmount());
  });

  it('shows the unsupported state for a legacy maintenance payload returned with 200', async () => {
    mocks.getUsageMaintenance.mockResolvedValueOnce({ events: [] });
    mocks.listUsageArchives.mockResolvedValueOnce({ runs: [] });
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<UsageMaintenancePage />);
    });
    expect(getText(renderer.root)).toContain('older than the usage maintenance API');
    expect(getText(renderer.root)).not.toContain('full-screen-loading');
    expect(mocks.probeUsageMaintenance).toHaveBeenCalledTimes(1);
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(1);
    expect(mocks.listUsageArchives).toHaveBeenCalledTimes(1);
    act(() => renderer.unmount());
  });

  it('shows the unsupported state for a legacy archive-list payload returned with 200', async () => {
    mocks.getUsageMaintenance.mockResolvedValueOnce(maintenance());
    mocks.listUsageArchives.mockResolvedValueOnce({ events: [] });
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<UsageMaintenancePage />);
    });
    expect(getText(renderer.root)).toContain('older than the usage maintenance API');
    expect(getText(renderer.root)).not.toContain('full-screen-loading');
    expect(mocks.probeUsageMaintenance).toHaveBeenCalledTimes(1);
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(1);
    expect(mocks.listUsageArchives).toHaveBeenCalledTimes(1);
    act(() => renderer.unmount());
  });

  it('shows the unsupported state for a malformed preview payload returned with 200', async () => {
    mocks.previewUsageArchive.mockResolvedValueOnce({
      cutoff_timestamp_ms: 1_700_000_000_000,
      target_event_id: 100,
      event_count: 7,
      estimated_bytes: 2_048,
      min_timestamp_ms: Number.NaN,
    });
    const renderer = await renderResolvedPage();

    await act(async () => {
      findButtons(renderer, 'Preview')[0].props.onClick();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(getText(renderer.root)).toContain('older than the usage maintenance API');
    expect(findButtons(renderer, 'Create archive run')).toHaveLength(0);
    expect(mocks.showNotification).not.toHaveBeenCalled();
    act(() => renderer.unmount());
  });

  it('offers Resume for verifying and deleting runs', async () => {
    const renderer = await renderResolvedPage(maintenance(), [
      archive('verifying'),
      archive('deleting'),
    ]);
    expect(findButtons(renderer, 'Resume')).toHaveLength(2);
    act(() => renderer.unmount());
  });

  it('translates known statuses and modes while preserving unknown server values', async () => {
    const renderer = await renderResolvedPage(maintenance(), [
      archive('verified', 'known-run'),
      { ...archive('verified', 'retention-run'), mode: 'retention' },
      { ...archive('future-state', 'unknown-run'), mode: 'future-mode' },
    ]);
    const text = getText(renderer.root);
    expect(text).toContain('translated:usage_maintenance.run_status_verified');
    expect(text).toContain('translated:usage_maintenance.run_mode_manual');
    expect(text).toContain('translated:usage_maintenance.migration_status_completed');
    expect(text).toContain('translated:usage_maintenance.aggregate_status_ready');
    expect(text).toContain('future-state');
    expect(text).toContain('future-mode');
    expect(mocks.t).not.toHaveBeenCalledWith(
      'usage_maintenance.run_status_future-state',
      expect.anything()
    );
    const verifiedBadges = renderer.root
      .findAllByType('span')
      .filter((node) => getText(node) === 'translated:usage_maintenance.run_status_verified');
    expect(verifiedBadges).toHaveLength(2);
    expect(verifiedBadges[0].props.className).not.toContain('badgeActive');
    expect(verifiedBadges[1].props.className).toContain('badgeActive');
    act(() => renderer.unmount());
  });

  it('identifies the run, event count, and cutoff in destructive confirmation', async () => {
    const run = {
      ...archive('verified', 'delete-target-run'),
      event_count: 12_345,
      deleted_event_count: 345,
      cutoff_timestamp_ms: 1_700_000_000_000,
    };
    const renderer = await renderResolvedPage(maintenance(), [run]);
    act(() => findButtons(renderer, 'Delete raw')[0].props.onClick());
    const confirmation = mocks.showConfirmation.mock.calls[0][0] as { message: string };
    expect(confirmation.message).toContain(run.id);
    expect(confirmation.message).toContain(
      (run.event_count - run.deleted_event_count).toLocaleString('en')
    );
    expect(confirmation.message).toContain(run.event_count.toLocaleString('en'));
    expect(confirmation.message).toContain(formatDateTime(new Date(run.cutoff_timestamp_ms), 'en'));
    expect(mocks.t).toHaveBeenCalledWith(
      'usage_maintenance.delete_confirm_message',
      expect.objectContaining({
        runId: run.id,
        remainingEventCount: (run.event_count - run.deleted_event_count).toLocaleString('en'),
        totalEventCount: run.event_count.toLocaleString('en'),
        cutoff: formatDateTime(new Date(run.cutoff_timestamp_ms), 'en'),
      })
    );
    act(() => renderer.unmount());
  });

  it('requires destructive confirmation before resuming delete stages', async () => {
    const deletingRun = { ...archive('deleting', 'deleting-run'), deleted_event_count: 3 };
    const failedDeletingRun = {
      ...archive('failed', 'failed-deleting-run'),
      resume_status: 'deleting',
      deleted_event_count: 4,
    };
    const renderer = await renderResolvedPage(maintenance(), [deletingRun, failedDeletingRun]);
    const resumeButtons = findButtons(renderer, 'Resume');
    expect(resumeButtons).toHaveLength(2);
    expect(resumeButtons.every((button) => button.props.className.includes('btn-danger'))).toBe(
      true
    );

    act(() => resumeButtons[0].props.onClick());
    act(() => resumeButtons[1].props.onClick());
    expect(mocks.resumeUsageArchive).not.toHaveBeenCalled();
    expect(mocks.showConfirmation).toHaveBeenCalledTimes(2);

    const confirmations = mocks.showConfirmation.mock.calls.map(
      (call) => call[0] as { message: string; onConfirm: () => Promise<void> }
    );
    expect(confirmations[0].message).toContain('deleting-run');
    expect(confirmations[0].message).toContain('7');
    expect(confirmations[1].message).toContain('failed-deleting-run');
    expect(confirmations[1].message).toContain('6');
    await act(async () => {
      await confirmations[0].onConfirm();
      await confirmations[1].onConfirm();
    });
    expect(mocks.resumeUsageArchive).toHaveBeenCalledTimes(2);
    expect(mocks.resumeUsageArchive).toHaveBeenNthCalledWith(
      1,
      'http://manager-a.local:18317',
      deletingRun.id,
      'management-key-a',
      expect.any(AbortSignal)
    );
    expect(mocks.resumeUsageArchive).toHaveBeenNthCalledWith(
      2,
      'http://manager-a.local:18317',
      failedDeletingRun.id,
      'management-key-a',
      expect.any(AbortSignal)
    );
    expect(mocks.showNotification).toHaveBeenCalledWith('Logical deletion completed.', 'success');
    act(() => renderer.unmount());
  });

  it('translates active migration and aggregate phases and only hard-disables delete when archive deletion is disabled', async () => {
    const pending = maintenance({
      migration: { ...maintenance().migration, status: 'applying' },
      hourly_aggregate: { ...maintenance().hourly_aggregate, status: 'catching_up' },
      readiness: {
        migration_ready: false,
        hourly_aggregate_ready: false,
        archive_delete_enabled: true,
      },
    });
    const renderer = await renderResolvedPage(pending, [archive('verified')]);
    let deleteButton = findButtons(renderer, 'Delete raw')[0];
    expect(deleteButton.props.disabled).toBe(false);
    expect(deleteButton.props.title).toContain('exact coverage');
    expect(getText(renderer.root)).toContain(
      'translated:usage_maintenance.migration_status_applying'
    );
    expect(getText(renderer.root)).toContain(
      'translated:usage_maintenance.aggregate_status_catching_up'
    );
    expect(getText(renderer.root)).toContain('Hourly aggregate');

    act(() => renderer.unmount());
    const clearingRenderer = await renderResolvedPage(
      maintenance({
        migration: { ...maintenance().migration, status: 'clearing' },
        hourly_aggregate: { ...maintenance().hourly_aggregate, status: 'clearing' },
      })
    );
    expect(getText(clearingRenderer.root)).toContain(
      'translated:usage_maintenance.migration_status_clearing'
    );
    expect(getText(clearingRenderer.root)).toContain(
      'translated:usage_maintenance.aggregate_status_clearing'
    );
    act(() => clearingRenderer.unmount());

    const disabledRenderer = await renderResolvedPage(
      maintenance({
        readiness: {
          migration_ready: true,
          hourly_aggregate_ready: true,
          archive_delete_enabled: false,
        },
      }),
      [
        archive('verified'),
        archive('deleting'),
        { ...archive('failed'), resume_status: 'deleting' },
      ]
    );
    deleteButton = findButtons(disabledRenderer, 'Delete raw')[0];
    expect(deleteButton.props.disabled).toBe(true);
    expect(deleteButton.props.title).toContain('disabled');
    const destructiveResumeButtons = findButtons(disabledRenderer, 'Resume');
    expect(destructiveResumeButtons).toHaveLength(2);
    expect(destructiveResumeButtons.every((button) => button.props.disabled)).toBe(true);
    expect(
      destructiveResumeButtons.every((button) => String(button.props.title).includes('disabled'))
    ).toBe(true);
    act(() => disabledRenderer.unmount());
  });

  it('aborts stale base/key loads and prevents old responses from replacing the new context', async () => {
    const oldMaintenance = deferred<UsageMaintenanceStatus>();
    const oldArchives = deferred<UsageArchiveList>();
    mocks.getUsageMaintenance.mockImplementation((base: string) =>
      base.includes('manager-a')
        ? oldMaintenance.promise
        : Promise.resolve(maintenance({ raw_event_count: 22 }))
    );
    mocks.listUsageArchives.mockImplementation((base: string) =>
      base.includes('manager-a')
        ? oldArchives.promise
        : Promise.resolve({ runs: [archive('completed', 'new-context-run')] })
    );

    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(<UsageMaintenancePage />);
      await Promise.resolve();
      await Promise.resolve();
    });
    const oldMaintenanceSignal = mocks.getUsageMaintenance.mock.calls[0][2] as AbortSignal;
    const oldArchivesSignal = mocks.listUsageArchives.mock.calls[0][3] as AbortSignal;

    mocks.availability.managerServiceBase = 'http://manager-b.local:18317';
    mocks.managementKey = 'management-key-b';
    await act(async () => {
      renderer.update(<UsageMaintenancePage />);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(oldMaintenanceSignal.aborted).toBe(true);
    expect(oldArchivesSignal.aborted).toBe(true);
    expect(mocks.probeUsageMaintenance).toHaveBeenCalledTimes(2);
    expect(mocks.probeUsageMaintenance).toHaveBeenNthCalledWith(
      2,
      'http://manager-b.local:18317',
      'management-key-b',
      expect.any(AbortSignal)
    );
    expect(getText(renderer.root)).toContain('22');
    expect(getText(renderer.root)).toContain('new-context-run');

    await act(async () => {
      oldMaintenance.resolve(maintenance({ raw_event_count: 999 }));
      oldArchives.resolve({ runs: [archive('completed', 'stale-context-run')] });
      await Promise.resolve();
    });
    expect(getText(renderer.root)).not.toContain('999');
    expect(getText(renderer.root)).not.toContain('stale-context-run');
    act(() => renderer.unmount());
  });

  it('clears old-context data when the new archive configuration is unavailable', async () => {
    const renderer = await renderResolvedPage(maintenance({ raw_event_count: 11 }), [
      archive('completed', 'old-context-run'),
    ]);

    mocks.availability.managerServiceBase = 'http://manager-b.local:18317';
    mocks.managementKey = 'management-key-b';
    mocks.getUsageMaintenance.mockRejectedValueOnce(
      Object.assign(new Error('usage archive is unavailable'), {
        status: 503,
        code: 'usage_archive_unavailable',
      })
    );
    mocks.listUsageArchives.mockResolvedValueOnce({ runs: [] });
    await act(async () => {
      renderer.update(<UsageMaintenancePage />);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(getText(renderer.root)).toContain('usage archive is unavailable');
    expect(getText(renderer.root)).not.toContain('older than the usage maintenance API');
    expect(getText(renderer.root)).not.toContain('old-context-run');
    act(() => renderer.unmount());
  });

  it('aborts and ignores a pending preview when base and key change', async () => {
    const previewRequest = deferred<UsageArchivePreview>();
    mocks.previewUsageArchive.mockImplementationOnce(() => previewRequest.promise);
    const renderer = await renderResolvedPage();
    act(() => findButtons(renderer, 'Preview')[0].props.onClick());
    const oldSignal = mocks.previewUsageArchive.mock.calls[0][3] as AbortSignal;

    mocks.availability.managerServiceBase = 'http://manager-b.local:18317';
    mocks.managementKey = 'management-key-b';
    await act(async () => {
      renderer.update(<UsageMaintenancePage />);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(oldSignal.aborted).toBe(true);

    await act(async () => {
      previewRequest.resolve({
        cutoff_timestamp_ms: 1_700_000_000_000,
        target_event_id: 100,
        event_count: 7,
        estimated_bytes: 2_048,
      });
      await Promise.resolve();
    });
    expect(findButtons(renderer, 'Create archive run')).toHaveLength(0);
    expect(findButtons(renderer, 'Preview')[0].props.disabled).toBe(false);
    expect(mocks.showNotification).not.toHaveBeenCalled();
    act(() => renderer.unmount());
  });

  it('isolates a pending mutation from a new-context operation', async () => {
    const oldRun = archive('previewed', 'old-context-run');
    const oldResume = deferred<unknown>();
    mocks.resumeUsageArchive.mockImplementationOnce(() => oldResume.promise);
    const renderer = await renderResolvedPage(maintenance({ active_run: oldRun }), [oldRun]);
    act(() => findButtons(renderer, 'Resume')[0].props.onClick());
    const oldSignal = mocks.resumeUsageArchive.mock.calls[0][3] as AbortSignal;

    mocks.availability.managerServiceBase = 'http://manager-b.local:18317';
    mocks.managementKey = 'management-key-b';
    mocks.getUsageMaintenance.mockResolvedValue(maintenance({ raw_event_count: 22 }));
    mocks.listUsageArchives.mockResolvedValue({
      runs: [archive('completed', 'new-context-run')],
    });
    await act(async () => {
      renderer.update(<UsageMaintenancePage />);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(oldSignal.aborted).toBe(true);
    expect(getText(renderer.root)).toContain('new-context-run');

    const newPreview = deferred<UsageArchivePreview>();
    mocks.previewUsageArchive.mockImplementationOnce(() => newPreview.promise);
    act(() => findButtons(renderer, 'Preview')[0].props.onClick());
    const newSignal = mocks.previewUsageArchive.mock.calls[0][3] as AbortSignal;
    expect(findButtons(renderer, 'Preview')[0].props.disabled).toBe(true);

    await act(async () => {
      oldResume.resolve({});
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(newSignal.aborted).toBe(false);
    expect(findButtons(renderer, 'Preview')[0].props.disabled).toBe(true);
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(2);
    expect(mocks.showNotification).not.toHaveBeenCalled();

    await act(async () => {
      newPreview.resolve({
        cutoff_timestamp_ms: 1_700_000_000_000,
        target_event_id: 100,
        event_count: 7,
        estimated_bytes: 2_048,
      });
      await Promise.resolve();
    });
    expect(findButtons(renderer, 'Create archive run')).toHaveLength(1);
    act(() => renderer.unmount());
  });

  it('does not execute a destructive confirmation after base and key change', async () => {
    const run = archive('verified', 'old-confirmation-run');
    const renderer = await renderResolvedPage(maintenance(), [run]);
    act(() => findButtons(renderer, 'Delete raw')[0].props.onClick());
    const confirmation = mocks.showConfirmation.mock.calls[0][0] as {
      onConfirm: () => Promise<void>;
    };

    mocks.availability.managerServiceBase = 'http://manager-b.local:18317';
    mocks.managementKey = 'management-key-b';
    mocks.getUsageMaintenance.mockResolvedValue(maintenance({ raw_event_count: 22 }));
    mocks.listUsageArchives.mockResolvedValue({ runs: [] });
    await act(async () => {
      renderer.update(<UsageMaintenancePage />);
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      await confirmation.onConfirm();
    });
    expect(mocks.deleteUsageArchive).not.toHaveBeenCalled();
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(2);
    act(() => renderer.unmount());
  });

  it('does not execute saved create or delete confirmations after unmount', async () => {
    mocks.previewUsageArchive.mockResolvedValueOnce({
      cutoff_timestamp_ms: 1_700_000_000_000,
      target_event_id: 100,
      event_count: 7,
      estimated_bytes: 2_048,
    });
    const createRenderer = await renderResolvedPage();
    await act(async () => {
      findButtons(createRenderer, 'Preview')[0].props.onClick();
      await Promise.resolve();
      await Promise.resolve();
    });
    act(() => findButtons(createRenderer, 'Create archive run')[0].props.onClick());
    const createConfirmation = mocks.showConfirmation.mock.calls[0][0] as {
      onConfirm: () => Promise<void>;
    };
    const createLoadCount = mocks.getUsageMaintenance.mock.calls.length;
    act(() => createRenderer.unmount());
    await createConfirmation.onConfirm();
    expect(mocks.createUsageArchive).not.toHaveBeenCalled();
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(createLoadCount);
    expect(mocks.showNotification).not.toHaveBeenCalled();

    vi.clearAllMocks();
    const run = archive('verified', 'unmounted-delete-run');
    const deleteRenderer = await renderResolvedPage(maintenance(), [run]);
    act(() => findButtons(deleteRenderer, 'Delete raw')[0].props.onClick());
    const deleteConfirmation = mocks.showConfirmation.mock.calls[0][0] as {
      onConfirm: () => Promise<void>;
    };
    const deleteLoadCount = mocks.getUsageMaintenance.mock.calls.length;
    act(() => deleteRenderer.unmount());
    await deleteConfirmation.onConfirm();
    expect(mocks.deleteUsageArchive).not.toHaveBeenCalled();
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(deleteLoadCount);
    expect(mocks.showNotification).not.toHaveBeenCalled();
  });

  it('does not execute an old destructive confirmation after an A-B-A context change', async () => {
    const run = archive('verified', 'aba-delete-run');
    const renderer = await renderResolvedPage(maintenance(), [run]);
    act(() => findButtons(renderer, 'Delete raw')[0].props.onClick());
    const confirmation = mocks.showConfirmation.mock.calls[0][0] as {
      onConfirm: () => Promise<void>;
    };

    mocks.availability.managerServiceBase = 'http://manager-b.local:18317';
    mocks.managementKey = 'management-key-b';
    await act(async () => {
      renderer.update(<UsageMaintenancePage />);
      await Promise.resolve();
      await Promise.resolve();
    });
    mocks.availability.managerServiceBase = 'http://manager-a.local:18317';
    mocks.managementKey = 'management-key-a';
    await act(async () => {
      renderer.update(<UsageMaintenancePage />);
      await Promise.resolve();
      await Promise.resolve();
    });
    const loadCount = mocks.getUsageMaintenance.mock.calls.length;
    await confirmation.onConfirm();

    expect(mocks.deleteUsageArchive).not.toHaveBeenCalled();
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(loadCount);
    expect(mocks.showNotification).not.toHaveBeenCalled();
    act(() => renderer.unmount());
  });

  it('polls active state without a loading flash, pauses while working, refreshes failures, and cleans up', async () => {
    vi.useFakeTimers();
    const active = archive('verifying');
    const renderer = await renderResolvedPage(maintenance({ active_run: active }), [active]);
    const pollMaintenance = deferred<UsageMaintenanceStatus>();
    const pollArchives = deferred<UsageArchiveList>();
    mocks.getUsageMaintenance.mockImplementationOnce(() => pollMaintenance.promise);
    mocks.listUsageArchives.mockImplementationOnce(() => pollArchives.promise);

    act(() => vi.advanceTimersByTime(5_000));
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(2);
    expect(mocks.probeUsageMaintenance).toHaveBeenCalledTimes(1);
    expect(getText(renderer.root)).not.toContain('full-screen-loading');
    const pollMaintenanceSignal = mocks.getUsageMaintenance.mock.calls[1][2] as AbortSignal;
    const pollArchivesSignal = mocks.listUsageArchives.mock.calls[1][3] as AbortSignal;
    act(() => renderer.unmount());
    expect(pollMaintenanceSignal.aborted).toBe(true);
    expect(pollArchivesSignal.aborted).toBe(true);
    act(() => vi.advanceTimersByTime(10_000));
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(2);

    vi.clearAllMocks();
    mocks.availability.managerServiceBase = 'http://manager-a.local:18317';
    mocks.getUsageMaintenance
      .mockResolvedValueOnce(maintenance({ active_run: active }))
      .mockResolvedValueOnce(
        maintenance({ active_run: { ...archive('failed'), resume_status: 'verifying' } })
      );
    mocks.listUsageArchives
      .mockResolvedValueOnce({ runs: [active] })
      .mockResolvedValueOnce({ runs: [{ ...archive('failed'), resume_status: 'verifying' }] });
    const resumeFailure = deferred<unknown>();
    mocks.resumeUsageArchive.mockImplementationOnce(() => resumeFailure.promise);
    const failedRenderer = await renderResolvedPage(maintenance({ active_run: active }), [active]);
    act(() => {
      findButtons(failedRenderer, 'Resume')[0].props.onClick();
    });
    act(() => vi.advanceTimersByTime(5_000));
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(1);
    await act(async () => {
      resumeFailure.reject(new Error('verification interrupted'));
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mocks.showNotification).toHaveBeenCalledWith('verification interrupted', 'error');
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(2);
    expect(getText(failedRenderer.root)).toContain('failed');
    act(() => failedRenderer.unmount());
  });

  it('polls while an explicit maintenance lock is present', async () => {
    vi.useFakeTimers();
    const locked = maintenance({
      active_lock: {
        run_id: 'locked-run',
        operation: 'archive',
        acquired_at_ms: 1_700_000_000_000,
        updated_at_ms: 1_700_000_001_000,
      },
    });
    const renderer = await renderResolvedPage(locked);
    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(2);
    act(() => renderer.unmount());
  });

  it('keeps polling a retention run while the worker is waiting to retry a failure', async () => {
    vi.useFakeTimers();
    const failedRetentionRun = {
      ...archive('failed', 'retention-retry-run'),
      mode: 'retention',
      resume_status: 'verifying',
    };
    const renderer = await renderResolvedPage(maintenance({ active_run: failedRetentionRun }), [
      failedRetentionRun,
    ]);

    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mocks.getUsageMaintenance).toHaveBeenCalledTimes(2);
    expect(mocks.listUsageArchives).toHaveBeenCalledTimes(2);
    act(() => renderer.unmount());
  });
});
