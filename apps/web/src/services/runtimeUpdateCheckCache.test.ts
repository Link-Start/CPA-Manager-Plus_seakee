import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { RuntimeUpdateCheckResult } from '@/services/api/usageService';
import {
  clearRuntimeUpdateCheckCache,
  loadRuntimeUpdateCheck,
  readRuntimeUpdateCheckCache,
  runtimeUpdateCheckScope,
  storeRuntimeUpdateCheckCache,
} from './runtimeUpdateCheckCache';

const result = (checkedAtMs: number): RuntimeUpdateCheckResult => ({
  checkedAtMs,
  deployment: {
    schemaVersion: 1,
    mode: 'integrated',
    panelBasePath: '/management.html',
    panelBasePathSource: 'runtime',
    runtimeManaged: true,
    cpampUpdatesManaged: true,
    cpaUpdatesManaged: true,
  },
  components: {},
});

describe('runtime update check cache', () => {
  beforeEach(() => {
    clearRuntimeUpdateCheckCache();
  });

  it('scopes cached checks to the active API base and credential', () => {
    const scope = runtimeUpdateCheckScope('http://manager.local', 'admin-key');
    const otherScope = runtimeUpdateCheckScope('http://manager.local', 'other-key');
    storeRuntimeUpdateCheckCache(scope, result(1));

    expect(readRuntimeUpdateCheckCache(scope)?.checkedAtMs).toBe(1);
    expect(readRuntimeUpdateCheckCache(otherScope)).toBeNull();
  });

  it('deduplicates automatic checks but forces a manual refresh', async () => {
    const scope = runtimeUpdateCheckScope('http://manager.local', 'admin-key');
    const loader = vi
      .fn<() => Promise<RuntimeUpdateCheckResult>>()
      .mockResolvedValueOnce(result(1))
      .mockResolvedValueOnce(result(2));

    const first = loadRuntimeUpdateCheck(scope, loader);
    const second = loadRuntimeUpdateCheck(scope, loader);
    await expect(Promise.all([first, second])).resolves.toEqual([result(1), result(1)]);
    expect(loader).toHaveBeenCalledTimes(1);

    await expect(loadRuntimeUpdateCheck(scope, loader, true)).resolves.toEqual(result(2));
    expect(loader).toHaveBeenCalledTimes(2);
    expect(readRuntimeUpdateCheckCache(scope)?.checkedAtMs).toBe(2);
  });

  it('does not restore a pending result after the session cache is cleared', async () => {
    const scope = runtimeUpdateCheckScope('http://manager.local', 'admin-key');
    let resolve!: (value: RuntimeUpdateCheckResult) => void;
    const pending = loadRuntimeUpdateCheck(
      scope,
      () =>
        new Promise<RuntimeUpdateCheckResult>((promiseResolve) => {
          resolve = promiseResolve;
        })
    );

    clearRuntimeUpdateCheckCache();
    resolve(result(1));
    await pending;

    expect(readRuntimeUpdateCheckCache(scope)).toBeNull();
  });
});
