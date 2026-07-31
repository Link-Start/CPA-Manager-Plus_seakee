import type { ManagerLatestRelease } from '@/services/api/version';
import type { RuntimeStatusResult, RuntimeUpdateCheckComponent } from '@/services/api/usageService';
import type { VersionComparison } from '@/utils/version';

type VersionPayload = Record<string, unknown> | undefined | null;
type RuntimeUpdateStatus = {
  deployment?: Pick<RuntimeStatusResult['deployment'], 'mode'>;
  runtimeAvailable?: boolean;
  updateCapabilities?: Pick<RuntimeStatusResult['updateCapabilities'], 'all'>;
};

export interface VersionBadgeCheck {
  comparison: VersionComparison;
  latest: string;
}

export const readManagerLatestTag = (data: ManagerLatestRelease | VersionPayload): string => {
  if (!data) return '';
  const raw = data.tag_name ?? data.name ?? data.latest_version ?? data.latest;
  return typeof raw === 'string' ? raw : raw == null ? '' : String(raw);
};

export const readApiLatestVersion = (data: VersionPayload): string => {
  if (!data) return '';
  const raw = data['latest-version'] ?? data.latest_version ?? data.latest;
  return typeof raw === 'string' ? raw : raw == null ? '' : String(raw);
};

export const canManageRuntimeUpdates = (status: RuntimeUpdateStatus | null | undefined) =>
  Boolean(
    status?.deployment?.mode === 'integrated' &&
    status.runtimeAvailable &&
    status.updateCapabilities?.all
  );

export const resolveVersionBadgeCheck = (
  runtimeUpdatesManaged: boolean,
  runtimeComponent: RuntimeUpdateCheckComponent | undefined,
  publicComparison: VersionComparison,
  publicLatest: string
): VersionBadgeCheck | null => {
  if (!runtimeUpdatesManaged) {
    return { comparison: publicComparison, latest: publicLatest };
  }
  if (!runtimeComponent) return null;
  if (!runtimeComponent.updateAvailable) {
    return {
      comparison: 0,
      latest: runtimeComponent.availableVersion || runtimeComponent.currentVersion || '',
    };
  }
  const latest = runtimeComponent.availableVersion?.trim() || '';
  return latest ? { comparison: 1, latest } : null;
};

export const shouldShowRuntimeUpdateLink = (
  sessionMode: string,
  runtimeUpdatesManaged: boolean,
  runtimeUpdateAvailable: boolean
) => sessionMode === 'manager_embedded' && runtimeUpdatesManaged && runtimeUpdateAvailable;
