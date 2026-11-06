import { describe, expect, it } from 'vitest';
import {
  annotationTypeFilter,
  boundAnnotations,
  isAnnotatedEvent,
} from './annotations';
import { eventTypes } from './event-types.generated';

describe('chart annotations', () => {
  it('keeps boundary events and aggregates overlapping markers', () => {
    const from = new Date('2026-07-11T11:00:00Z');
    const to = new Date('2026-07-11T12:00:00Z');
    const markers = boundAnnotations(
      [
        { ts: from.toISOString(), type: 'deployment', summary: 'Deploy' },
        {
          ts: '2026-07-11T11:00:01Z',
          type: 'container_start',
          summary: 'Start',
        },
        {
          ts: to.toISOString(),
          type: 'container_oom',
          summary: 'OOM',
          resourceId: 'res_1',
        },
        { ts: '2026-07-11T11:30:00Z', type: 'unrelated', summary: 'Ignored' },
      ],
      from,
      to,
      2,
    );
    expect(markers).toHaveLength(2);
    expect(markers[0]!.count).toBe(2);
    expect(markers[1]!.href).toBe('/resources/res_1');
  });
});

describe('annotation event types', () => {
  it('listen for the types the backend writes', () => {
    for (const type of [
      'host_reboot',
      'container_oom',
      'container_restart',
      'deployment',
      'collector_degraded',
      'persistence_gap',
      'persistence_emergency',
    ])
      expect(isAnnotatedEvent(type)).toBe(true);
  });

  it('ignore types the backend never writes and bookkeeping events', () => {
    for (const type of [
      'boot',
      'oom',
      'replacement',
      'collector_state',
      'collector_failure',
      'alert_triggered',
      'container_health_status_change',
    ])
      expect(isAnnotatedEvent(type)).toBe(false);
  });

  it('only annotate types from the backend catalogue', () => {
    const known = new Set<string>(eventTypes);
    for (const type of annotationTypeFilter.split(','))
      expect(known.has(type)).toBe(true);
    // The events API accepts at most 32 types per query.
    expect(annotationTypeFilter.split(',').length).toBeLessThanOrEqual(32);
  });
});
