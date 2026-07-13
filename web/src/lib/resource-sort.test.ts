import { describe, expect, it } from 'vitest';
import {
  defaultDirection,
  loadSortPreference,
  resourceStatusLabel,
  saveSortPreference,
  sortResources,
  type ResourceSortField,
} from './resource-sort';

const resources = [
  {
    id: 'res_b',
    name: 'Beta',
    status: 'unknown',
    context: 'prod',
    cpuHostPct: 2,
    memoryBytes: 20,
    components: [{ healthStatus: 'starting' }, {}],
    archivedAt: '2026-07-12T00:00:00Z',
  },
  {
    id: 'res_a',
    name: 'Alpha',
    status: 'healthy',
    context: 'stage',
    cpuHostPct: 1,
    memoryBytes: 10,
    components: [{}],
    archivedAt: '2026-07-11T00:00:00Z',
  },
];

describe('resource sorting', () => {
  const expectations: Array<[ResourceSortField, string, string]> = [
    ['name', 'res_a', 'res_b'],
    ['state', 'res_a', 'res_b'],
    ['cpu', 'res_a', 'res_b'],
    ['memory', 'res_a', 'res_b'],
    ['context', 'res_b', 'res_a'],
    ['components', 'res_a', 'res_b'],
    ['archived', 'res_a', 'res_b'],
  ];

  it.each(expectations)('sorts %s in both directions', (field, first, last) => {
    expect(
      sortResources(resources, { field, direction: 'asc' }).map(
        (resource) => resource.id,
      ),
    ).toEqual([first, last]);
    expect(
      sortResources(resources, { field, direction: 'desc' }).map(
        (resource) => resource.id,
      ),
    ).toEqual([last, first]);
  });

  it('keeps missing values last in either direction', () => {
    const missing = { id: 'res_missing', name: 'Missing' };
    for (const direction of ['asc', 'desc'] as const)
      expect(
        sortResources([...resources, missing], { field: 'cpu', direction }).at(
          -1,
        )?.id,
      ).toBe('res_missing');
  });

  it('uses name and ID as deterministic tie-breakers without mutating input', () => {
    const input = [
      { id: 'res_2', name: 'Same', cpuHostPct: 1 },
      { id: 'res_1', name: 'Same', cpuHostPct: 1 },
      { id: 'res_3', name: 'Alpha', cpuHostPct: 1 },
    ];
    const result = sortResources(input, { field: 'cpu', direction: 'desc' });
    expect(result.map((resource) => resource.id)).toEqual([
      'res_3',
      'res_1',
      'res_2',
    ]);
    expect(input.map((resource) => resource.id)).toEqual([
      'res_2',
      'res_1',
      'res_3',
    ]);
  });

  it('presents starting health without changing normalized state', () => {
    expect(resourceStatusLabel(resources[0])).toBe('starting');
    expect(resources[0].status).toBe('unknown');
  });
});

describe('resource sort preferences', () => {
  it('defaults numeric fields descending and text fields ascending', () => {
    expect(defaultDirection('cpu')).toBe('desc');
    expect(defaultDirection('archived')).toBe('desc');
    expect(defaultDirection('state')).toBe('asc');
  });

  it('stores active and archived preferences separately', () => {
    const values = new Map<string, string>();
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => {
        values.set(key, value);
      },
    } as unknown as Storage;
    saveSortPreference('active', { field: 'cpu', direction: 'desc' }, storage);
    saveSortPreference(
      'archived',
      { field: 'archived', direction: 'asc' },
      storage,
    );
    expect(loadSortPreference('active', storage)).toEqual({
      field: 'cpu',
      direction: 'desc',
    });
    expect(loadSortPreference('archived', storage)).toEqual({
      field: 'archived',
      direction: 'asc',
    });
  });

  it('ignores invalid stored preferences', () => {
    const storage = {
      getItem: () => '{"field":"cpu","direction":"sideways"}',
    } as unknown as Storage;
    expect(loadSortPreference('active', storage)).toEqual({
      field: 'name',
      direction: 'asc',
    });
  });
});
