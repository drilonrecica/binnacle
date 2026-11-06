import type { HistoricalEvent } from '../api/events';
import type { LiveEvent } from '../live.svelte';
import { formatSpan } from '../ui/relative-time';

export interface ChangeRow {
  /** Event id: live events carry the id they are stored under. */
  id: string;
  at: string;
  type: string;
  severity: string;
  message: string;
  resourceId?: string;
  live: boolean;
}

/** What repeat grouping needs from a row. */
export interface RepeatRow {
  id: string;
  at: string;
  type: string;
  resourceId?: string;
}

export interface ChangeGroup<T extends RepeatRow = ChangeRow> {
  key: string;
  /** The newest event of the group. */
  latest: T;
  /** Every event of the group, newest first; members[0] is latest. */
  members: T[];
  count: number;
  /** The oldest event's time; equals latest.at for a single event. */
  firstAt: string;
}

/** Repeats further apart than this start a new group. */
export const repeatWindowMs = 3_600_000;

/**
 * Live events and history in one newest-first list. A live event and its
 * stored copy share the id, so each appears once.
 */
export function mergeChanges(
  live: LiveEvent[],
  history: HistoricalEvent[],
): ChangeRow[] {
  const rows = new Map<string, ChangeRow>();
  for (const event of history)
    rows.set(event.id, {
      id: event.id,
      at: event.ts,
      type: event.type,
      severity: event.severity,
      message: event.summary,
      resourceId: event.resourceId,
      live: false,
    });
  for (const event of live)
    rows.set(event.id, {
      id: event.id,
      at: event.at ?? new Date().toISOString(),
      type: event.type,
      severity: event.severity ?? 'info',
      message: event.message,
      resourceId: event.resourceId,
      live: !rows.has(event.id),
    });
  return [...rows.values()].sort(
    (a, b) => Date.parse(b.at) - Date.parse(a.at) || b.id.localeCompare(a.id),
  );
}

/**
 * Folds repeats of one event type on one resource into a single entry
 * ("restarted · 3× in 10m"), while each repeat is within the window of the
 * group's newest event. Input and output are newest first.
 */
export function groupRepeats<T extends RepeatRow>(
  rows: T[],
  limit = 10,
  windowMs = repeatWindowMs,
): ChangeGroup<T>[] {
  const groups: ChangeGroup<T>[] = [];
  const open = new Map<string, ChangeGroup<T>>();
  for (const row of rows) {
    const key = `${row.type}|${row.resourceId ?? ''}`;
    const group = open.get(key);
    if (group && Date.parse(group.latest.at) - Date.parse(row.at) <= windowMs) {
      group.count += 1;
      group.members.push(row);
      group.firstAt = row.at;
      continue;
    }
    if (groups.length >= limit) {
      // A full list only grows existing groups; skip new ones.
      open.delete(key);
      continue;
    }
    const next: ChangeGroup<T> = {
      key: `${key}|${row.id}`,
      latest: row,
      members: [row],
      count: 1,
      firstAt: row.at,
    };
    groups.push(next);
    open.set(key, next);
  }
  return groups;
}

/** "3× in 10m" for a group, or "" for a single event. */
export function repeatText(group: ChangeGroup<RepeatRow>): string {
  if (group.count < 2) return '';
  return `${group.count}× in ${formatSpan(group.firstAt, group.latest.at)}`;
}
