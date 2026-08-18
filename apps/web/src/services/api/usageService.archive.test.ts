import axios, { AxiosHeaders, type AxiosResponse } from 'axios';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/features/demo/demoMode', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/demo/demoMode')>()),
  isDemoMode: () => false,
}));

import { getUsageServiceErrorCode, usageServiceApi } from './usageService';

const head = vi.spyOn(axios, 'head');
const get = vi.spyOn(axios, 'get');
const post = vi.spyOn(axios, 'post');
const responseWithStatus = (status: number): AxiosResponse => ({
  data: undefined,
  status,
  statusText: '',
  headers: {},
  config: { headers: new AxiosHeaders() },
});

beforeEach(() => {
  head.mockReset();
  get.mockReset();
  post.mockReset();
});

describe('usage maintenance capability probe', () => {
  it('accepts the authenticated 204 capability response', async () => {
    const signal = new AbortController().signal;
    head.mockResolvedValue(responseWithStatus(204));

    await usageServiceApi.probeUsageMaintenance('http://manager.local:18317/', 'admin-key', signal);

    expect(head).toHaveBeenCalledWith(
      'http://manager.local:18317/v0/management/usage/maintenance',
      expect.objectContaining({
        headers: { Authorization: 'Bearer admin-key' },
        signal,
      })
    );
  });

  it('rejects a generic 200 response that does not prove capability support', async () => {
    head.mockResolvedValue(responseWithStatus(200));

    await expect(
      usageServiceApi.probeUsageMaintenance('http://manager.local:18317', 'admin-key')
    ).rejects.toMatchObject({
      status: 200,
      code: 'usage_archive_unavailable',
    });
  });

  it('uses the authenticated archive and maintenance endpoints with cancellation support', async () => {
    const signal = new AbortController().signal;
    post.mockResolvedValue(responseWithStatus(200));
    get.mockResolvedValue(responseWithStatus(200));

    await usageServiceApi.previewUsageArchive(
      'http://manager.local:18317/',
      1_700_000_000_000,
      'admin-key',
      signal
    );
    await usageServiceApi.createUsageArchive(
      'http://manager.local:18317/',
      1_700_000_000_000,
      'admin-key',
      signal
    );
    await usageServiceApi.listUsageArchives('http://manager.local:18317/', 'admin-key', 25, signal);
    await usageServiceApi.getUsageArchive(
      'http://manager.local:18317/',
      'run/id',
      'admin-key',
      signal
    );
    await usageServiceApi.getUsageMaintenance('http://manager.local:18317/', 'admin-key', signal);

    expect(post).toHaveBeenNthCalledWith(
      1,
      'http://manager.local:18317/v0/management/usage/archives/preview',
      { cutoff_timestamp_ms: 1_700_000_000_000 },
      expect.objectContaining({
        timeout: 0,
        headers: { Authorization: 'Bearer admin-key' },
        signal,
      })
    );
    expect(post).toHaveBeenNthCalledWith(
      2,
      'http://manager.local:18317/v0/management/usage/archives',
      { cutoff_timestamp_ms: 1_700_000_000_000 },
      expect.objectContaining({
        timeout: 0,
        headers: { Authorization: 'Bearer admin-key' },
        signal,
      })
    );
    expect(get).toHaveBeenNthCalledWith(
      1,
      'http://manager.local:18317/v0/management/usage/archives',
      expect.objectContaining({
        headers: { Authorization: 'Bearer admin-key' },
        params: { limit: 25 },
        signal,
      })
    );
    expect(get).toHaveBeenNthCalledWith(
      2,
      'http://manager.local:18317/v0/management/usage/archives/run%2Fid',
      expect.objectContaining({
        headers: { Authorization: 'Bearer admin-key' },
        signal,
      })
    );
    expect(get).toHaveBeenNthCalledWith(
      3,
      'http://manager.local:18317/v0/management/usage/maintenance',
      expect.objectContaining({
        headers: { Authorization: 'Bearer admin-key' },
        signal,
      })
    );
  });

  it('preserves stable archive error codes for UI handling', () => {
    for (const code of [
      'usage_archive_invalid_id',
      'usage_archive_invalid_request',
      'usage_archive_request_too_large',
      'usage_archive_no_events',
      'usage_archive_maintenance_locked',
      'usage_archive_invalid_state',
      'usage_archive_coverage_incomplete',
      'usage_archive_delete_unavailable',
      'usage_archive_not_found',
      'usage_archive_unavailable',
    ]) {
      expect(getUsageServiceErrorCode({ code })).toBe(code);
    }
  });

  it('keeps long-running archive actions unbounded while forwarding cancellation', async () => {
    const signal = new AbortController().signal;
    post.mockResolvedValue(responseWithStatus(200));

    await usageServiceApi.resumeUsageArchive(
      'http://manager.local:18317',
      'run/id',
      'admin-key',
      signal
    );
    await usageServiceApi.verifyUsageArchive(
      'http://manager.local:18317',
      'run/id',
      'admin-key',
      signal
    );
    await usageServiceApi.deleteUsageArchive(
      'http://manager.local:18317',
      'run/id',
      'admin-key',
      signal
    );

    for (const [index, action] of ['resume', 'verify', 'delete'].entries()) {
      expect(post).toHaveBeenNthCalledWith(
        index + 1,
        `http://manager.local:18317/v0/management/usage/archives/run%2Fid/${action}`,
        undefined,
        expect.objectContaining({
          timeout: 0,
          headers: { Authorization: 'Bearer admin-key' },
          signal,
        })
      );
    }
  });
});
