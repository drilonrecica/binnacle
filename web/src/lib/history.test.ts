import { describe, expect, it } from 'vitest';
import { rangeFor, validateRange } from './history';

describe('history ranges', () => {
  it('uses bounded preset and custom ranges', () => {
    const now = new Date('2026-01-01T00:00:00Z');
    expect(rangeFor('1h', now).from.toISOString()).toBe(
      '2025-12-31T23:00:00.000Z',
    );
    expect(
      validateRange(new Date('2026-01-01'), new Date('2026-02-02')),
    ).toContain('30 days');
  });
});
