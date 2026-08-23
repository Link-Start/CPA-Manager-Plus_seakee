import { describe, expect, it } from 'vitest';
import {
  beginAccountQuotaRequest,
  completeAccountQuotaMutation,
  getAccountQuotaRequestVersion,
} from './accountQuotaRequestGate';

describe('beginAccountQuotaRequest', () => {
  it('invalidates older requests for the same quota target only', () => {
    const versions = new Map<string, number>();
    const firstCodex = beginAccountQuotaRequest(versions, 'codex:shared');
    const xai = beginAccountQuotaRequest(versions, 'xai:shared');
    const secondCodex = beginAccountQuotaRequest(versions, 'codex:shared');

    expect(firstCodex()).toBe(false);
    expect(secondCodex()).toBe(true);
    expect(xai()).toBe(true);
  });

  it('makes mutation completion invalidate reads that started before it', () => {
    const versions = new Map<string, number>();
    const beforeMutation = beginAccountQuotaRequest(versions, 'codex:credential');

    const fenceVersion = completeAccountQuotaMutation(versions, 'codex:credential');

    expect(fenceVersion).toBe(2);
    expect(getAccountQuotaRequestVersion(versions, 'codex:credential')).toBe(2);
    expect(beforeMutation()).toBe(false);
  });

  it('allows reads started after the mutation fence to commit normally', () => {
    const versions = new Map<string, number>();
    const beforeMutation = beginAccountQuotaRequest(versions, 'codex:credential');
    completeAccountQuotaMutation(versions, 'codex:credential');
    const afterMutation = beginAccountQuotaRequest(versions, 'codex:credential');

    expect(beforeMutation()).toBe(false);
    expect(afterMutation()).toBe(true);
    expect(getAccountQuotaRequestVersion(versions, 'codex:credential')).toBe(3);
  });

  it('keeps the fence credential-scoped', () => {
    const versions = new Map<string, number>();
    const codex = beginAccountQuotaRequest(versions, 'codex:credential');
    const otherCredential = beginAccountQuotaRequest(versions, 'codex:other');

    completeAccountQuotaMutation(versions, 'codex:credential');

    expect(codex()).toBe(false);
    expect(otherCredential()).toBe(true);
  });
});
