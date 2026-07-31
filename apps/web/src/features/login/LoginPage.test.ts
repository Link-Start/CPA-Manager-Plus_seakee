import { describe, expect, it } from 'vitest';
import { resolveUsageServiceLoginMode } from './loginMode';
import { localizeSetupDocsUrl } from './setupDocs';

describe('resolveUsageServiceLoginMode', () => {
  it('keeps CPA-hosted panels on the regular login flow', () => {
    expect(resolveUsageServiceLoginMode(undefined)).toEqual({
      hostedByUsageService: false,
      usageServiceNeedsSetup: false,
    });
    expect(resolveUsageServiceLoginMode({ service: 'cli-proxy-api' })).toEqual({
      hostedByUsageService: false,
      usageServiceNeedsSetup: false,
    });
  });

  it('uses setup only for unconfigured Usage Service hosted panels', () => {
    expect(
      resolveUsageServiceLoginMode({ service: 'cpa-manager-plus', configured: false })
    ).toEqual({
      hostedByUsageService: true,
      usageServiceNeedsSetup: true,
    });
  });

  it('uses regular login for configured Usage Service hosted panels', () => {
    expect(resolveUsageServiceLoginMode({ service: 'cpa-manager-plus', configured: true })).toEqual(
      {
        hostedByUsageService: true,
        usageServiceNeedsSetup: false,
      }
    );
  });

  it('honors the explicit setup state used by migrated and partially initialized deployments', () => {
    expect(
      resolveUsageServiceLoginMode({
        service: 'cpa-manager-plus',
        configured: true,
        projectInitialized: true,
        setupRequired: true,
      })
    ).toEqual({
      hostedByUsageService: true,
      usageServiceNeedsSetup: true,
    });
    expect(
      resolveUsageServiceLoginMode({
        service: 'cpa-manager-plus',
        configured: true,
        projectInitialized: false,
      })
    ).toEqual({
      hostedByUsageService: true,
      usageServiceNeedsSetup: true,
    });
  });

  it('still recognizes legacy service ids (cpa-manager) as Usage Service', () => {
    expect(resolveUsageServiceLoginMode({ service: 'cpa-manager', configured: true })).toEqual({
      hostedByUsageService: true,
      usageServiceNeedsSetup: false,
    });
  });
});

describe('localizeSetupDocsUrl', () => {
  const setupUrl =
    'https://seakee.github.io/CPA-Manager-Plus/docs/troubleshooting/setup.html#cpa-unreachable';

  it('opens the English troubleshooting document for non-Chinese interfaces', () => {
    expect(localizeSetupDocsUrl(setupUrl, 'en')).toContain(
      '/docs/en/troubleshooting/setup.html#cpa-unreachable'
    );
    expect(localizeSetupDocsUrl(setupUrl, 'ru')).toContain(
      '/docs/en/troubleshooting/setup.html#cpa-unreachable'
    );
  });

  it('keeps Chinese interfaces on the Chinese troubleshooting document', () => {
    expect(localizeSetupDocsUrl(setupUrl, 'zh-CN')).toBe(setupUrl);
    expect(localizeSetupDocsUrl(setupUrl.replace('/docs/', '/docs/en/'), 'zh-TW')).toBe(setupUrl);
  });
});
