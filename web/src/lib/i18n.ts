export const primitiveMessages = {
  close: 'Close', loading: 'Loading…', details: 'Technical details',
  'shell.skip': 'Skip to content', 'shell.access': 'Checking access…', 'shell.live': 'Live monitoring',
  unavailable: 'Unavailable', stale: 'Stale', 'overview.empty': 'No current observations are available.',
} as const;
export type MessageKey = keyof typeof primitiveMessages;
export function t(key: MessageKey): string { return primitiveMessages[key]; }

const formatter = (locale?: string) => new Intl.NumberFormat(locale, { maximumFractionDigits: 1 });
export function formatNumber(value: number | null | undefined, locale?: string) { return value == null ? t('unavailable') : formatter(locale).format(value); }
export function formatBytes(value: number | null | undefined, locale?: string) { if (value == null) return t('unavailable'); const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']; let index = 0; while (Math.abs(value) >= 1024 && index < units.length - 1) { value /= 1024; index++; } return `${formatter(locale).format(value)} ${units[index]}`; }
export function formatRate(value: number | null | undefined, locale?: string) { return value == null ? t('unavailable') : `${formatBytes(value, locale)}/s`; }
export function formatDate(value: string | Date | null | undefined, locale?: string) { return value == null ? t('unavailable') : new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value)); }
export function formatDuration(seconds: number | null | undefined, locale?: string) { return seconds == null ? t('unavailable') : new Intl.NumberFormat(locale, { style: 'unit', unit: seconds >= 3600 ? 'hour' : seconds >= 60 ? 'minute' : 'second', maximumFractionDigits: 1 }).format(seconds >= 3600 ? seconds / 3600 : seconds >= 60 ? seconds / 60 : seconds); }
