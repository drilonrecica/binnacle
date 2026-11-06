/**
 * The local "seen by" marker for incidents (UI-21). Binnacle has no
 * acknowledgement workflow; this lets whoever is looking note "someone is on
 * it" in this browser. It is never sent to the server, so it is shown with
 * the words "this browser" wherever it appears.
 */

export interface SeenMark {
  by: string;
  /** ISO time the mark was set. */
  at: string;
}

export const seenStorageKey = 'binnacle.incidents.seen.v1';
/** Marks kept at most; the oldest are dropped first. */
export const seenLimit = 200;

/** Parses stored marks, dropping anything malformed. */
export function parseSeen(raw: string | null): Record<string, SeenMark> {
  if (!raw) return {};
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    return {};
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  const marks: Record<string, SeenMark> = {};
  for (const [id, mark] of Object.entries(value as Record<string, unknown>)) {
    if (!mark || typeof mark !== 'object') continue;
    const { by, at } = mark as Record<string, unknown>;
    if (typeof by !== 'string' || typeof at !== 'string') continue;
    if (!Number.isFinite(Date.parse(at))) continue;
    marks[id] = { by, at };
  }
  return marks;
}

/** Keeps the newest `limit` marks so storage stays bounded. */
export function pruneSeen(
  marks: Record<string, SeenMark>,
  limit = seenLimit,
): Record<string, SeenMark> {
  const entries = Object.entries(marks);
  if (entries.length <= limit) return marks;
  entries.sort(([, a], [, b]) => Date.parse(b.at) - Date.parse(a.at));
  return Object.fromEntries(entries.slice(0, limit));
}

/** A new mark for `by` (an empty name reads as "you"). */
export function newSeenMark(by: string, now = Date.now()): SeenMark {
  return { by: by || 'you', at: new Date(now).toISOString() };
}
