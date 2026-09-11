import { describe, expect, it } from 'vitest';
import type { AuthFileItem, CodexQuotaState, XaiBillingSummary } from '@/types';
import { getAuthFileSelectionKey } from '@/features/authFiles/model/credentialStatus';
import { CODEX_SPARK_MODEL_ID } from '@/utils/quota/codexQuota';
import { buildQuotaCredentialIdentity } from '@/utils/quota/credentialScope';
import { resolveAccountQuota, type AccountQuotaStores } from './accountQuotaSummary';

const emptyStores = (): AccountQuotaStores => ({
  antigravityQuota: {},
  claudeQuota: {},
  codexQuota: {},
  kimiQuota: {},
  xaiQuota: {},
});

const makeXaiBilling = (overrides: Partial<XaiBillingSummary> = {}): XaiBillingSummary => ({
  periodType: 'weekly',
  usagePercent: null,
  productUsage: [],
  monthlyLimitCents: null,
  usedCents: null,
  includedUsedCents: null,
  onDemandCapCents: null,
  onDemandUsedCents: null,
  onDemandUsedPercent: null,
  usedPercent: null,
  ...overrides,
});

describe('resolveAccountQuota', () => {
  it.each([
    {
      label: 'weekly current-period data',
      billing: makeXaiBilling({
        usagePercent: 42,
        periodStart: '2026-09-05T00:00:00Z',
        periodEnd: '2026-09-12T00:00:00Z',
      }),
    },
    {
      label: 'legacy monthly data without a positive limit',
      billing: makeXaiBilling({
        periodType: 'monthly',
        usedPercent: 20,
        usedCents: 2_000,
        includedUsedCents: 2_000,
        billingPeriodEnd: '2026-10-01T00:00:00Z',
      }),
    },
  ])('does not expose $label as xAI account quota', ({ billing }) => {
    const file = { name: 'xai.json', type: 'xai' } as AuthFileItem;
    const stores = emptyStores();
    stores.xaiQuota[file.name] = {
      ...buildQuotaCredentialIdentity(file),
      status: 'success',
      billing,
    };

    expect(resolveAccountQuota(file, stores)).toMatchObject({
      status: 'unknown',
      remainingPercent: null,
      usedPercent: null,
      resetLabel: '-',
      resetAtMs: null,
    });
  });

  it('does not expose billing quota for an explicitly Free xAI plan', () => {
    const file = { name: 'xai-free.json', type: 'xai', planType: 'free' } as AuthFileItem;
    const stores = emptyStores();
    stores.xaiQuota[file.name] = {
      ...buildQuotaCredentialIdentity(file),
      status: 'success',
      billing: makeXaiBilling({
        periodType: 'monthly',
        monthlyLimitCents: 10_000,
        usedCents: 2_000,
        includedUsedCents: 2_000,
        usedPercent: 20,
        billingPeriodEnd: '2026-10-01T00:00:00Z',
      }),
    };

    expect(resolveAccountQuota(file, stores)).toMatchObject({
      status: 'unknown',
      remainingPercent: null,
      usedPercent: null,
    });
  });

  it('keeps confirmed paid xAI billing quota available', () => {
    const file = { name: 'xai-paid.json', type: 'xai' } as AuthFileItem;
    const stores = emptyStores();
    stores.xaiQuota[file.name] = {
      ...buildQuotaCredentialIdentity(file),
      status: 'success',
      billing: makeXaiBilling({
        periodType: 'monthly',
        monthlyLimitCents: 10_000,
        usedCents: 2_000,
        includedUsedCents: 2_000,
        usedPercent: 20,
        billingPeriodEnd: '2026-10-01T00:00:00Z',
      }),
    };

    expect(resolveAccountQuota(file, stores)).toMatchObject({
      status: 'ok',
      remainingPercent: 80,
      usedPercent: 20,
      resetLabel: '2026-10-01T00:00:00Z',
    });
  });

  it('keeps the account summary on Codex Main when Spark is more constrained', () => {
    const file = {
      name: 'codex.json',
      type: 'codex',
      authIndex: 'auth-1',
    } as AuthFileItem;
    const quota: CodexQuotaState = {
      status: 'success',
      windows: [
        {
          id: 'weekly',
          label: 'Weekly',
          usedPercent: 36,
          resetLabel: 'main-reset',
          modelScope: { kind: 'family', key: 'codex_main', complete: true },
        },
        {
          id: 'spark-weekly-0',
          label: 'Spark Weekly',
          usedPercent: 95,
          resetLabel: 'spark-reset',
          modelScope: {
            kind: 'models',
            models: [CODEX_SPARK_MODEL_ID],
            complete: true,
          },
        },
      ],
    };

    const summary = resolveAccountQuota(file, emptyStores(), {
      codexQuotaBySelectionKey: new Map([[getAuthFileSelectionKey(file), quota]]),
    });

    expect(summary).toMatchObject({
      usedPercent: 36,
      remainingPercent: 64,
      resetLabel: 'main-reset',
    });
  });

  it('does not treat a scoped Header observation as fresh account-wide quota evidence', () => {
    const file = {
      name: 'codex.json',
      type: 'codex',
      authIndex: 'auth-1',
    } as AuthFileItem;
    const quota: CodexQuotaState = {
      status: 'success',
      fetchedAtMs: 1_000,
      observedAtMs: 2_000,
      observedFromUsageHeaders: true,
      observedModelScope: {
        kind: 'models',
        models: [CODEX_SPARK_MODEL_ID],
        complete: true,
      },
      observedTraceId: 'spark-trace',
      activeLimit: 'main',
      windows: [
        {
          id: 'weekly',
          label: 'Weekly',
          usedPercent: 36,
          resetLabel: 'main-reset',
          modelScope: { kind: 'family', key: 'codex_main', complete: true },
        },
        {
          id: 'spark-weekly-0',
          label: 'Spark Weekly',
          usedPercent: 0,
          resetLabel: 'spark-reset',
          modelScope: {
            kind: 'models',
            models: [CODEX_SPARK_MODEL_ID],
            complete: true,
          },
        },
      ],
    };

    const summary = resolveAccountQuota(file, emptyStores(), {
      codexQuotaBySelectionKey: new Map([[getAuthFileSelectionKey(file), quota]]),
    });

    expect(summary).toMatchObject({
      source: 'cache',
      fetchedAtMs: 1_000,
      observedAtMs: 2_000,
      observedTraceId: 'spark-trace',
      activeLimit: 'main',
      usedPercent: 36,
      remainingPercent: 64,
    });
    expect(summary.observedQuotaAtMs).toBeUndefined();
  });

  it('uses Antigravity tier metadata when the stored plan is unknown', () => {
    const file = {
      name: 'antigravity.json',
      type: 'antigravity',
      authIndex: 'auth-1',
      planType: 'unknown',
    } as AuthFileItem;
    const stores = emptyStores();
    stores.antigravityQuota[file.name] = {
      ...buildQuotaCredentialIdentity(file),
      status: 'success',
      groups: [],
      subscription: {
        plan: 'unknown',
        tierName: 'Antigravity Future',
        tierId: 'future-tier',
      },
    };

    expect(resolveAccountQuota(file, stores).planType).toBe('Antigravity Future');
  });
});
