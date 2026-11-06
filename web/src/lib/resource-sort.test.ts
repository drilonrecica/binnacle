import { describe, expect, it } from 'vitest';
import { resourceStatusLabel } from './resource-sort';
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

describe('resourceStatusLabel', () => {
  it('presents starting health without changing normalized state', () => {
    expect(resourceStatusLabel(resources[0]!)).toBe('starting');
    expect(resources[0]!.status).toBe('unknown');
  });
});
