import { describe, expect, it } from 'vitest';
import type { HistoricalEvent } from '../api/events';
import { groupTimeline, liveRow, mergeTimeline } from './timeline';

function stored(id: string, minute: number, type = 'container_restart') {
  return {
    id,
    ts: new Date(Date.UTC(2026, 10, 6, 12, minute)).toISOString(),
    type,
    severity: 'warning',
    summary: `${type} ${id}`,
    resourceId: 'res1',
    source: 'docker',
  } satisfies HistoricalEvent;
}

describe('Changes timeline (UI-14)', () => {
  it('keeps the server id of a live event', () => {
    const row = liveRow({
      id: '01J0LIVE',
      seq: 7,
      at: '2026-11-06T12:00:00Z',
      type: 'container_oom',
      message: 'worker was OOM killed',
      severity: 'bogus',
    });
    expect(row.id).toBe('01J0LIVE');
    expect(row.severity).toBe('info');
  });

  it('shows a live event and its stored copy once, preferring the stored copy', () => {
    const copy = stored('E2', 2);
    const live = { ...copy, source: 'live', summary: 'from the stream' };
    const { rows } = mergeTimeline([live], [copy, stored('E1', 1)]);
    expect(rows.map((row) => row.id)).toEqual(['E2', 'E1']);
    expect(rows[0]?.source).toBe('docker');
    // The other way round as well: history first, live arriving later.
    const again = mergeTimeline(rows, [live]).rows;
    expect(again.map((row) => [row.id, row.source])).toEqual([
      ['E2', 'docker'],
      ['E1', 'docker'],
    ]);
  });

  it('keeps the list newest first and cut at the cap', () => {
    const page = Array.from({ length: 8 }, (_, i) => stored(`E${i}`, i));
    const { rows, capped } = mergeTimeline([], page, 5);
    expect(capped).toBe(true);
    expect(rows.map((row) => row.id)).toEqual(['E7', 'E6', 'E5', 'E4', 'E3']);
    expect(mergeTimeline([], page.slice(0, 5), 5).capped).toBe(false);
  });

  it('folds repeats and keeps the members for expansion', () => {
    const { rows } = mergeTimeline(
      [],
      [stored('E3', 10), stored('E2', 5, 'container_oom'), stored('E1', 0)],
    );
    const groups = groupTimeline(rows);
    expect(
      groups.map((group) => [
        group.latest.id,
        group.members.map((member) => member.id),
      ]),
    ).toEqual([
      ['E3', ['E3', 'E1']],
      ['E2', ['E2']],
    ]);
  });
});
