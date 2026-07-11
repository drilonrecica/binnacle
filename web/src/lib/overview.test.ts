import { describe, expect, it } from 'vitest';
import { prioritizedResources, staleResource } from './overview';

describe('production overview prioritization', () => {
  it('places unhealthy resources first and identifies stale values', () => {
    const values = prioritizedResources([
      { id: 'a', name: 'Healthy', status: 'healthy' },
      { id: 'b', name: 'Broken', status: 'down' },
    ]);
    expect(values[0].name).toBe('Broken');
    expect(
      staleResource(
        {
          id: 'a',
          name: 'A',
          status: 'healthy',
          lastSeenAt: '2026-07-11T11:59:00Z',
        },
        '2026-07-11T12:00:00Z',
      ),
    ).toBe(true);
  });
});
