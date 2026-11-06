// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest';
import { newSeenMark, parseSeen, pruneSeen, seenStorageKey } from './seen';
import { SeenMarkers } from './seen.svelte';

describe('seen markers (UI-21)', () => {
  beforeEach(() => localStorage.clear());

  it('parses stored marks and drops malformed entries', () => {
    const raw = JSON.stringify({
      a: { by: 'alice', at: '2026-11-06T10:00:00.000Z' },
      b: { by: 'bob' },
      c: { by: 3, at: '2026-11-06T10:00:00.000Z' },
      d: { by: 'dana', at: 'not a date' },
      e: null,
    });
    expect(parseSeen(raw)).toEqual({
      a: { by: 'alice', at: '2026-11-06T10:00:00.000Z' },
    });
    expect(parseSeen('{not json')).toEqual({});
    expect(parseSeen('[]')).toEqual({});
    expect(parseSeen(null)).toEqual({});
  });

  it('keeps only the newest marks', () => {
    const marks = Object.fromEntries(
      [1, 2, 3].map((n) => [
        `inc-${n}`,
        { by: 'a', at: new Date(Date.UTC(2026, 10, n)).toISOString() },
      ]),
    );
    expect(Object.keys(pruneSeen(marks, 2)).sort()).toEqual(['inc-2', 'inc-3']);
  });

  it('reads an empty name as you', () => {
    expect(newSeenMark('', 0)).toEqual({
      by: 'you',
      at: '1970-01-01T00:00:00.000Z',
    });
  });

  it('stores marks in this browser only and reads them back', () => {
    const first = new SeenMarkers();
    first.mark('inc-1', 'alice', Date.parse('2026-11-06T10:00:00Z'));
    expect(first.get('inc-1')?.by).toBe('alice');
    expect(JSON.parse(localStorage.getItem(seenStorageKey) ?? '{}')).toEqual({
      'inc-1': { by: 'alice', at: '2026-11-06T10:00:00.000Z' },
    });

    const second = new SeenMarkers();
    expect(second.get('inc-1')?.by).toBe('alice');
    second.clear('inc-1');
    expect(new SeenMarkers().get('inc-1')).toBeUndefined();
  });
});
