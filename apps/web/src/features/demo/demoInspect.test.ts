import { describe, expect, it } from 'vitest';
import {
  getDemoAuthFiles,
  getDemoQuotaStoreState,
  getDemoAccountWindowUsage,
} from './demoFixtures';
import { buildAccountRows } from '@/features/accounts/model/accountRows';
import { buildAccountQuotaDisplayWindows } from '@/features/accounts/model/accountQuotaDisplayWindows';
import { buildAccountQuotaWindowDefinitions } from '@/features/accounts/model/accountQuotaWindowDefinitions';
import { selectAccountQuotaMainListWindows } from '@/features/accounts/model/accountsPagePresentation';
import { buildAccountSubscriptionPresentation } from '@/features/accounts/model/accountSubscriptionPresentation';
import { buildAccountWindowUsageTargetEntries } from '@/features/accounts/model/accountWindowUsageRows';
import { resolveAccountQuotaWindowUsageAndForecast } from '@/features/accounts/model/accountQuotaWindowUsagePresentation';
import type { AccountRow } from '@/features/accounts/model/accountRows';
import type { MonitoringAccountWindowUsageItem } from '@/services/api/usageService';

describe('Demo accounts quota & usage presentation regression', () => {
  it('correctly presents subscription plans and remaining days for demo accounts', () => {
    const authFiles = getDemoAuthFiles().files;
    const quotaState = getDemoQuotaStoreState();
    const rows = buildAccountRows(authFiles, quotaState);

    expect(rows.length).toBe(23);

    const proRow = rows.find((r) => r.fileName === 'codex-pro-20x-01.json');
    expect(proRow).toBeDefined();
    const proCodexQuota = Object.values(quotaState.codexQuota).find(
      (q) => q?.authFileName === proRow?.fileName
    );
    const proSub = buildAccountSubscriptionPresentation({
      row: proRow!,
      codexQuota: proCodexQuota,
    });
    expect(proSub.isPaidCodex).toBe(true);
    expect(proSub.effectivePlanType).toBe('pro');
    expect(proSub.remainingDays).toBeGreaterThan(0);

    const plusRow = rows.find((r) => r.fileName === 'codex-email-user.json');
    expect(plusRow).toBeDefined();
    const plusCodexQuota = Object.values(quotaState.codexQuota).find(
      (q) => q?.authFileName === plusRow?.fileName
    );
    const plusSub = buildAccountSubscriptionPresentation({
      row: plusRow!,
      codexQuota: plusCodexQuota,
    });
    expect(plusSub.isPaidCodex).toBe(true);
    expect(plusSub.effectivePlanType).toBe('plus');
    expect(plusSub.remainingDays).toBeGreaterThan(0);
  });

  it('selects valid quota list windows and produces reliable actual usage and forecasts', () => {
    const authFiles = getDemoAuthFiles().files;
    const quotaState = getDemoQuotaStoreState();
    const rows = buildAccountRows(authFiles, quotaState);

    const options = {
      stores: quotaState,
      getDisplayCodexQuota: (raw: { name?: string }) =>
        Object.values(quotaState.codexQuota).find((q) => q?.authFileName === raw.name),
      translateQuotaWindowLabel: (label?: string, key?: string) => label || key || '',
      t: ((k: string) => k) as unknown as (key: string) => string,
    };

    const windowsByRowKey = new Map();
    rows.forEach((row) => {
      const displayWindows = buildAccountQuotaDisplayWindows(row, options);
      const definitions = buildAccountQuotaWindowDefinitions(displayWindows);
      windowsByRowKey.set(row.selectionKey, definitions);
    });

    const targetEntries = buildAccountWindowUsageTargetEntries(rows, windowsByRowKey);
    expect(targetEntries.length).toBeGreaterThan(0);

    const response = getDemoAccountWindowUsage({
      windows: targetEntries.map((e) => e.target),
    });
    expect(response.items.length).toBe(targetEntries.length);
    expect(response.items.every((i) => i.matched)).toBe(true);

    const usageByKey = new Map<string, MonitoringAccountWindowUsageItem>();
    response.items.forEach((item) => {
      if (item.request_key) {
        usageByKey.set(item.request_key, item);
      }
    });

    const checkProviderPresentation = (
      provider: AccountRow['provider'],
      fileNameFilter?: string
    ) => {
      const matchedRows = rows.filter(
        (r) => r.provider === provider && (!fileNameFilter || r.fileName === fileNameFilter)
      );
      expect(matchedRows.length).toBeGreaterThan(0);

      matchedRows.forEach((row) => {
        const displayWindows = buildAccountQuotaDisplayWindows(row, options);
        const mainWindows = selectAccountQuotaMainListWindows(row, displayWindows);
        expect(mainWindows.length).toBeGreaterThan(0);
        expect(mainWindows.length).toBeLessThanOrEqual(2);

        mainWindows.forEach((w) => {
          expect(w.windowMode).not.toBe('unknown');
          expect(w.quotaProgressObservedAtMs).toBeTypeOf('number');

          const usageData = resolveAccountQuotaWindowUsageAndForecast(row, w, usageByKey);
          expect(usageData.hasTrustedCurrentActual).toBe(true);
          expect(usageData.currentCost).toBeTypeOf('number');
          expect(usageData.currentTokens).toBeTypeOf('number');

          // Ensure linear extrapolation forecast successfully computes without falling back to null
          expect(usageData.forecast).not.toBeNull();
          expect(usageData.forecastCost).toBeTypeOf('number');
          expect(usageData.forecastTokens).toBeTypeOf('number');
        });
      });
    };

    // Verify key representative accounts across all supported providers
    checkProviderPresentation('codex', 'codex-pro-20x-01.json');
    checkProviderPresentation('codex', 'codex-email-user.json');
    checkProviderPresentation('claude', 'claude-team-01.json');
    checkProviderPresentation('antigravity', 'antigravity-builder.json');
    checkProviderPresentation('kimi', 'kimi-coding.json');
    checkProviderPresentation('xai', 'xai-ops.json');
  });
});
