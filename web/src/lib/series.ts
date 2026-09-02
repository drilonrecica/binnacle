/** Pure helpers for sparkline data. Kept DOM-free so they are unit-tested. */

export interface Sample {
  at: number;
  value: number | null;
}

/** Averages samples into `slots` equal buckets between `from` and `to`. */
export function bucketize(
  samples: Sample[],
  from: number,
  to: number,
  slots: number,
): Array<number | null> {
  const out: Array<number | null> = Array(slots).fill(null);
  if (to <= from || slots < 1) return out;
  const sums = Array(slots).fill(0);
  const counts = Array(slots).fill(0);
  const width = (to - from) / slots;
  for (const sample of samples) {
    if (sample.value == null || !Number.isFinite(sample.value)) continue;
    if (sample.at < from || sample.at > to) continue;
    const index = Math.min(slots - 1, Math.floor((sample.at - from) / width));
    sums[index] += sample.value;
    counts[index] += 1;
  }
  for (let index = 0; index < slots; index++) {
    if (counts[index]) out[index] = sums[index] / counts[index];
  }
  return out;
}

/**
 * Extends a server sparkline with live samples that arrived after its `to`
 * timestamp, keeping the array length fixed by dropping the oldest slots.
 */
export function mergeSparkline(
  base: Array<number | null>,
  stepSeconds: number,
  toMs: number,
  live: Sample[],
): Array<number | null> {
  const stepMs = stepSeconds * 1000;
  const newer = live.filter((sample) => sample.at > toMs);
  if (!newer.length || stepMs <= 0) return base;
  const last = newer[newer.length - 1].at;
  const extraSlots = Math.max(1, Math.ceil((last - toMs) / stepMs));
  const extra = bucketize(newer, toMs, toMs + extraSlots * stepMs, extraSlots);
  const merged = [...base, ...extra];
  return merged.slice(-base.length);
}

/** Difference between the newest and the oldest defined value. */
export function delta(values: Array<number | null>): number | null {
  const defined = values.filter((value): value is number => value != null);
  if (defined.length < 2) return null;
  return defined[defined.length - 1] - defined[0];
}

export function lastDefined(values: Array<number | null>): number | null {
  for (let index = values.length - 1; index >= 0; index--) {
    const value = values[index];
    if (value != null) return value;
  }
  return null;
}
