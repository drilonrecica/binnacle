import { describe, expect, it } from 'vitest';
import { eventRangeFor } from './events';

describe('event history helpers', () => {
  it('computes ranges from now', () => {
    const now = new Date('2026-07-12T12:00:00.000Z');
    const { from, to } = eventRangeFor('24h', now);
    expect(to.toISOString()).toBe('2026-07-12T12:00:00.000Z');
    expect(from.toISOString()).toBe('2026-07-11T12:00:00.000Z');
  });
});
