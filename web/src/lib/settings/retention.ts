/**
 * Retention tiers as the server applies them, so the Data settings can tell
 * when a save would shorten history and confirm it first (UI-8).
 */
import { parseGoDuration } from './units';

export const tierKeys = [
  'retention.raw',
  'retention.one_minute',
  'retention.fifteen_minute',
  'retention.one_hour',
] as const;
export type TierKey = (typeof tierKeys)[number];

export const tierLabels: Record<TierKey, string> = {
  'retention.raw': 'Raw samples (10s)',
  'retention.one_minute': '1-minute rollups',
  'retention.fifteen_minute': '15-minute rollups',
  'retention.one_hour': 'Hourly rollups',
};

const hour = 3_600_000;
const day = 24 * hour;

/**
 * Milliseconds kept per tier for each preset, mirroring RetentionPreset in
 * internal/settings/settings.go. Zero keeps the tier forever.
 */
const presetTiers: Record<string, Record<TierKey, number>> = {
  minimal: {
    'retention.raw': 12 * hour,
    'retention.one_minute': 7 * day,
    'retention.fifteen_minute': 90 * day,
    'retention.one_hour': 365 * day,
  },
  balanced: {
    'retention.raw': 48 * hour,
    'retention.one_minute': 30 * day,
    'retention.fifteen_minute': 365 * day,
    'retention.one_hour': 0,
  },
  'long-term': {
    'retention.raw': 7 * day,
    'retention.one_minute': 90 * day,
    'retention.fifteen_minute': 730 * day,
    'retention.one_hour': 0,
  },
};

/**
 * The milliseconds each tier keeps for a set of setting values: the preset's
 * tiers unless the preset is advanced, else the tier values themselves. Zero
 * means the tier is never trimmed; null means the value does not parse.
 */
export function effectiveTiers(
  values: Record<string, string | undefined>,
): Record<TierKey, number | null> {
  const preset = presetTiers[values['retention.preset'] ?? ''];
  if (preset) return { ...preset };
  const out = {} as Record<TierKey, number | null>;
  for (const key of tierKeys) out[key] = parseGoDuration(values[key] ?? '');
  return out;
}

export interface Shortening {
  /** Tiers that would keep less history than they do now. */
  tiers: TierKey[];
  /** The shortest new duration among them, in milliseconds. */
  keepMs: number;
}

/** Which tiers a change shortens, or null when nothing gets shorter. */
export function shortening(
  current: Record<TierKey, number | null>,
  next: Record<TierKey, number | null>,
): Shortening | null {
  const tiers: TierKey[] = [];
  let keepMs = Infinity;
  for (const key of tierKeys) {
    const before = current[key];
    const after = next[key];
    if (before == null || after == null || after === 0) continue;
    if (before === 0 || after < before) {
      tiers.push(key);
      keepMs = Math.min(keepMs, after);
    }
  }
  return tiers.length ? { tiers, keepMs } : null;
}
