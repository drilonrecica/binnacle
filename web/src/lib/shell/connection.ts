import type { ConnectionState } from '../live.svelte';
import { formatClock, formatSpan } from '../ui/relative-time';

/** Snapshots arrive every 2 s; five missed ones make the data stale. */
export const staleAfterMs = 10_000;

export type ConnectionTone = 'ok' | 'warn' | 'critical' | 'neutral';

export interface ConnectionView {
  tone: ConnectionTone;
  /**
   * The state word. It is the only text inside the live region, so it must
   * not change while the state holds (no clock, no counter).
   */
  label: string;
  /** Since when the data has been old, e.g. "since 11:32"; static. */
  since: string;
  /** How old the last update is, e.g. "2m"; ticks, so kept out of the live region. */
  age: string;
  /** Long description for the tooltip. */
  detail: string;
}

/**
 * One description of the live connection for the shell's status region:
 * live, stale (connected but quiet), connecting, disconnected, or signed out.
 */
export function connectionView(
  state: ConnectionState,
  lastReceivedAt: string | null,
  now = Date.now(),
): ConnectionView {
  const last = lastReceivedAt ? Date.parse(lastReceivedAt) : Number.NaN;
  const known = Number.isFinite(last);
  const quiet = known && now - last > staleAfterMs;
  const since = known ? `since ${formatClock(last)}` : '';
  const age = known ? formatSpan(last, now) : '';
  const lastLine = known
    ? `Last update ${formatClock(last)}`
    : 'No update received yet';

  if (state === 'connected' && !quiet)
    return { tone: 'ok', label: 'Live', since: '', age: '', detail: lastLine };
  if (state === 'connected')
    return {
      tone: 'warn',
      label: 'Stale',
      since,
      age,
      detail: `Connected, but no update for ${age}. ${lastLine}.`,
    };
  if (state === 'connecting')
    return {
      tone: 'warn',
      label: 'Connecting',
      since,
      age,
      detail: `Connecting to live updates. ${lastLine}.`,
    };
  if (state === 'unauthorized')
    return {
      tone: 'critical',
      label: 'Signed out',
      since,
      age,
      detail: 'Your session ended.',
    };
  return {
    tone: 'critical',
    label: 'Disconnected',
    since,
    age,
    detail: `Live updates are disconnected. ${lastLine}.`,
  };
}
