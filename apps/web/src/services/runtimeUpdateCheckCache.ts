import type { RuntimeUpdateCheckResult } from '@/services/api/usageService';
import { sha256Hex } from '@/utils/apiKeyHash';

interface PendingRuntimeUpdateCheck {
  scope: string;
  revision: number;
  promise: Promise<RuntimeUpdateCheckResult>;
}

let cachedScope = '';
let cachedResult: RuntimeUpdateCheckResult | null = null;
let pendingCheck: PendingRuntimeUpdateCheck | null = null;
let revision = 0;

export const runtimeUpdateCheckScope = (apiBase: string, managementKey: string) =>
  apiBase && managementKey ? sha256Hex(`${apiBase}\u0000${managementKey}`) : '';

export const readRuntimeUpdateCheckCache = (scope: string) =>
  scope && cachedScope === scope ? cachedResult : null;

export const storeRuntimeUpdateCheckCache = (
  scope: string,
  result: RuntimeUpdateCheckResult
) => {
  if (!scope) return;
  revision += 1;
  cachedScope = scope;
  cachedResult = result;
  pendingCheck = null;
};

export const clearRuntimeUpdateCheckCache = () => {
  revision += 1;
  cachedScope = '';
  cachedResult = null;
  pendingCheck = null;
};

export const loadRuntimeUpdateCheck = (
  scope: string,
  loader: () => Promise<RuntimeUpdateCheckResult>,
  force = false
) => {
  if (!scope) {
    return Promise.reject(new Error('runtime update check scope is required'));
  }
  if (!force) {
    const cached = readRuntimeUpdateCheckCache(scope);
    if (cached) return Promise.resolve(cached);
    if (pendingCheck?.scope === scope) return pendingCheck.promise;
  }

  revision += 1;
  const requestRevision = revision;
  const promise = loader().then(
    (result) => {
      if (revision === requestRevision) {
        cachedScope = scope;
        cachedResult = result;
        pendingCheck = null;
      }
      return result;
    },
    (error: unknown) => {
      if (revision === requestRevision) {
        pendingCheck = null;
      }
      throw error;
    }
  );
  pendingCheck = { scope, revision: requestRevision, promise };
  return promise;
};
