import { afterEach, describe, expect, it, vi } from 'vitest';
import { eventTypes } from '../event-types.generated';
import {
  eventFamilies,
  eventFamily,
  familyTypes,
  isEventFamily,
  listEvents,
} from './events';

afterEach(() => vi.unstubAllGlobals());

describe('event families (UI-14)', () => {
  it('gives every catalogue type exactly one family', () => {
    for (const type of eventTypes) {
      const owners = eventFamilies.filter((family) =>
        familyTypes(family.value).includes(type),
      );
      expect(owners.map((family) => family.value)).toEqual([eventFamily(type)]);
    }
  });

  it('matches by the table, never by a type prefix', () => {
    // Was matched as a container event by prefix while the history query
    // filed it under deployments, so the two views disagreed.
    expect(eventFamily('container_replacement')).toBeUndefined();
    expect(eventFamily('alert_something_new')).toBeUndefined();
    expect(eventFamily('container_oom')).toBe('health');
    expect(eventFamily('deployment')).toBe('deploys');
  });

  it('offers the six design families', () => {
    expect(eventFamilies.map((family) => family.value)).toEqual([
      'deploys',
      'lifecycle',
      'health',
      'collectors',
      'alerts',
      'system',
    ]);
    expect(isEventFamily('containers')).toBe(false);
    expect(isEventFamily('toString')).toBe(false);
  });

  it('expands a family into the same types the page matches live events by', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response('[]', {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    vi.stubGlobal('fetch', fetcher);
    await listEvents({
      from: new Date('2026-11-06T00:00:00Z'),
      to: new Date('2026-11-06T01:00:00Z'),
      family: 'health',
    });
    const url = new URL(String(fetcher.mock.calls[0]?.[0]), 'http://x');
    const sent = url.searchParams.get('type')?.split(',') ?? [];
    expect(sent).toEqual(['container_oom', 'container_health_status_change']);
    expect(sent.every((type) => eventFamily(type) === 'health')).toBe(true);
  });
});
