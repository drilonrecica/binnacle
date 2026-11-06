import { describe, expect, it } from 'vitest';
import type { HistoricalEvent } from '../api/events';
import type { LiveEvent } from '../live.svelte';
import { groupRepeats, mergeChanges, repeatText } from './changes';

function stored(
  id: string,
  ts: string,
  type = 'container_restart',
): HistoricalEvent {
  return {
    id,
    ts,
    type,
    severity: 'warning',
    summary: `${type} ${id}`,
    resourceId: 'res1',
    source: 'docker',
  };
}

describe('changes', () => {
  it('shows a live event and its stored copy once', () => {
    const live: LiveEvent[] = [
      {
        id: 'E3',
        seq: 3,
        at: '2026-11-06T12:00:00Z',
        type: 'container_oom',
        message: 'worker was OOM killed',
        resourceId: 'res1',
      },
    ];
    const rows = mergeChanges(live, [
      stored('E3', '2026-11-06T12:00:00Z', 'container_oom'),
      stored('E1', '2026-11-06T11:00:00Z'),
    ]);
    expect(rows.map((row) => row.id)).toEqual(['E3', 'E1']);
    expect(rows[0]?.live).toBe(false);
  });

  it('groups repeats of one type on one resource', () => {
    const rows = mergeChanges(
      [],
      [
        stored('E4', '2026-11-06T12:10:00Z'),
        stored('E3', '2026-11-06T12:05:00Z', 'container_oom'),
        stored('E2', '2026-11-06T12:04:00Z'),
        stored('E1', '2026-11-06T12:00:00Z'),
        // More than an hour before the newest restart: a separate entry.
        stored('E0', '2026-11-06T10:00:00Z'),
      ],
    );
    const groups = groupRepeats(rows);
    expect(groups.map((group) => [group.latest.id, group.count])).toEqual([
      ['E4', 3],
      ['E3', 1],
      ['E0', 1],
    ]);
    expect(groups[0] && repeatText(groups[0])).toBe('3× in 10m');
    expect(groups[1] && repeatText(groups[1])).toBe('');
  });

  it('keeps at most the limit of entries', () => {
    const rows = mergeChanges(
      [],
      Array.from({ length: 30 }, (_, i) =>
        stored(
          `E${i}`,
          new Date(Date.UTC(2026, 10, 6, 12, i)).toISOString(),
          `t${i}`,
        ),
      ),
    );
    expect(groupRepeats(rows, 10)).toHaveLength(10);
  });
});
