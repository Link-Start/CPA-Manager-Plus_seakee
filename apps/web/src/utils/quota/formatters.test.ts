import { describe, expect, it } from 'vitest';
import { deleteTrackedPromise } from './formatters';

describe('deleteTrackedPromise', () => {
  it('deletes the tracked promise when references match', () => {
    const map = new Map<string, Promise<unknown>>();
    const promise = Promise.resolve('ok');
    map.set('key-1', promise);

    expect(deleteTrackedPromise(map, 'key-1', promise)).toBe(true);
    expect(map.has('key-1')).toBe(false);
  });

  it('preserves a newer in-flight promise when an older promise completes', () => {
    const map = new Map<string, Promise<unknown>>();
    const oldPromise = Promise.resolve('old');
    const newPromise = Promise.resolve('new');

    map.set('key-1', newPromise);

    expect(deleteTrackedPromise(map, 'key-1', oldPromise)).toBe(false);
    expect(map.get('key-1')).toBe(newPromise);
  });

  it('handles non-existent keys gracefully without error', () => {
    const map = new Map<string, Promise<unknown>>();
    const promise = Promise.resolve('other');

    expect(deleteTrackedPromise(map, 'key-2', promise)).toBe(false);
  });
});
