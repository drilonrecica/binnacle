import { missing } from '../i18n';
export { missing };
import type { Sample } from '../live-series.svelte';
import { formatRate } from '../i18n';
import { cpuDirection, type TrendView } from '../overview/headroom';
import type { TabItem } from '../ui/Tabs.svelte';

/**
 * Tabs of the resource detail (DESIGN §Resources). Archived resources have
 * no live containers, incidents or logs, so they keep Metrics and Changes.
 */
export function resourceTabs(archived: boolean, containers: number): TabItem[] {
  if (archived)
    return [
      { id: 'metrics', label: 'Metrics' },
      { id: 'changes', label: 'Changes' },
    ];
  return [
    { id: 'metrics', label: 'Metrics' },
    { id: 'containers', label: 'Containers', count: containers },
    { id: 'incidents', label: 'Incidents' },
    { id: 'changes', label: 'Changes' },
    { id: 'logs', label: 'Logs' },
  ];
}

/** Tab names used before the redesign, for links that still carry them. */
const retiredTabs: Record<string, string> = {
  alerts: 'incidents',
  activity: 'changes',
};

export function currentTabName(tab: string): string | null {
  return retiredTabs[tab] ?? null;
}

/**
 * Direction of a series that has no natural 0..100 scale (memory bytes):
 * the least-squares change over the window relative to its mean. Under a
 * tenth of the mean is steady.
 */
export function relativeDirection(
  samples: Sample[],
  fraction = 0.1,
): TrendView {
  const points = samples.filter(
    (sample): sample is { at: number; value: number } =>
      sample.value != null && Number.isFinite(sample.value),
  );
  const mean =
    points.reduce((sum, point) => sum + point.value, 0) / (points.length || 1);
  if (!(mean > 0)) return cpuDirection(points);
  // Scale to percent of the mean and reuse the CPU rule (five points).
  const scaled = points.map((point) => ({
    at: point.at,
    value: (point.value / mean) * (5 / fraction),
  }));
  return cpuDirection(scaled);
}

/**
 * Value and second line for a pair of rates, e.g. network or block I/O:
 * "1.2 MiB/s" over "in · 4 MiB/s out". Either side may be missing.
 */
export function ratePair(
  first: number | null | undefined,
  second: number | null | undefined,
  words: [string, string],
): { value: string; secondary: string } {
  const value = first == null ? missing : formatRate(first);
  const other = second == null ? missing : formatRate(second);
  return { value, secondary: `${words[0]} · ${other} ${words[1]}` };
}
