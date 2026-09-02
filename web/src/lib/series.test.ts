import { describe, expect, it } from 'vitest';
import { bucketize, delta, lastDefined, mergeSparkline } from './series';

describe('bucketize', () => {
  it('averages samples per slot and leaves empty slots null', () => {
    const samples = [
      { at: 0, value: 10 },
      { at: 5, value: 20 },
      { at: 25, value: 40 },
    ];
    expect(bucketize(samples, 0, 40, 4)).toEqual([15, null, 40, null]);
  });

  it('ignores samples outside the window and non-finite values', () => {
    const samples = [
      { at: -1, value: 5 },
      { at: 3, value: null },
      { at: 3, value: Number.NaN },
      { at: 41, value: 5 },
    ];
    expect(bucketize(samples, 0, 40, 2)).toEqual([null, null]);
  });

  it('places the last instant in the final slot', () => {
    expect(bucketize([{ at: 40, value: 1 }], 0, 40, 4)).toEqual([
      null,
      null,
      null,
      1,
    ]);
  });
});

describe('mergeSparkline', () => {
  it('appends newer live samples and keeps the length fixed', () => {
    const base = [1, 2, 3, 4];
    const merged = mergeSparkline(base, 60, 240_000, [
      { at: 250_000, value: 10 },
      { at: 290_000, value: 20 },
      { at: 320_000, value: 30 },
    ]);
    expect(merged).toEqual([3, 4, 15, 30]);
  });

  it('returns the base when nothing is newer', () => {
    const base = [1, null, 3];
    expect(mergeSparkline(base, 60, 1000, [{ at: 500, value: 9 }])).toBe(base);
  });
});

describe('delta and lastDefined', () => {
  it('compares oldest and newest defined values', () => {
    expect(delta([null, 2, 5, null, 9])).toBe(7);
    expect(delta([null, 4])).toBeNull();
    expect(lastDefined([1, 2, null])).toBe(2);
    expect(lastDefined([null])).toBeNull();
  });
});
