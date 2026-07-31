import { describe, expect, it } from 'vitest';
import {
  canManageRuntimeUpdates,
  resolveVersionBadgeCheck,
  shouldShowRuntimeUpdateLink,
} from '@/features/system/versionChecks';

describe('canManageRuntimeUpdates', () => {
  const status = {
    deployment: { mode: 'integrated' },
    runtimeAvailable: true,
    updateCapabilities: { all: true },
  };

  it('requires an available Integrated runtime with full update capability', () => {
    expect(canManageRuntimeUpdates(status)).toBe(true);
    expect(canManageRuntimeUpdates({ ...status, runtimeAvailable: false })).toBe(false);
    expect(
      canManageRuntimeUpdates({
        ...status,
        deployment: { mode: 'installer-managed' },
      })
    ).toBe(false);
    expect(
      canManageRuntimeUpdates({
        ...status,
        updateCapabilities: { all: false },
      })
    ).toBe(false);
  });
});

describe('resolveVersionBadgeCheck', () => {
  it('uses the signed runtime manifest instead of a contradictory public release result', () => {
    expect(
      resolveVersionBadgeCheck(
        true,
        {
          name: 'cpamp',
          currentVersion: 'v1.11.9',
          availableVersion: 'v1.11.9',
          updateAvailable: false,
        },
        1,
        'v1.12.0'
      )
    ).toEqual({ comparison: 0, latest: 'v1.11.9' });
  });

  it('does not claim a missing signed component is current', () => {
    expect(resolveVersionBadgeCheck(true, undefined, 1, 'v1.12.0')).toBeNull();
  });

  it('keeps public version comparisons for deployments without managed runtime updates', () => {
    expect(resolveVersionBadgeCheck(false, undefined, 1, 'v1.12.0')).toEqual({
      comparison: 1,
      latest: 'v1.12.0',
    });
  });
});

describe('shouldShowRuntimeUpdateLink', () => {
  it('shows the runtime update entry for integrated sessions with a CPAMP update', () => {
    expect(shouldShowRuntimeUpdateLink('manager_embedded', true, true)).toBe(true);
  });

  it('keeps the update action out of external panel sessions', () => {
    expect(shouldShowRuntimeUpdateLink('external_panel', true, true)).toBe(false);
  });

  it('keeps the update action out of split or unavailable Manager sessions', () => {
    expect(shouldShowRuntimeUpdateLink('manager_embedded', false, true)).toBe(false);
  });

  it('uses the signed runtime manifest result instead of public version comparisons', () => {
    expect(shouldShowRuntimeUpdateLink('manager_embedded', true, false)).toBe(false);
  });
});
