import type { AccountRow } from '@/features/accounts/model/accountRows';
import type { CodexResetCreditEvidence } from '@/features/accounts/model/accountQuotaSnapshots';
import type { AuthFileItem } from '@/types';
import { normalizeAuthIndex } from '@/utils/authIndex';

const CODEX_PROVIDER_TYPE = 'codex';

/**
 * Reset action state for a Codex credential.
 *
 * Display evidence (live quota merged with persisted quota snapshots) decides
 * whether the action is reachable; snapshots never authorize the mutation by
 * themselves — a positive snapshot count only yields `needs_verification`,
 * and clicking always verifies the live quota first. `available` means the
 * latest evidence is a verified live count, not "execute without checking".
 */
export type CodexResetActionState =
  | { kind: 'unsupported'; reasonKey: string }
  | {
      kind: 'busy';
      phase:
        | 'verifying'
        | 'awaiting_confirmation'
        | 'consuming'
        | 'refreshing_after_consume'
        | 'configuration';
    }
  | { kind: 'available'; count: number }
  | { kind: 'needs_verification'; snapshotCount: number | null }
  | { kind: 'unavailable'; reasonKey: string };

export type CodexResetTransactionPhase =
  | 'verifying'
  | 'awaiting_confirmation'
  | 'consuming'
  | 'refreshing_after_consume';

export interface CodexResetActionStateInput {
  row: Pick<AccountRow, 'provider' | 'runtimeOnly' | 'raw'>;
  evidence: CodexResetCreditEvidence;
  configurationSaving: boolean;
  transactionPhase?: CodexResetTransactionPhase;
  /** Compatibility inputs for pure callers that have not adopted the phase map yet. */
  verifying?: boolean;
  consuming?: boolean;
}

export interface CodexResetActionPresentation {
  disabled: boolean;
  busy: boolean;
  titleKey: string | null;
  /** Whether the action is reachable at all (drawer menu item visibility). */
  interactive: boolean;
}

const hasResetAuthIndex = (file: AuthFileItem): boolean =>
  normalizeAuthIndex(file['auth_index'] ?? file.authIndex) !== null;

// row.disabled intentionally does not block the reset: the CPA management
// /api-call resolves disabled credentials by auth_index without filtering, and
// reset credits exist to recover quota-limited credentials. runtimeOnly rows
// stay blocked because plugin virtual credentials have no stable mutation
// target.
export const resolveCodexResetActionState = ({
  row,
  evidence,
  configurationSaving,
  transactionPhase,
  verifying = false,
  consuming = false,
}: CodexResetActionStateInput): CodexResetActionState => {
  if (row.provider !== CODEX_PROVIDER_TYPE || row.runtimeOnly) {
    return { kind: 'unsupported', reasonKey: 'codex_quota.reset_unsupported_credential' };
  }
  if (!hasResetAuthIndex(row.raw)) {
    return { kind: 'unsupported', reasonKey: 'codex_quota.missing_auth_index' };
  }
  const activePhase =
    transactionPhase ?? (consuming ? 'consuming' : verifying ? 'verifying' : null);
  if (activePhase) return { kind: 'busy', phase: activePhase };
  if (configurationSaving) return { kind: 'busy', phase: 'configuration' };

  const displayedCount = evidence.displayedCount;
  const hasPositiveDisplayedEvidence =
    displayedCount === null ? evidence.displayedCreditsCount > 0 : displayedCount > 0;
  switch (evidence.source) {
    case 'live':
      return (displayedCount ?? 0) > 0
        ? { kind: 'available', count: displayedCount as number }
        : { kind: 'unavailable', reasonKey: 'codex_quota.reset_unavailable_no_credits' };
    case 'snapshot':
      // A fresher snapshot owns the display. A positive snapshot count only
      // unlocks verification, never a direct consume; a fresher snapshot zero
      // withdraws an older live count.
      return hasPositiveDisplayedEvidence
        ? { kind: 'needs_verification', snapshotCount: displayedCount }
        : { kind: 'unavailable', reasonKey: 'codex_quota.reset_unavailable_no_credits' };
    case 'unverified':
      // Preserved count or credit list under a non-success live state, or a
      // live success with an unknown count: positive evidence only unlocks
      // verification.
      return hasPositiveDisplayedEvidence
        ? { kind: 'needs_verification', snapshotCount: displayedCount }
        : { kind: 'unavailable', reasonKey: 'codex_quota.reset_unavailable_no_credits' };
    case 'invalidated':
      return { kind: 'needs_verification', snapshotCount: null };
    case 'none':
      return { kind: 'unavailable', reasonKey: 'codex_quota.reset_unavailable_no_credits' };
  }
};

export const isCodexResetActionExecutable = (state: CodexResetActionState): boolean =>
  state.kind === 'available' || state.kind === 'needs_verification';

export const resolveCodexResetActionPresentation = (
  state: CodexResetActionState
): CodexResetActionPresentation => {
  switch (state.kind) {
    case 'unsupported':
      return { disabled: true, busy: false, titleKey: state.reasonKey, interactive: false };
    case 'busy':
      switch (state.phase) {
        case 'verifying':
          return {
            disabled: true,
            busy: true,
            titleKey: 'codex_quota.reset_verify_in_progress',
            interactive: true,
          };
        case 'consuming':
        case 'refreshing_after_consume':
          return {
            disabled: true,
            busy: true,
            titleKey: 'codex_quota.reset_consuming_in_progress',
            interactive: true,
          };
        case 'awaiting_confirmation':
          return {
            disabled: true,
            busy: false,
            titleKey: 'codex_quota.reset_confirmation_in_progress',
            interactive: true,
          };
        default:
          return { disabled: true, busy: false, titleKey: null, interactive: true };
      }
    case 'available':
    case 'needs_verification':
      return {
        disabled: false,
        busy: false,
        titleKey: 'codex_quota.reset_requires_verification_hint',
        interactive: true,
      };
    case 'unavailable':
      return { disabled: true, busy: false, titleKey: state.reasonKey, interactive: false };
  }
};
