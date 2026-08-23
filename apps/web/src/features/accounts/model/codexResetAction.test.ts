import { describe, expect, it } from 'vitest';
import {
  isCodexResetActionExecutable,
  resolveCodexResetActionPresentation,
  resolveCodexResetActionState,
  type CodexResetActionState,
  type CodexResetActionStateInput,
} from '@/features/accounts/model/codexResetAction';
import type { CodexResetCreditEvidence } from '@/features/accounts/model/accountQuotaSnapshots';
import type { AuthFileItem } from '@/types';

const codexFile = { name: 'codex.json', authIndex: 'auth-1' } as AuthFileItem;

const evidence = (overrides: Partial<CodexResetCreditEvidence>): CodexResetCreditEvidence => ({
  displayedCount: null,
  displayedCreditsCount: 0,
  source: 'none',
  observedAtMs: null,
  liveCount: null,
  liveObservedAtMs: null,
  snapshotCount: null,
  snapshotObservedAtMs: null,
  liveIsAuthoritative: false,
  snapshotOverridesLive: false,
  ...overrides,
});

const buildInput = (
  overrides: Partial<CodexResetActionStateInput> = {}
): CodexResetActionStateInput => ({
  row: { provider: 'codex', runtimeOnly: false, raw: codexFile },
  evidence: evidence({}),
  configurationSaving: false,
  verifying: false,
  consuming: false,
  ...overrides,
});

describe('codex reset action state', () => {
  it('marks non-codex providers unsupported', () => {
    expect(
      resolveCodexResetActionState(buildInput({ row: { ...buildInput().row, provider: 'claude' } }))
    ).toEqual({
      kind: 'unsupported',
      reasonKey: 'codex_quota.reset_unsupported_credential',
    });
  });

  it('keeps runtime-only credentials unsupported', () => {
    expect(
      resolveCodexResetActionState(buildInput({ row: { ...buildInput().row, runtimeOnly: true } }))
    ).toEqual({
      kind: 'unsupported',
      reasonKey: 'codex_quota.reset_unsupported_credential',
    });
  });

  it('marks credentials without auth_index unsupported', () => {
    const file = { name: 'codex.json' } as AuthFileItem;
    expect(
      resolveCodexResetActionState(buildInput({ row: { ...buildInput().row, raw: file } }))
    ).toEqual({
      kind: 'unsupported',
      reasonKey: 'codex_quota.missing_auth_index',
    });
  });

  it('treats an in-flight verification as busy in the verifying phase', () => {
    expect(resolveCodexResetActionState(buildInput({ verifying: true }))).toEqual({
      kind: 'busy',
      phase: 'verifying',
    });
  });

  it('treats an in-flight consume as busy in the consuming phase', () => {
    expect(resolveCodexResetActionState(buildInput({ consuming: true }))).toEqual({
      kind: 'busy',
      phase: 'consuming',
    });
  });

  it('treats configuration saving as busy in the configuration phase', () => {
    expect(resolveCodexResetActionState(buildInput({ configurationSaving: true }))).toEqual({
      kind: 'busy',
      phase: 'configuration',
    });
  });

  it('prefers the consuming phase over verifying when both are in flight', () => {
    expect(resolveCodexResetActionState(buildInput({ verifying: true, consuming: true }))).toEqual({
      kind: 'busy',
      phase: 'consuming',
    });
  });

  it('allows reset immediately when verified live evidence shows a positive count', () => {
    expect(
      resolveCodexResetActionState(
        buildInput({ evidence: evidence({ source: 'live', displayedCount: 3 }) })
      )
    ).toEqual({ kind: 'available', count: 3 });
  });

  it('stays unavailable when verified live evidence shows zero credits', () => {
    expect(
      resolveCodexResetActionState(
        buildInput({ evidence: evidence({ source: 'live', displayedCount: 0 }) })
      )
    ).toEqual({ kind: 'unavailable', reasonKey: 'codex_quota.reset_unavailable_no_credits' });
  });

  it('requires verification when a fresher snapshot shows a positive count', () => {
    expect(
      resolveCodexResetActionState(
        buildInput({ evidence: evidence({ source: 'snapshot', displayedCount: 1 }) })
      )
    ).toEqual({ kind: 'needs_verification', snapshotCount: 1 });
  });

  it('withdraws an older live count when a fresher snapshot shows zero', () => {
    expect(
      resolveCodexResetActionState(
        buildInput({ evidence: evidence({ source: 'snapshot', displayedCount: 0 }) })
      )
    ).toEqual({ kind: 'unavailable', reasonKey: 'codex_quota.reset_unavailable_no_credits' });
  });

  it('requires verification for unverified preserved counts and credit lists', () => {
    expect(
      resolveCodexResetActionState(
        buildInput({ evidence: evidence({ source: 'unverified', displayedCount: 1 }) })
      )
    ).toEqual({ kind: 'needs_verification', snapshotCount: 1 });
    expect(
      resolveCodexResetActionState(
        buildInput({
          evidence: evidence({ source: 'unverified', displayedCreditsCount: 2 }),
        })
      )
    ).toEqual({ kind: 'needs_verification', snapshotCount: null });
  });

  it('stays unavailable for unverified evidence without positive credits', () => {
    expect(
      resolveCodexResetActionState(
        buildInput({ evidence: evidence({ source: 'unverified', displayedCount: 0 }) })
      )
    ).toEqual({ kind: 'unavailable', reasonKey: 'codex_quota.reset_unavailable_no_credits' });
    expect(
      resolveCodexResetActionState(buildInput({ evidence: evidence({ source: 'none' }) }))
    ).toEqual({ kind: 'unavailable', reasonKey: 'codex_quota.reset_unavailable_no_credits' });
  });

  it('does not block disabled credentials with verified reset credits', () => {
    const file = { name: 'codex.json', authIndex: 'auth-1', disabled: true } as AuthFileItem;
    expect(
      resolveCodexResetActionState(
        buildInput({
          row: { provider: 'codex', runtimeOnly: false, raw: file },
          evidence: evidence({ source: 'live', displayedCount: 1 }),
        })
      )
    ).toEqual({ kind: 'available', count: 1 });
  });
});

describe('codex reset credit evidence freshness (live vs snapshot)', () => {
  const cases: Array<{
    name: string;
    source: CodexResetCreditEvidence['source'];
    displayedCount: number | null;
    expected: CodexResetActionState;
  }> = [
    {
      name: 'F1: older live zero superseded by newer snapshot one → needs_verification',
      source: 'snapshot',
      displayedCount: 1,
      expected: { kind: 'needs_verification', snapshotCount: 1 },
    },
    {
      name: 'F2: older live one superseded by newer snapshot zero → unavailable',
      source: 'snapshot',
      displayedCount: 0,
      expected: { kind: 'unavailable', reasonKey: 'codex_quota.reset_unavailable_no_credits' },
    },
    {
      name: 'F3: newer live zero over older snapshot one → unavailable',
      source: 'live',
      displayedCount: 0,
      expected: { kind: 'unavailable', reasonKey: 'codex_quota.reset_unavailable_no_credits' },
    },
    {
      name: 'F4: newer live one over older snapshot zero → available',
      source: 'live',
      displayedCount: 1,
      expected: { kind: 'available', count: 1 },
    },
  ];

  it.each(cases)('$name', ({ source, displayedCount, expected }) => {
    expect(
      resolveCodexResetActionState(buildInput({ evidence: evidence({ source, displayedCount }) }))
    ).toEqual(expected);
  });
});

describe('codex reset action presentation', () => {
  it('disables unsupported credentials with their reason as title', () => {
    expect(
      resolveCodexResetActionPresentation({
        kind: 'unsupported',
        reasonKey: 'codex_quota.reset_unsupported_credential',
      })
    ).toEqual({
      disabled: true,
      busy: false,
      titleKey: 'codex_quota.reset_unsupported_credential',
      interactive: false,
    });
  });

  it('shows a verifying spinner for the busy verifying phase', () => {
    expect(resolveCodexResetActionPresentation({ kind: 'busy', phase: 'verifying' })).toEqual({
      disabled: true,
      busy: true,
      titleKey: 'codex_quota.reset_verify_in_progress',
      interactive: true,
    });
  });

  it('shows a consuming spinner for the busy consuming phase', () => {
    expect(resolveCodexResetActionPresentation({ kind: 'busy', phase: 'consuming' })).toEqual({
      disabled: true,
      busy: true,
      titleKey: 'codex_quota.reset_consuming_in_progress',
      interactive: true,
    });
  });

  it('keeps configuration-saving busy disabled without a title', () => {
    expect(resolveCodexResetActionPresentation({ kind: 'busy', phase: 'configuration' })).toEqual({
      disabled: true,
      busy: false,
      titleKey: null,
      interactive: true,
    });
  });

  it('hints that verification runs before reset for executable states', () => {
    const states: CodexResetActionState[] = [
      { kind: 'available', count: 1 },
      { kind: 'needs_verification', snapshotCount: 1 },
    ];
    for (const state of states) {
      expect(resolveCodexResetActionPresentation(state)).toEqual({
        disabled: false,
        busy: false,
        titleKey: 'codex_quota.reset_requires_verification_hint',
        interactive: true,
      });
    }
  });

  it('disables unavailable credentials with the no-credits reason', () => {
    expect(
      resolveCodexResetActionPresentation({
        kind: 'unavailable',
        reasonKey: 'codex_quota.reset_unavailable_no_credits',
      })
    ).toEqual({
      disabled: true,
      busy: false,
      titleKey: 'codex_quota.reset_unavailable_no_credits',
      interactive: false,
    });
  });
});

describe('isCodexResetActionExecutable', () => {
  it('only treats available and needs_verification as executable', () => {
    expect(isCodexResetActionExecutable({ kind: 'available', count: 1 })).toBe(true);
    expect(isCodexResetActionExecutable({ kind: 'needs_verification', snapshotCount: 1 })).toBe(
      true
    );
    expect(
      isCodexResetActionExecutable({
        kind: 'unsupported',
        reasonKey: 'codex_quota.reset_unsupported_credential',
      })
    ).toBe(false);
    expect(isCodexResetActionExecutable({ kind: 'busy', phase: 'consuming' })).toBe(false);
    expect(
      isCodexResetActionExecutable({
        kind: 'unavailable',
        reasonKey: 'codex_quota.reset_unavailable_no_credits',
      })
    ).toBe(false);
  });
});
