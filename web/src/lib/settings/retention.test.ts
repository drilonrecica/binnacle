import { describe, expect, it } from 'vitest';
import { effectiveTiers, shortening } from './retention';

const hour = 3_600_000;
const day = 24 * hour;

describe('retention shortening (UI-8)', () => {
  it('reads preset tiers and server-formatted advanced tiers', () => {
    expect(effectiveTiers({ 'retention.preset': 'balanced' })).toEqual({
      'retention.raw': 48 * hour,
      'retention.one_minute': 30 * day,
      'retention.fifteen_minute': 365 * day,
      'retention.one_hour': 0,
    });
    expect(
      effectiveTiers({
        'retention.preset': 'advanced',
        'retention.raw': '48h0m0s',
        'retention.one_minute': '720h0m0s',
        'retention.fifteen_minute': '8760h0m0s',
        'retention.one_hour': '0s',
      })['retention.one_minute'],
    ).toBe(30 * day);
  });

  it('flags a preset that keeps less', () => {
    const result = shortening(
      effectiveTiers({ 'retention.preset': 'balanced' }),
      effectiveTiers({ 'retention.preset': 'minimal' }),
    );
    // Hourly goes from forever to one year, which also trims history.
    expect(result).toEqual({
      tiers: [
        'retention.raw',
        'retention.one_minute',
        'retention.fifteen_minute',
        'retention.one_hour',
      ],
      keepMs: 12 * hour,
    });
  });

  it('does not flag a change that only keeps more', () => {
    expect(
      shortening(
        effectiveTiers({ 'retention.preset': 'balanced' }),
        effectiveTiers({ 'retention.preset': 'long-term' }),
      ),
    ).toBeNull();
  });

  it('flags one advanced tier and ignores unparsable values', () => {
    const current = effectiveTiers({ 'retention.preset': 'balanced' });
    expect(
      shortening(
        current,
        effectiveTiers({
          'retention.preset': 'advanced',
          'retention.raw': '48h',
          'retention.one_minute': '168h',
          'retention.fifteen_minute': 'soon',
          'retention.one_hour': '0',
        }),
      ),
    ).toEqual({ tiers: ['retention.one_minute'], keepMs: 7 * day });
  });
});
