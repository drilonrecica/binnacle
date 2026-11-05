/** Geometry and text helpers shared by the Gauge, Scale and Sparkline. */

/** Clamps a reading into [min, max]; null for missing or non-finite values. */
export function clampReading(
  value: number | null | undefined,
  min = 0,
  max = 100,
): number | null {
  if (value == null || !Number.isFinite(value)) return null;
  return Math.min(max, Math.max(min, value));
}

/** Fraction (0..1) of the way from min to max. */
export function fraction(value: number, min = 0, max = 100): number {
  const span = max - min;
  if (span <= 0) return 0;
  return Math.min(1, Math.max(0, (value - min) / span));
}

/**
 * Point on a 180-degree arc that opens downward from the left end (fraction
 * 0) over the top to the right end (fraction 1), centred on (cx, cy).
 */
export function arcPoint(
  cx: number,
  cy: number,
  radius: number,
  at: number,
): { x: number; y: number } {
  const angle = Math.PI * (1 - at);
  return {
    x: round(cx + radius * Math.cos(angle)),
    y: round(cy - radius * Math.sin(angle)),
  };
}

/** SVG path for the arc from fraction `from` to fraction `to`. */
export function arcPath(
  cx: number,
  cy: number,
  radius: number,
  from: number,
  to: number,
): string {
  const start = arcPoint(cx, cy, radius, from);
  const end = arcPoint(cx, cy, radius, to);
  // A half circle at most, so the large-arc flag is always 0; sweep 1 runs
  // clockwise on screen, left to right over the top.
  return `M${start.x} ${start.y} A${radius} ${radius} 0 0 1 ${end.x} ${end.y}`;
}

function round(value: number): number {
  return Math.round(value * 100) / 100;
}

export interface SeriesSummary {
  min: number;
  max: number;
  avg: number;
  now: number;
}

/** Min, average, max and latest of the finite values; null when there are none. */
export function summarize(
  values: ReadonlyArray<number | null | undefined>,
): SeriesSummary | null {
  let min = Infinity;
  let max = -Infinity;
  let sum = 0;
  let count = 0;
  let now: number | null = null;
  for (const value of values) {
    if (value == null || !Number.isFinite(value)) continue;
    min = Math.min(min, value);
    max = Math.max(max, value);
    sum += value;
    count += 1;
    now = value;
  }
  if (!count || now == null) return null;
  return { min, max, avg: sum / count, now };
}

/** Screen-reader sentence for a series: "CPU: now 42%, minimum 3%, …". */
export function describeSeries(
  label: string,
  values: ReadonlyArray<number | null | undefined>,
  format: (value: number) => string,
): string {
  const summary = summarize(values);
  if (!summary) return `${label}: no measurements.`;
  return `${label}: now ${format(summary.now)}, minimum ${format(summary.min)}, average ${format(summary.avg)}, maximum ${format(summary.max)}.`;
}
