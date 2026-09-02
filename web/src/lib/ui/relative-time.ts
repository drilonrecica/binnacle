const units: Array<[Intl.RelativeTimeFormatUnit, number]> = [
  ['year', 365 * 86_400_000],
  ['month', 30 * 86_400_000],
  ['week', 7 * 86_400_000],
  ['day', 86_400_000],
  ['hour', 3_600_000],
  ['minute', 60_000],
];

/** Compact relative time such as "3m ago", "in 2h", or "just now". */
export function formatRelative(
  value: string | number | Date,
  now = Date.now(),
): string {
  const at =
    value instanceof Date ? value.getTime() : new Date(value).getTime();
  if (!Number.isFinite(at)) return '—';
  const delta = at - now;
  const abs = Math.abs(delta);
  // Live events can carry timestamps slightly ahead of the local clock.
  if (abs < 45_000 || (delta > 0 && delta < 90_000)) return 'just now';
  const short: Record<string, string> = {
    year: 'y',
    month: 'mo',
    week: 'w',
    day: 'd',
    hour: 'h',
    minute: 'm',
  };
  for (const [unit, size] of units) {
    if (abs >= size) {
      const count = Math.round(abs / size);
      return delta < 0
        ? `${count}${short[unit]} ago`
        : `in ${count}${short[unit]}`;
    }
  }
  return 'just now';
}

/** Duration between two instants such as "2h 15m" or "45s". */
export function formatSpan(
  from: string | number | Date,
  to: string | number | Date = Date.now(),
): string {
  const start = new Date(from).getTime();
  const end = new Date(to).getTime();
  if (!Number.isFinite(start) || !Number.isFinite(end)) return '—';
  const seconds = Math.max(0, Math.floor((end - start) / 1000));
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  if (days) return `${days}d ${hours}h`;
  if (hours) return `${hours}h ${minutes}m`;
  if (minutes) return `${minutes}m`;
  return `${seconds}s`;
}

export function formatAbsolute(
  value: string | number | Date,
  locale?: string,
): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return '—';
  return new Intl.DateTimeFormat(locale, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}

export function formatClock(
  value: string | number | Date,
  locale?: string,
): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return '—';
  return new Intl.DateTimeFormat(locale, { timeStyle: 'medium' }).format(date);
}
