/** Parsing and formatting for Go-style durations and byte sizes. */

const durationUnits: Record<string, number> = {
  ms: 1,
  s: 1000,
  m: 60_000,
  h: 3_600_000,
};

/** Parses `1h30m`, `45s`, `500ms`; returns milliseconds or null. */
export function parseGoDuration(input: string): number | null {
  const value = input.trim();
  if (!value) return null;
  const pattern = /(\d+(?:\.\d+)?)(ms|s|m|h)/g;
  let total = 0;
  let consumed = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(value))) {
    if (match.index !== consumed) return null;
    total += Number(match[1]) * durationUnits[match[2]];
    consumed = match.index + match[0].length;
  }
  return consumed === value.length && consumed > 0 ? total : null;
}

/** Formats milliseconds as the shortest exact Go duration (`48h`, `2m30s`). */
export function formatGoDuration(ms: number): string {
  if (ms <= 0) return '0s';
  const parts: string[] = [];
  let rest = ms;
  for (const [unit, size] of [
    ['h', 3_600_000],
    ['m', 60_000],
    ['s', 1000],
    ['ms', 1],
  ] as const) {
    const count = Math.floor(rest / size);
    if (count > 0) {
      parts.push(`${count}${unit}`);
      rest -= count * size;
    }
  }
  return parts.join('');
}

/** Human phrasing such as "2 days", "30 minutes", "5 seconds". */
export function humanDuration(input: string): string {
  const ms = parseGoDuration(input);
  if (ms == null) return input;
  const units: Array<[number, string]> = [
    [86_400_000, 'day'],
    [3_600_000, 'hour'],
    [60_000, 'minute'],
    [1000, 'second'],
  ];
  for (const [size, name] of units) {
    if (ms >= size) {
      const count = Math.round((ms / size) * 10) / 10;
      return `${count} ${name}${count === 1 ? '' : 's'}`;
    }
  }
  return `${ms} ms`;
}

const byteUnits = ['B', 'KiB', 'MiB', 'GiB', 'TiB'] as const;
export type ByteUnit = (typeof byteUnits)[number];

export function bytesToInput(bytes: number): { value: number; unit: ByteUnit } {
  let unit = 0;
  let value = bytes;
  while (
    value >= 1024 &&
    unit < byteUnits.length - 1 &&
    Number.isInteger(value / 1024)
  ) {
    value /= 1024;
    unit += 1;
  }
  return { value, unit: byteUnits[unit] };
}

export function inputToBytes(value: number, unit: ByteUnit): number | null {
  if (!Number.isFinite(value) || value < 0) return null;
  return Math.round(value * 1024 ** byteUnits.indexOf(unit));
}

export const byteUnitOptions = byteUnits;
