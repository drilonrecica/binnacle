/** The one missing-value glyph, used wherever a reading is absent (UI-18). */
export const missing = '—';

/**
 * Intl formatters are costly to build and cheap to use, and live pages
 * format hundreds of values on every snapshot, so each locale and option set
 * is built once and reused.
 */
const numberFormats = new Map<string, Intl.NumberFormat>();
const dateFormats = new Map<string, Intl.DateTimeFormat>();

export function numberFormat(
  locale: string | undefined,
  options: Intl.NumberFormatOptions,
): Intl.NumberFormat {
  const key = `${locale ?? ''}|${JSON.stringify(options)}`;
  let format = numberFormats.get(key);
  if (!format) {
    format = new Intl.NumberFormat(locale, options);
    numberFormats.set(key, format);
  }
  return format;
}

export function dateFormat(
  locale: string | undefined,
  options: Intl.DateTimeFormatOptions,
): Intl.DateTimeFormat {
  const key = `${locale ?? ''}|${JSON.stringify(options)}`;
  let format = dateFormats.get(key);
  if (!format) {
    format = new Intl.DateTimeFormat(locale, options);
    dateFormats.set(key, format);
  }
  return format;
}

const formatter = (locale?: string) =>
  numberFormat(locale, { maximumFractionDigits: 1 });
export function formatNumber(
  value: number | null | undefined,
  locale?: string,
) {
  return value == null ? missing : formatter(locale).format(value);
}
export function formatBytes(value: number | null | undefined, locale?: string) {
  if (value == null) return missing;
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let index = 0;
  while (Math.abs(value) >= 1024 && index < units.length - 1) {
    value /= 1024;
    index++;
  }
  return `${formatter(locale).format(value)} ${units[index]}`;
}
export function formatRate(value: number | null | undefined, locale?: string) {
  return value == null ? missing : `${formatBytes(value, locale)}/s`;
}
