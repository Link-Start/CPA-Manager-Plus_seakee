import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { LoginPage } from './LoginPage';

const mocks = vi.hoisted(() => ({
  getInfo: vi.fn(),
  setup: vi.fn(),
  selectSlimCPA: vi.fn(),
  initializeAdminKey: vi.fn(),
  generateAdminKey: vi.fn(),
  restoreSession: vi.fn(),
  login: vi.fn(),
  setUsageServiceConfig: vi.fn(),
  showNotification: vi.fn(),
  setLanguage: vi.fn(),
  cycleTheme: vi.fn(),
  windowListeners: new Map<string, EventListener>(),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock('@/stores', () => ({
  useAuthStore: (
    selector: (state: {
      isAuthenticated: boolean;
      login: typeof mocks.login;
      restoreSession: typeof mocks.restoreSession;
      apiBase: string;
      managementKey: string;
      rememberPassword: boolean;
    }) => unknown
  ) =>
    selector({
      isAuthenticated: false,
      login: mocks.login,
      restoreSession: mocks.restoreSession,
      apiBase: '',
      managementKey: '',
      rememberPassword: false,
    }),
  useLanguageStore: (
    selector: (state: { language: string; setLanguage: typeof mocks.setLanguage }) => unknown
  ) => selector({ language: 'en', setLanguage: mocks.setLanguage }),
  useThemeStore: (
    selector: (state: { theme: string; cycleTheme: typeof mocks.cycleTheme }) => unknown
  ) => selector({ theme: 'light', cycleTheme: mocks.cycleTheme }),
  useUsageServiceStore: (
    selector: (state: { setUsageServiceConfig: typeof mocks.setUsageServiceConfig }) => unknown
  ) => selector({ setUsageServiceConfig: mocks.setUsageServiceConfig }),
  useNotificationStore: () => ({ showNotification: mocks.showNotification }),
}));

vi.mock('@/services/api/usageService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/api/usageService')>();
  return {
    ...actual,
    usageServiceApi: {
      ...actual.usageServiceApi,
      getInfo: mocks.getInfo,
      setup: mocks.setup,
      selectSlimCPA: mocks.selectSlimCPA,
      initializeAdminKey: mocks.initializeAdminKey,
      generateAdminKey: mocks.generateAdminKey,
    },
  };
});

const textContent = (node: ReactTestInstance): string =>
  node.children.map((child) => (typeof child === 'string' ? child : textContent(child))).join('');

const findButton = (root: ReactTestInstance, label: string) =>
  root.findAllByType('button').find((button) => textContent(button).includes(label));

function LocationProbe() {
  const location = useLocation();
  return <span data-location={`${location.pathname}${location.search}${location.hash}`} />;
}

const flush = async () => {
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
};

const createDeferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
};

describe('LoginPage Slim setup', () => {
  beforeAll(() => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  });

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.windowListeners.clear();
    mocks.restoreSession.mockResolvedValue(false);
    mocks.getInfo.mockResolvedValue({
      service: 'cpa-manager-plus',
      configured: false,
      adminReady: true,
      projectInitialized: false,
      setupRequired: true,
      deploymentMode: 'slim',
      bootstrapRequired: false,
      panelBasePath: '/panel',
    });
    mocks.selectSlimCPA.mockResolvedValue({
      ok: true,
      action: 'download_latest',
      nextStep: 'complete',
      installed: { version: 'v7.2.0' },
      deployment: { mode: 'integrated' },
    });
    mocks.setup.mockResolvedValue({ ok: true, nextStep: 'admin_key' });
    mocks.initializeAdminKey.mockResolvedValue({ ok: true });
    mocks.generateAdminKey.mockResolvedValue({ adminKey: 'cpamp_GeneratedAdmin!123456' });
    vi.stubGlobal('window', {
      location: {
        protocol: 'http:',
        hostname: 'manager.local',
        port: '18137',
        pathname: '/panel',
      },
      addEventListener: vi.fn((type: string, listener: EventListener) => {
        mocks.windowListeners.set(type, listener);
      }),
      removeEventListener: vi.fn((type: string, listener: EventListener) => {
        if (mocks.windowListeners.get(type) === listener) mocks.windowListeners.delete(type);
      }),
    });
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
    });
  });

  it('requires and submits the existing CPAMP Admin Key before choosing a CPA source', async () => {
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(
        <MemoryRouter initialEntries={['/panel']}>
          <LoginPage />
        </MemoryRouter>
      );
      await flush();
    });

    const adminLabel = renderer.root
      .findAllByType('label')
      .find((label) => textContent(label) === 'login.admin_key_label');
    expect(adminLabel).toBeDefined();
    const adminInput = renderer.root
      .findAllByType('input')
      .find((input) => input.props.id === adminLabel?.props.htmlFor);
    expect(adminInput).toBeDefined();

    const downloadButton = findButton(renderer.root, 'login.slim_download_latest');
    expect(downloadButton).toBeDefined();
    await act(async () => {
      downloadButton?.props.onClick();
      await Promise.resolve();
    });
    expect(mocks.selectSlimCPA).not.toHaveBeenCalled();
    expect(textContent(renderer.root)).toContain('login.admin_key_required');

    await act(async () => {
      adminInput?.props.onChange({ target: { value: 'cpamp_ExistingAdmin!123456' } });
    });
    await act(async () => {
      downloadButton?.props.onClick();
      await flush();
    });
    expect(mocks.selectSlimCPA).toHaveBeenCalledWith(
      'http://manager.local:18137',
      'download_latest',
      { adminKey: 'cpamp_ExistingAdmin!123456' }
    );
    renderer.unmount();
  });

  it('blocks duplicate Slim provisioning before the loading state rerenders', async () => {
    const pending = createDeferred<{
      ok: boolean;
      action: string;
      nextStep: string;
      installed: { version: string };
      deployment: { mode: string };
    }>();
    mocks.selectSlimCPA.mockReturnValue(pending.promise);
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(
        <MemoryRouter initialEntries={['/panel']}>
          <LoginPage />
        </MemoryRouter>
      );
      await flush();
    });

    const adminLabel = renderer.root
      .findAllByType('label')
      .find((label) => textContent(label) === 'login.admin_key_label');
    const adminInput = renderer.root
      .findAllByType('input')
      .find((input) => input.props.id === adminLabel?.props.htmlFor);
    await act(async () => {
      adminInput?.props.onChange({ target: { value: 'cpamp_ExistingAdmin!123456' } });
    });
    const downloadButton = findButton(renderer.root, 'login.slim_download_latest');
    await act(async () => {
      downloadButton?.props.onClick();
      downloadButton?.props.onClick();
      await Promise.resolve();
    });

    expect(mocks.selectSlimCPA).toHaveBeenCalledTimes(1);
    await act(async () => {
      pending.resolve({
        ok: true,
        action: 'download_latest',
        nextStep: 'complete',
        installed: { version: 'v7.2.0' },
        deployment: { mode: 'integrated' },
      });
      await flush();
    });
    renderer.unmount();
  });

  it('keeps the existing-CPA connection step during an admin-ready focus refresh', async () => {
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(
        <MemoryRouter initialEntries={['/panel']}>
          <LoginPage />
        </MemoryRouter>
      );
      await flush();
    });

    const adminLabel = renderer.root
      .findAllByType('label')
      .find((label) => textContent(label) === 'login.admin_key_label');
    const adminInput = renderer.root
      .findAllByType('input')
      .find((input) => input.props.id === adminLabel?.props.htmlFor);
    await act(async () => {
      adminInput?.props.onChange({ target: { value: 'cpamp_ExistingAdmin!123456' } });
    });
    await act(async () => {
      findButton(renderer.root, 'login.slim_use_existing')?.props.onClick();
      await flush();
    });
    expect(textContent(renderer.root)).toContain('login.cpa_connection_label');

    await act(async () => {
      mocks.windowListeners.get('focus')?.({ type: 'focus' } as Event);
      await flush();
    });

    expect(textContent(renderer.root)).toContain('login.cpa_connection_label');
    expect(textContent(renderer.root)).not.toContain('login.slim_choice_title');
    renderer.unmount();
  });

  it('does not start a second setup-state refresh while initial detection is pending', async () => {
    const pendingInfo = createDeferred<{
      service: string;
      configured: boolean;
      adminReady: boolean;
      projectInitialized: boolean;
      setupRequired: boolean;
      deploymentMode: string;
      bootstrapRequired: boolean;
    }>();
    mocks.getInfo.mockReturnValue(pendingInfo.promise);
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(
        <MemoryRouter initialEntries={['/panel']}>
          <LoginPage />
        </MemoryRouter>
      );
      await Promise.resolve();
    });
    expect(mocks.getInfo).toHaveBeenCalledTimes(1);

    await act(async () => {
      mocks.windowListeners.get('focus')?.({ type: 'focus' } as Event);
      await Promise.resolve();
    });
    expect(mocks.getInfo).toHaveBeenCalledTimes(1);

    await act(async () => {
      pendingInfo.resolve({
        service: 'cpa-manager-plus',
        configured: false,
        adminReady: true,
        projectInitialized: false,
        setupRequired: true,
        deploymentMode: 'slim',
        bootstrapRequired: false,
      });
      await flush();
    });
    renderer.unmount();
  });

  it('does not publish an async setup error after the page unmounts', async () => {
    const pending = createDeferred<never>();
    mocks.selectSlimCPA.mockReturnValue(pending.promise);
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(
        <MemoryRouter initialEntries={['/panel']}>
          <LoginPage />
        </MemoryRouter>
      );
      await flush();
    });

    const adminLabel = renderer.root
      .findAllByType('label')
      .find((label) => textContent(label) === 'login.admin_key_label');
    const adminInput = renderer.root
      .findAllByType('input')
      .find((input) => input.props.id === adminLabel?.props.htmlFor);
    await act(async () => {
      adminInput?.props.onChange({ target: { value: 'cpamp_ExistingAdmin!123456' } });
    });
    await act(async () => {
      findButton(renderer.root, 'login.slim_download_latest')?.props.onClick();
      await Promise.resolve();
      renderer.unmount();
    });
    await act(async () => {
      pending.reject(new Error('late provisioning failure'));
      await flush();
    });

    expect(mocks.showNotification).not.toHaveBeenCalled();
  });

  it('refreshes setup state when another browser tab completes initialization', async () => {
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(
        <MemoryRouter initialEntries={['/panel']}>
          <LoginPage />
        </MemoryRouter>
      );
      await flush();
    });
    expect(textContent(renderer.root)).toContain('login.slim_choice_title');

    mocks.getInfo.mockResolvedValue({
      service: 'cpa-manager-plus',
      configured: true,
      adminReady: true,
      projectInitialized: true,
      setupRequired: false,
      deploymentMode: 'integrated',
      bootstrapRequired: false,
      panelBasePath: '/panel',
    });
    await act(async () => {
      mocks.windowListeners.get('focus')?.({ type: 'focus' } as Event);
      await flush();
    });

    expect(mocks.getInfo).toHaveBeenCalledTimes(2);
    expect(textContent(renderer.root)).not.toContain('login.slim_choice_title');
    expect(textContent(renderer.root)).toContain('login.docker_login_subtitle');
    renderer.unmount();
  });

  it('opens setup troubleshooting in the current interface language', async () => {
    mocks.selectSlimCPA.mockRejectedValue(
      Object.assign(new Error('CPA provisioning failed'), {
        code: 'slim_cpa_provision_failed',
        docsUrl:
          'https://seakee.github.io/CPA-Manager-Plus/docs/troubleshooting/setup.html#slim-cpa-provision-failed',
      })
    );
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(
        <MemoryRouter initialEntries={['/panel']}>
          <LoginPage />
        </MemoryRouter>
      );
      await flush();
    });

    const adminLabel = renderer.root
      .findAllByType('label')
      .find((label) => textContent(label) === 'login.admin_key_label');
    const adminInput = renderer.root
      .findAllByType('input')
      .find((input) => input.props.id === adminLabel?.props.htmlFor);
    await act(async () => {
      adminInput?.props.onChange({ target: { value: 'cpamp_ExistingAdmin!123456' } });
    });
    await act(async () => {
      findButton(renderer.root, 'login.slim_download_latest')?.props.onClick();
      await flush();
    });

    const docsLink = renderer.root
      .findAllByType('a')
      .find((link) => textContent(link).includes('login.open_solution_docs'));
    expect(docsLink?.props.href).toBe(
      'https://seakee.github.io/CPA-Manager-Plus/docs/en/troubleshooting/setup.html#slim-cpa-provision-failed'
    );
    renderer.unmount();
  });

  it('removes only the bootstrap token from the visible URL', async () => {
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(
        <MemoryRouter initialEntries={['/panel?bootstrap=secret-token&source=installer#setup']}>
          <LoginPage />
          <LocationProbe />
        </MemoryRouter>
      );
      await flush();
    });

    const locationProbe = renderer.root.find(
      (node) => node.type === 'span' && typeof node.props['data-location'] === 'string'
    );
    expect(locationProbe.props['data-location']).toBe('/panel?source=installer#setup');
    renderer.unmount();
  });

  it('counts Unicode code points consistently with the backend admin-key policy', async () => {
    mocks.getInfo.mockResolvedValue({
      service: 'cpa-manager-plus',
      configured: true,
      adminReady: false,
      projectInitialized: true,
      setupRequired: true,
      deploymentMode: 'integrated',
      bootstrapRequired: true,
    });
    let renderer!: ReactTestRenderer;
    await act(async () => {
      renderer = create(
        <MemoryRouter initialEntries={['/panel?bootstrap=bootstrap-token']}>
          <LoginPage />
        </MemoryRouter>
      );
      await flush();
    });

    const adminLabel = renderer.root
      .findAllByType('label')
      .find((label) => textContent(label) === 'login.admin_key_label');
    const adminInput = renderer.root
      .findAllByType('input')
      .find((input) => input.props.id === adminLabel?.props.htmlFor);
    await act(async () => {
      adminInput?.props.onChange({ target: { value: 'Aa1😀bbbbbbbbbbb' } });
      findButton(renderer.root, 'common.next')?.props.onClick();
      await flush();
    });

    expect(mocks.initializeAdminKey).not.toHaveBeenCalled();
    expect(textContent(renderer.root)).toContain('login.admin_key_policy');
    renderer.unmount();
  });
});
