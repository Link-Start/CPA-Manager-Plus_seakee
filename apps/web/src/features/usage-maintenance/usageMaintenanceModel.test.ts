import { describe, expect, it } from 'vitest';
import {
  getArchiveRunPresentationStage,
  recommendRetentionDays,
  resolveRawEventRange,
  resolveRetentionCutoff,
  toLocalDateTimeValue,
} from './usageMaintenanceModel';

const dayMS = 24 * 60 * 60 * 1000;

describe('usage maintenance model', () => {
  it('resolves preset and valid custom retention cutoffs', () => {
    const nowMS = new Date('2026-08-18T12:00:00Z').getTime();
    expect(resolveRetentionCutoff(30, '', nowMS)).toBe(nowMS - 30 * dayMS);

    const custom = toLocalDateTimeValue(nowMS - 12 * dayMS);
    expect(resolveRetentionCutoff('custom', custom, nowMS)).toBe(nowMS - 12 * dayMS);
  });

  it('rejects invalid and future custom cutoffs', () => {
    const nowMS = new Date('2026-08-18T12:00:00Z').getTime();
    expect(resolveRetentionCutoff('custom', '', nowMS)).toBeNull();
    expect(resolveRetentionCutoff('custom', 'not-a-date', nowMS)).toBeNull();
    expect(resolveRetentionCutoff('custom', toLocalDateTimeValue(nowMS + dayMS), nowMS)).toBeNull();
  });

  it('distinguishes empty, compatible, and legacy raw ranges', () => {
    expect(resolveRawEventRange({ raw_event_count: 0 })).toEqual({ kind: 'empty' });
    expect(resolveRawEventRange({ raw_event_count: 10 })).toEqual({ kind: 'unavailable' });
    expect(
      resolveRawEventRange({
        raw_event_count: 10,
        raw_min_timestamp_ms: 1_000,
        raw_max_timestamp_ms: 3_000,
      })
    ).toEqual({ kind: 'available', minTimestampMS: 1_000, maxTimestampMS: 3_000 });
    expect(
      resolveRawEventRange({
        raw_event_count: 10,
        raw_min_timestamp_ms: 3_000,
        raw_max_timestamp_ms: 1_000,
      })
    ).toEqual({ kind: 'unavailable' });
  });

  it('recommends the longest preset that still matches at least one raw event', () => {
    const nowMS = new Date('2026-08-18T12:00:00Z').getTime();
    const range = (ageDays: number) =>
      resolveRawEventRange({
        raw_event_count: 10,
        raw_min_timestamp_ms: nowMS - ageDays * dayMS,
        raw_max_timestamp_ms: nowMS - dayMS,
      });

    expect(recommendRetentionDays(range(120), nowMS)).toBe(90);
    expect(recommendRetentionDays(range(45), nowMS)).toBe(30);
    expect(recommendRetentionDays(range(20), nowMS)).toBe(7);
    expect(recommendRetentionDays(range(3), nowMS)).toBeNull();
  });

  it('maps internal archive states to user-facing workflow stages', () => {
    expect(getArchiveRunPresentationStage({ status: 'previewed' })).toBe('archiving');
    expect(getArchiveRunPresentationStage({ status: 'failed', resume_status: 'verifying' })).toBe(
      'attention'
    );
    expect(getArchiveRunPresentationStage({ status: 'verified' })).toBe('delete_ready');
    expect(getArchiveRunPresentationStage({ status: 'deleting' })).toBe('deleting');
    expect(getArchiveRunPresentationStage({ status: 'completed' })).toBe('completed');
    expect(getArchiveRunPresentationStage({ status: 'cancelled' })).toBe('attention');
  });
});
