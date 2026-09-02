import { describe, expect, it } from 'vitest';
import { rankItems, scoreMatch, type PaletteItem } from './palette';

describe('scoreMatch', () => {
  it('prefers exact, then prefix, then word starts, then substrings', () => {
    expect(scoreMatch('host', 'host')).toBeGreaterThan(
      scoreMatch('host', 'host metrics'),
    );
    expect(scoreMatch('host', 'host metrics')).toBeGreaterThan(
      scoreMatch('host', 'open host'),
    );
    expect(scoreMatch('host', 'open host')).toBeGreaterThan(
      scoreMatch('host', 'ghostly'),
    );
  });

  it('accepts subsequence matches and rejects the rest', () => {
    expect(scoreMatch('apr', 'api.production')).toBeGreaterThan(0);
    expect(scoreMatch('zzz', 'api.production')).toBe(0);
  });

  it('treats an empty query as a match', () => {
    expect(scoreMatch('', 'anything')).toBe(1);
  });
});

describe('rankItems', () => {
  const items: PaletteItem[] = [
    {
      id: 'overview',
      group: 'Pages',
      label: 'Overview',
      keywords: ['home', 'dashboard'],
    },
    {
      id: 'host',
      group: 'Pages',
      label: 'Host',
      keywords: ['server', 'processes'],
    },
    {
      id: 'res1',
      group: 'Resources',
      label: 'api.production',
      hint: 'binnacle/production',
    },
    {
      id: 'res2',
      group: 'Resources',
      label: 'worker.production',
      hint: 'binnacle/production',
    },
  ];

  it('returns the first items in order for an empty query', () => {
    expect(rankItems(items, '').map((item) => item.id)).toEqual([
      'overview',
      'host',
      'res1',
      'res2',
    ]);
  });

  it('matches keywords', () => {
    expect(rankItems(items, 'server')[0].id).toBe('host');
  });

  it('ranks label matches above hint matches', () => {
    expect(rankItems(items, 'api')[0].id).toBe('res1');
    expect(rankItems(items, 'production').map((item) => item.id)).toEqual([
      'res1',
      'res2',
    ]);
  });

  it('honours the limit', () => {
    expect(rankItems(items, '', 2)).toHaveLength(2);
  });
});
