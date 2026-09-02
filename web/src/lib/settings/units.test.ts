import { describe, expect, it } from 'vitest';
import {
  bytesToInput,
  formatGoDuration,
  humanDuration,
  inputToBytes,
  parseGoDuration,
} from './units';

describe('durations', () => {
  it('parses Go duration strings', () => {
    expect(parseGoDuration('5s')).toBe(5000);
    expect(parseGoDuration('1h30m')).toBe(5_400_000);
    expect(parseGoDuration('500ms')).toBe(500);
    expect(parseGoDuration('1.5h')).toBe(5_400_000);
  });

  it('rejects malformed values', () => {
    expect(parseGoDuration('')).toBeNull();
    expect(parseGoDuration('5')).toBeNull();
    expect(parseGoDuration('5 s')).toBeNull();
    expect(parseGoDuration('2d')).toBeNull();
    expect(parseGoDuration('1h x')).toBeNull();
  });

  it('formats exact durations compactly', () => {
    expect(formatGoDuration(5000)).toBe('5s');
    expect(formatGoDuration(5_400_000)).toBe('1h30m');
    expect(formatGoDuration(172_800_000)).toBe('48h');
    expect(formatGoDuration(0)).toBe('0s');
  });

  it('phrases durations for humans', () => {
    expect(humanDuration('48h')).toBe('2 days');
    expect(humanDuration('30m')).toBe('30 minutes');
    expect(humanDuration('1h')).toBe('1 hour');
    expect(humanDuration('nonsense')).toBe('nonsense');
  });
});

describe('bytes', () => {
  it('round-trips byte sizes through the largest exact unit', () => {
    expect(bytesToInput(1 << 30)).toEqual({ value: 1, unit: 'GiB' });
    expect(bytesToInput(1536 * 1024 * 1024)).toEqual({
      value: 1536,
      unit: 'MiB',
    });
    expect(inputToBytes(2, 'GiB')).toBe(2 * 1024 ** 3);
    expect(inputToBytes(-1, 'MiB')).toBeNull();
  });
});
