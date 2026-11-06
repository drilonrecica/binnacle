/** The one missing-value glyph, used wherever a reading is absent (UI-18). */
export const missing = '—';

const formatter = (locale?: string) =>
  new Intl.NumberFormat(locale, { maximumFractionDigits: 1 });
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
