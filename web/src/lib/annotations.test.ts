import { describe, expect, it } from 'vitest';
import { boundAnnotations } from './annotations';

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
    expect(markers[0].count).toBe(2);
    expect(markers[1].href).toBe('/resources/res_1');
  });
});
