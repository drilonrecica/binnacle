import type { HistoricalEvent } from '../api/events';
import type { LiveEvent } from '../live.svelte';
import { groupRepeats, type ChangeGroup } from '../overview/changes';

/**
 * At most this many events are held on the Changes page, live and history
 * together. Past it the page stops loading older events and says so (UI-14).
 */
export const timelineCap = 1000;

export type TimelineRow = HistoricalEvent & { at: string };
export type TimelineGroup = ChangeGroup<TimelineRow>;

/**
 * A live event as a timeline row. It keeps the server id: a live event and
 * its stored copy share it, so the two are deduped when they meet.
 */
export function liveRow(event: LiveEvent, now = new Date()): HistoricalEvent {
  return {
    id: event.id,
    ts: event.at ?? now.toISOString(),
    type: event.type,
    severity: severityOf(event.severity),
    summary: event.message,
    resourceId: event.resourceId,
    containerInstanceId: event.containerInstanceId,
    source: 'live',
  };
}

function severityOf(value: string | undefined): HistoricalEvent['severity'] {
  return value === 'warning' || value === 'critical' ? value : 'info';
}

function newestFirst(a: HistoricalEvent, b: HistoricalEvent) {
  return (
    Date.parse(b.ts) - Date.parse(a.ts) ||
    (b.id < a.id ? -1 : b.id > a.id ? 1 : 0)
  );
}

/**
 * Merges two newest-first lists into one, each server id once, cut at the
 * cap. A stored copy wins over a live row with the same id, because it
 * carries the details and source the stream does not. `capped` is true when
 * rows were cut, so the page can say the list is not complete.
 */
export function mergeTimeline(
  current: HistoricalEvent[],
  incoming: HistoricalEvent[],
  cap = timelineCap,
): { rows: HistoricalEvent[]; capped: boolean } {
  const byId = new Map<string, HistoricalEvent>();
  for (const row of [...current, ...incoming]) {
    const held = byId.get(row.id);
    if (!held || (held.source === 'live' && row.source !== 'live'))
      byId.set(row.id, row);
  }
  const rows = [...byId.values()].sort(newestFirst);
  return rows.length > cap
    ? { rows: rows.slice(0, cap), capped: true }
    : { rows, capped: false };
}

/** Repeats of one type on one resource folded into one timeline entry. */
export function groupTimeline(rows: HistoricalEvent[]): TimelineGroup[] {
  return groupRepeats(
    rows.map((row) => ({ ...row, at: row.ts })),
    Number.POSITIVE_INFINITY,
  );
}
