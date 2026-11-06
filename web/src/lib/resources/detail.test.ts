import { describe, expect, it } from 'vitest';
import {
  currentTabName,
  ratePair,
  relativeDirection,
  resourceTabs,
} from './detail';

function series(values: number[], stepMs = 60_000) {
  return values.map((value, index) => ({ at: index * stepMs, value }));
}

describe('resourceTabs', () => {
  it('orders the live tabs as the design spec does', () => {
    expect(resourceTabs(false, 3).map((tab) => tab.label)).toEqual([
      'Metrics',
      'Containers',
      'Incidents',
      'Changes',
      'Logs',
    ]);
  });

  it('keeps only history tabs for archived resources', () => {
    expect(resourceTabs(true, 0).map((tab) => tab.id)).toEqual([
      'metrics',
      'changes',
    ]);
  });

  it('maps retired tab names', () => {
    expect(currentTabName('alerts')).toBe('incidents');
    expect(currentTabName('activity')).toBe('changes');
    expect(currentTabName('logs')).toBeNull();
  });
});

describe('relativeDirection', () => {
  it('needs ten minutes of samples', () => {
    expect(relativeDirection(series([1, 2, 3], 1_000)).text).toBe(
      'not enough history',
    );
  });

  it('reads growth relative to the mean', () => {
    const mib = 1024 ** 2;
    expect(
      relativeDirection(
        series(Array.from({ length: 20 }, (_, i) => (100 + i * 3) * mib)),
      ).text,
    ).toBe('rising');
    expect(
      relativeDirection(series(Array.from({ length: 20 }, () => 400 * mib)))
        .text,
    ).toBe('steady');
    expect(
      relativeDirection(
        series(Array.from({ length: 20 }, (_, i) => (200 - i * 5) * mib)),
      ).text,
    ).toBe('falling');
  });
});

describe('ratePair', () => {
  it('names both directions without dangling arrows (UI-18)', () => {
    const pair = ratePair(1024, 4 * 1024 ** 2, ['in', 'out']);
    expect(pair.secondary).toMatch(/^in · .+ out$/);
    expect(pair.secondary).not.toContain('↓');
    expect(ratePair(null, null, ['read', 'written'])).toEqual({
      value: '—',
      secondary: 'read · — written',
    });
  });
});
