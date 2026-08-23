export type AccountQuotaRequestVersions = Map<string, number>;

export const beginAccountQuotaRequest = (
  versions: AccountQuotaRequestVersions,
  key: string
): (() => boolean) => {
  const version = (versions.get(key) ?? 0) + 1;
  versions.set(key, version);
  return () => versions.get(key) === version;
};

/**
 * Advance the version for a completed quota mutation (e.g. a consumed Codex
 * reset credit). Reads that started before mutation completion held an older
 * version, so their `isCurrent()` guards turn false and they can no longer
 * commit over the post-mutation state. Requests started afterwards receive a
 * newer version through `beginAccountQuotaRequest` and win normally, so the
 * fence never blocks future refreshes.
 */
export const completeAccountQuotaMutation = (
  versions: AccountQuotaRequestVersions,
  key: string
): number => {
  const nextVersion = (versions.get(key) ?? 0) + 1;
  versions.set(key, nextVersion);
  return nextVersion;
};

export const getAccountQuotaRequestVersion = (
  versions: AccountQuotaRequestVersions,
  key: string
): number => versions.get(key) ?? 0;
