import { beforeEach, describe, expect, it, vi } from 'vitest';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    request: vi.fn(),
    fetchQuota: vi.fn(),
  },
}));

vi.mock('./apiCall', () => ({
  apiCallApi: {
    request: mocks.request,
  },
  getApiCallErrorMessage: (result: { statusCode: number; bodyText?: string }) =>
    `${result.statusCode} ${result.bodyText ?? ''}`.trim(),
}));

vi.mock('@/utils/quota/providerRequests', () => ({
  fetchCodexQuota: mocks.fetchQuota,
}));

import { CODEX_RATE_LIMIT_RESET_CREDITS_CONSUME_URL } from '@/utils/quota/constants';
import {
  buildCodexUsageRequestHeaders,
  consumeCodexRateLimitResetCredit,
  resetCodexQuota,
} from './codexQuota';

beforeEach(() => {
  mocks.request.mockReset();
  mocks.fetchQuota.mockReset();
});

describe('buildCodexUsageRequestHeaders', () => {
  it('does not include Chatgpt-Account-Id when account id is missing', () => {
    const headers = buildCodexUsageRequestHeaders(null);

    expect(headers).not.toHaveProperty('Chatgpt-Account-Id');
    expect(headers.Authorization).toBe('Bearer $TOKEN$');
  });

  it('includes trimmed account id when available', () => {
    const headers = buildCodexUsageRequestHeaders(' account-123 ');

    expect(headers['Chatgpt-Account-Id']).toBe('account-123');
  });

  it('allows Codex inspection to override User-Agent', () => {
    const headers = buildCodexUsageRequestHeaders('account-123', {
      userAgent: 'codex-test-agent',
    });

    expect(headers['User-Agent']).toBe('codex-test-agent');
  });
});

describe('consumeCodexRateLimitResetCredit', () => {
  it('posts a redeem request through api-call with the Codex auth index', async () => {
    mocks.request.mockResolvedValue({
      statusCode: 200,
      hasStatusCode: true,
      header: {},
      bodyText: '{}',
      body: {},
    });

    await consumeCodexRateLimitResetCredit({
      name: 'codex-auth.json',
      type: 'codex',
      authIndex: ' auth-1 ',
      id_token: { account_id: 'acct-1' },
    });

    expect(mocks.request).toHaveBeenCalledTimes(1);
    const payload = mocks.request.mock.calls[0][0];
    expect(payload).toMatchObject({
      authIndex: 'auth-1',
      method: 'POST',
      url: CODEX_RATE_LIMIT_RESET_CREDITS_CONSUME_URL,
      header: expect.objectContaining({
        Authorization: 'Bearer $TOKEN$',
        'Chatgpt-Account-Id': 'acct-1',
      }),
    });
    expect(JSON.parse(payload.data)).toEqual({
      redeem_request_id: expect.any(String),
    });
  });

  it('posts the redeem request through the captured CPA scope', async () => {
    mocks.request.mockResolvedValue({
      statusCode: 200,
      hasStatusCode: true,
      header: {},
      bodyText: '{}',
      body: {},
    });

    await consumeCodexRateLimitResetCredit(
      { name: 'codex-auth.json', type: 'codex', authIndex: 'auth-1' },
      undefined,
      {
        apiBase: 'https://captured-cpa.example.test',
        managementKey: 'captured-key',
      }
    );

    expect(mocks.request.mock.calls[0]?.[1]).toMatchObject({
      baseURL: 'https://captured-cpa.example.test/v0/management',
      headers: { Authorization: 'Bearer captured-key' },
      cpampScopedRequest: true,
    });
  });
});

describe('resetCodexQuota', () => {
  const file = {
    name: 'codex-auth.json',
    type: 'codex',
    authIndex: 'auth-1',
  };
  const t = ((key: string) => key) as never;
  const quota = {
    planType: 'plus',
    windows: [],
    quotaInventoryObserved: true,
    rateLimitResetCreditsAvailableCount: 0,
    rateLimitResetCredits: [],
    rateLimitResetCreditsError: null,
  };

  it('fences the post-consume refresh behind the successful consume', async () => {
    const events: string[] = [];
    mocks.request.mockResolvedValueOnce({
      statusCode: 200,
      hasStatusCode: true,
      header: {},
      bodyText: '{}',
      body: {},
    });
    mocks.fetchQuota.mockImplementation(async () => {
      events.push('refresh');
      return quota;
    });

    const result = await resetCodexQuota(file, t, undefined, () => events.push('consumed'));

    expect(result).toEqual({ outcome: 'consumed_and_refreshed', quota });
    expect(events).toEqual(['consumed', 'refresh']);
    expect(mocks.request).toHaveBeenCalledTimes(1);
    expect(mocks.fetchQuota).toHaveBeenCalledTimes(1);
  });

  it('returns partial success when consume succeeds but the post-consume refresh fails', async () => {
    const onConsumed = vi.fn();
    mocks.request.mockResolvedValueOnce({
      statusCode: 200,
      hasStatusCode: true,
      header: {},
      bodyText: '{}',
      body: {},
    });
    mocks.fetchQuota.mockRejectedValueOnce(new Error('refresh unavailable'));

    const result = await resetCodexQuota(file, t, undefined, onConsumed);

    expect(result.outcome).toBe('consumed_refresh_failed');
    expect(result).toMatchObject({ refreshError: new Error('refresh unavailable') });
    expect(onConsumed).toHaveBeenCalledTimes(1);
    expect(mocks.fetchQuota).toHaveBeenCalledTimes(1);
  });

  it('keeps consume failures as failures and does not run the refresh', async () => {
    const onConsumed = vi.fn();
    mocks.request.mockResolvedValueOnce({
      statusCode: 409,
      hasStatusCode: true,
      header: {},
      bodyText: 'no credits',
      body: {},
    });

    await expect(resetCodexQuota(file, t, undefined, onConsumed)).rejects.toThrow('409');
    expect(onConsumed).not.toHaveBeenCalled();
    expect(mocks.fetchQuota).not.toHaveBeenCalled();
  });

  it('does not report a consume failure when the onConsumed callback throws and the refresh succeeds', async () => {
    mocks.request.mockResolvedValueOnce({
      statusCode: 200,
      hasStatusCode: true,
      header: {},
      bodyText: '{}',
      body: {},
    });
    mocks.fetchQuota.mockResolvedValue(quota);
    const onConsumed = vi.fn(() => {
      throw new Error('callback exploded');
    });

    const result = await resetCodexQuota(file, t, undefined, onConsumed);

    expect(result).toEqual({ outcome: 'consumed_and_refreshed', quota });
    expect(onConsumed).toHaveBeenCalledTimes(1);
    expect(mocks.fetchQuota).toHaveBeenCalledTimes(1);
  });

  it('keeps partial-success semantics when the onConsumed callback throws and the refresh fails', async () => {
    mocks.request.mockResolvedValueOnce({
      statusCode: 200,
      hasStatusCode: true,
      header: {},
      bodyText: '{}',
      body: {},
    });
    mocks.fetchQuota.mockRejectedValueOnce(new Error('refresh unavailable'));
    const onConsumed = vi.fn(() => {
      throw new Error('callback exploded');
    });

    const result = await resetCodexQuota(file, t, undefined, onConsumed);

    expect(result.outcome).toBe('consumed_refresh_failed');
    expect(result).toMatchObject({ refreshError: new Error('refresh unavailable') });
    expect(onConsumed).toHaveBeenCalledTimes(1);
    expect(mocks.fetchQuota).toHaveBeenCalledTimes(1);
  });
});
