import { beforeEach, describe, expect, it, vi } from 'vitest';

const { axiosMocks } = vi.hoisted(() => ({
  axiosMocks: {
    get: vi.fn(),
    post: vi.fn(),
  },
}));

vi.mock('axios', async (importOriginal) => {
  const actual = await importOriginal<typeof import('axios')>();
  const mockedAxios = Object.assign(actual.default, {
    get: axiosMocks.get,
    post: axiosMocks.post,
  });
  return { ...actual, default: mockedAxios };
});

import {
  getUsageServiceErrorCode,
  usageServiceApi,
  type UsageServiceApiError,
} from './usageService';

beforeEach(() => {
  axiosMocks.get.mockReset();
  axiosMocks.post.mockReset();
});

describe('usageServiceApi setup authentication', () => {
  it('uses the one-time bootstrap header while validating the CPA connection', async () => {
    axiosMocks.post.mockResolvedValue({
      data: { ok: true, upstream: 'http://127.0.0.1:8317', nextStep: 'admin_key' },
    });

    await usageServiceApi.setup(
      'http://127.0.0.1:18318',
      {
        cpaBaseUrl: 'http://127.0.0.1:8317',
        cpaManagementKey: 'cpa-key',
      },
      { bootstrapToken: 'bootstrap-token' }
    );

    expect(axiosMocks.post).toHaveBeenCalledWith(
      'http://127.0.0.1:18318/setup',
      expect.objectContaining({ cpaManagementKey: 'cpa-key' }),
      expect.objectContaining({
        headers: { 'X-CPAMP-Bootstrap-Token': 'bootstrap-token' },
      })
    );
  });

  it('uses the existing admin key for migration setup and admin-key generation', async () => {
    axiosMocks.post.mockResolvedValueOnce({
      data: { ok: true, upstream: 'http://cpa:8317', nextStep: 'complete' },
    });
    axiosMocks.post.mockResolvedValueOnce({ data: { adminKey: 'cpamp_generated-key' } });

    await usageServiceApi.setup(
      'http://manager:18318',
      { cpaBaseUrl: 'http://cpa:8317', cpaManagementKey: 'cpa-key' },
      { adminKey: 'cpamp-admin-key' }
    );
    await usageServiceApi.generateAdminKey('http://manager:18318', {
      adminKey: 'cpamp-admin-key',
    });

    expect(axiosMocks.post).toHaveBeenNthCalledWith(
      1,
      'http://manager:18318/setup',
      expect.any(Object),
      expect.objectContaining({ headers: { Authorization: 'Bearer cpamp-admin-key' } })
    );
    expect(axiosMocks.post).toHaveBeenNthCalledWith(
      2,
      'http://manager:18318/setup/admin-key/generate',
      undefined,
      expect.objectContaining({ headers: { Authorization: 'Bearer cpamp-admin-key' } })
    );
  });

  it('submits the explicit Slim CPA source choice with bootstrap authorization', async () => {
    axiosMocks.post.mockResolvedValue({
      data: { ok: true, action: 'use_existing', nextStep: 'cpa_connection' },
    });

    await usageServiceApi.selectSlimCPA('http://manager:18318', 'use_existing', {
      bootstrapToken: 'bootstrap-token',
    });

    expect(axiosMocks.post).toHaveBeenCalledWith(
      'http://manager:18318/setup/cpa-source',
      { action: 'use_existing' },
      expect.objectContaining({
        headers: { 'X-CPAMP-Bootstrap-Token': 'bootstrap-token' },
        timeout: 10 * 60 * 1000,
      })
    );
  });

  it('preserves the structured error code and documentation URL', async () => {
    axiosMocks.post.mockRejectedValue({
      isAxiosError: true,
      message: 'Request failed with status code 502',
      response: {
        status: 502,
        data: {
          code: 'setup_cpa_unreachable',
          message: 'CPA management API validation failed',
          docsUrl: 'https://docs.example/setup#cpa-management-api-unreachable',
        },
      },
    });

    let captured: UsageServiceApiError | undefined;
    try {
      await usageServiceApi.setup(
        'http://manager:18318',
        { cpaBaseUrl: 'http://cpa:8317', cpaManagementKey: 'wrong' },
        { bootstrapToken: 'bootstrap-token' }
      );
    } catch (error) {
      captured = error as UsageServiceApiError;
    }

    expect(captured?.code).toBe('setup_cpa_unreachable');
    expect(captured?.docsUrl).toBe('https://docs.example/setup#cpa-management-api-unreachable');
    expect(getUsageServiceErrorCode(captured)).toBe('setup_cpa_unreachable');
  });

  it('preserves the bounded admin verification error instead of falling back to a generic request failure', () => {
    expect(
      getUsageServiceErrorCode({
        code: 'admin_verification_busy',
        docsUrl:
          'https://seakee.github.io/CPA-Manager-Plus/docs/troubleshooting/setup.html#admin-verification-busy',
      })
    ).toBe('admin_verification_busy');
  });

  it('preserves the Slim transition recovery error for localized setup guidance', () => {
    expect(
      getUsageServiceErrorCode({
        code: 'slim_transition_recovery_pending',
        docsUrl:
          'https://seakee.github.io/CPA-Manager-Plus/docs/troubleshooting/setup.html#slim-transition-recovery-pending',
      })
    ).toBe('slim_transition_recovery_pending');
  });
});
