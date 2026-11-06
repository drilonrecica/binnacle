import { describe, expect, it, vi } from 'vitest';
import type { LogEntry } from '../api/logs';
import { LogRing } from './ring';

function entry(n: number): LogEntry {
  return {
    timestamp: new Date(n * 1000).toISOString(),
    component: 'c1',
    stream: 'stdout',
    severity: 'info',
    message: `line ${n}`,
  };
}

describe('LogRing', () => {
  it('keeps the newest lines in order once full', () => {
    const ring = new LogRing(3);
    for (let n = 0; n < 5; n++) ring.push(entry(n));
    expect(ring.size).toBe(3);
    expect(ring.dropped).toBe(2);
    expect(ring.tail(10).map((line) => line.entry.message)).toEqual([
      'line 2',
      'line 3',
      'line 4',
    ]);
    expect(ring.last?.entry.message).toBe('line 4');
    expect(ring.at(0)?.entry.message).toBe('line 2');
  });

  it('gives every line a sequence number that survives wrap-around', () => {
    const ring = new LogRing(2);
    for (let n = 0; n < 4; n++) ring.push(entry(n));
    expect(ring.tail(2).map((line) => line.seq)).toEqual([2, 3]);
  });

  it('returns only the newest window from tail', () => {
    const ring = new LogRing(10);
    for (let n = 0; n < 8; n++) ring.push(entry(n));
    expect(ring.tail(3).map((line) => line.seq)).toEqual([5, 6, 7]);
    expect(ring.tail(0)).toEqual([]);
  });

  it('bumps the version on every mutation', () => {
    const ring = new LogRing(2);
    const before = ring.version;
    ring.push(entry(1));
    ring.replace([entry(2), entry(3), entry(4)]);
    expect(ring.version).toBeGreaterThan(before + 1);
    expect(ring.tail(5).map((line) => line.entry.message)).toEqual([
      'line 3',
      'line 4',
    ]);
    ring.clear();
    expect(ring.size).toBe(0);
    expect(ring.dropped).toBe(0);
  });

  it('appends a streamed line without copying the buffer (UI-15)', () => {
    const ring = new LogRing(5_000);
    ring.replace(Array.from({ length: 5_000 }, (_, n) => entry(n)));
    const slice = vi.spyOn(Array.prototype, 'slice');
    const spread = vi.spyOn(Array.prototype, Symbol.iterator);
    try {
      ring.push(entry(5_000));
      expect(slice).not.toHaveBeenCalled();
      expect(spread).not.toHaveBeenCalled();
    } finally {
      slice.mockRestore();
      spread.mockRestore();
    }
    expect(ring.size).toBe(5_000);
    expect(ring.last?.entry.message).toBe('line 5000');
  });

  it('rejects a capacity that is not a positive integer', () => {
    expect(() => new LogRing(0)).toThrow();
  });
});
