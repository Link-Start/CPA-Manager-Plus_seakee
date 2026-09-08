import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import type { ConnectionStatus } from '@/types';
import { VersionCard } from './VersionCard';
import styles from './VersionCard.module.scss';

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const { mocks } = vi.hoisted(() => ({
  mocks: {
    checkManagerLatest: vi.fn(),
    checkLatest: vi.fn(),
    showNotification: vi.fn(),
    updates: {
      status: {} as Record<string, unknown>,
      check: vi.fn(),
      available: true,
      busy: false,
      error: false,
    },
  },
}));

vi.mock('@/features/system/ManagerUpdates', () => ({
  useManagerUpdates: () => mocks.updates,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) =>
      options?.version ? `${key}:${String(options.version)}` : key,
    i18n: { language: 'en-US' },
  }),
}));

vi.mock('@/stores', () => ({
  useNotificationStore: (
    selector: (state: { showNotification: typeof mocks.showNotification }) => unknown
  ) => selector({ showNotification: mocks.showNotification }),
}));

vi.mock('@/services/api', () => ({
  versionApi: {
    checkManagerLatest: mocks.checkManagerLatest,
    checkLatest: mocks.checkLatest,
  },
}));

let renderer: ReactTestRenderer | null = null;

const getText = (node: ReactTestInstance): string =>
  node.children.map((child) => (typeof child === 'string' ? child : getText(child))).join('');

const findAnchor = (renderer: ReactTestRenderer, className: string, text: string) =>
  renderer.root.find(
    (node) =>
      node.type === 'a' && node.props.className?.includes(className) && getText(node).includes(text)
  );

const findBadge = (renderer: ReactTestRenderer, type: string, text: string) =>
  renderer.root.find(
    (node) =>
      node.type === type &&
      node.props.className?.includes(styles.badge) &&
      node.props.className?.includes(styles.badgeUpdate) &&
      getText(node).includes(text)
  );

const renderCard = async ({
  appVersion = '1.12.6',
  apiVersion = '7.2.143',
  latestApp = '1.12.6',
  latestApi = '7.2.143',
  connectionStatus = 'connected' as ConnectionStatus,
  statusOverrides = {},
  error = false,
}: {
  appVersion?: string;
  apiVersion?: string;
  latestApp?: string;
  latestApi?: string;
  connectionStatus?: ConnectionStatus;
  statusOverrides?: Record<string, unknown>;
  error?: boolean;
} = {}) => {
  mocks.checkManagerLatest.mockResolvedValue({ tag_name: latestApp });
  mocks.updates.status = {
    current_version: appVersion,
    state: latestApp.includes('gabcdef')
      ? 'never_checked'
      : latestApp === appVersion
        ? 'up_to_date'
        : 'update_available',
    target: { release: { version: latestApp } },
    ...statusOverrides,
  };
  mocks.updates.error = error;
  mocks.checkLatest.mockResolvedValue({ 'latest-version': latestApi });

  await act(async () => {
    renderer = create(
      <MemoryRouter>
        <VersionCard
          appVersion={appVersion}
          apiVersion={apiVersion}
          cpaBase="http://cpa.local:8317"
          connectionStatus={connectionStatus}
          usageEnabled={false}
          usageLoading={false}
          collectorStatus={null}
          collectorLoading={false}
          errorLogCount={0}
          errorLogsLoading={false}
        />
      </MemoryRouter>
    );
    await Promise.resolve();
    await Promise.resolve();
  });

  if (!renderer) throw new Error('VersionCard did not render');
  return renderer;
};

afterEach(() => {
  if (renderer) {
    act(() => renderer?.unmount());
    renderer = null;
  }
  mocks.checkManagerLatest.mockReset();
  mocks.checkLatest.mockReset();
  mocks.showNotification.mockReset();
});

describe('VersionCard release links', () => {
  it('keeps current Manager and Core versions linked to their installed releases', async () => {
    const renderer = await renderCard();

    expect(findAnchor(renderer, styles.versionLink, '1.12.6').props.href).toBe(
      'https://github.com/seakee/CPA-Manager-Plus/releases/tag/v1.12.6'
    );
    expect(findAnchor(renderer, styles.versionLink, '7.2.143').props.href).toBe(
      'https://github.com/router-for-me/CLIProxyAPI/releases/tag/v7.2.143'
    );
    expect(mocks.checkManagerLatest).not.toHaveBeenCalled();
    expect(mocks.checkLatest).toHaveBeenCalledTimes(1);
  });

  it('links a Core update badge to the detected latest Core release', async () => {
    const renderer = await renderCard({ latestApi: 'v7.2.146' });
    const badge = findBadge(renderer, 'a', 'v7.2.146');

    expect(badge.props.href).toBe(
      'https://github.com/router-for-me/CLIProxyAPI/releases/tag/v7.2.146'
    );
    expect(badge.props.target).toBe('_blank');
    expect(badge.props.rel).toBe('noopener noreferrer');
  });

  it('opens the internal update page from a compact Manager badge and preserves the current release link', async () => {
    const renderer = await renderCard({ latestApp: 'v1.12.7' });
    const badge = findAnchor(
      renderer,
      styles.managerUpdateBadge,
      'manager_updates.available_badge'
    );

    expect(badge.props.href).toBe('/system/updates');
    expect(badge.props.target).toBeUndefined();
    expect(badge.props.title).toBe('manager_updates.view_version:v1.12.7');
    expect(getText(badge)).not.toContain('v1.12.7');
    expect(findAnchor(renderer, styles.versionLink, '1.12.6').props.href).toContain('/tag/v1.12.6');
    expect(renderer.root.findAllByType('select')).toHaveLength(0);
    expect(renderer.root.findAllByType('code')).toHaveLength(0);
  });

  it('does not create a badge link for an invalid latest version', async () => {
    const renderer = await renderCard({ latestApp: 'v1.12.7-5-gabcdef', latestApi: '' });

    expect(
      renderer.root.findAll(
        (node) => node.type === 'a' && node.props.className?.includes(styles.badge)
      )
    ).toHaveLength(0);
    expect(findAnchor(renderer, styles.versionLink, '1.12.6').props.href).toContain(
      '/CPA-Manager-Plus/releases/tag/v1.12.6'
    );
  });

  it('keeps the Manager overview quiet when there is no update', async () => {
    const renderer = await renderCard();

    expect(
      renderer.root.findAll(
        (node) =>
          node.type === 'span' &&
          node.props.className?.includes(styles.badgeLatest) &&
          getText(node) === 'dashboard.version_is_latest'
      )
    ).toHaveLength(1);
  });

  it.each([{ stale: true }, { last_error: 'offline' }])(
    'hides an untrusted Manager update badge: %j',
    async (statusOverrides) => {
      const renderer = await renderCard({ latestApp: 'v1.12.7', statusOverrides });
      expect(
        renderer.root.findAll((node) => node.type === 'a' && node.props.href === '/system/updates')
      ).toHaveLength(0);
    }
  );

  it('hides the Manager update badge after a failed request', async () => {
    const renderer = await renderCard({ latestApp: 'v1.12.7', error: true });
    expect(
      renderer.root.findAll((node) => node.type === 'a' && node.props.href === '/system/updates')
    ).toHaveLength(0);
  });

  it('shows the running Manager version when it differs from the panel', async () => {
    const renderer = await renderCard({ statusOverrides: { current_version: 'v1.12.5' } });
    expect(findAnchor(renderer, styles.versionLink, 'v1.12.5').props.href).toContain(
      '/tag/v1.12.5'
    );
    expect(getText(renderer.root)).not.toContain('manager_updates.panel_version');
  });
});
