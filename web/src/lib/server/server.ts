import { formatBytes, formatNumber } from '../i18n';
import type {
  CollectorState,
  FilesystemObservation,
  LiveSnapshot,
} from '../live.svelte';
import { percent, type Sample } from '../live-series.svelte';
import {
  cpuDirection,
  diskLabel,
  fallbackThresholds,
  orderFilesystems,
  trendView,
  type GaugeThresholds,
  type Instrument,
  type TrendView,
} from '../overview/headroom';
import { normalizeStatus, statusLabel } from '../ui/status';

/** The missing-value glyph, the same everywhere (UI-18). */
export const missing = '—';

/** Chart groups on Server › Metrics that an instrument can focus. */
export const focusableCharts = new Set([
  'cpu',
  'load',
  'memory',
  'network',
  'disk',
  'iops',
]);

/** The chart a `?metric=` link asks for, or '' when it names none. */
export function chartFocus(metric: string): string {
  if (metric === 'swap') return 'memory';
  return focusableCharts.has(metric) ? metric : '';
}

function usedOf(
  used: number | null | undefined,
  total: number | null | undefined,
) {
  if (used == null) return { valueText: undefined, caption: undefined };
  return {
    valueText: formatBytes(used),
    caption: total == null ? undefined : `of ${formatBytes(total)}`,
  };
}

const noEstimate: TrendView = { text: 'no estimate', tone: 'muted' };

/**
 * Instruments of the Server page (DESIGN §Server): CPU, memory, swap and
 * every filesystem, each disk with its time-to-full. Unlike Headroom there
 * is no cap; this is where the disks beyond the eighth instrument live.
 * CPU and memory link to their chart, disks to their Filesystems row.
 */
export function serverInstruments(
  snapshot: LiveSnapshot,
  thresholds: GaugeThresholds,
  cpuSamples: Sample[],
): Instrument[] {
  const host = snapshot.host;
  const memoryPct =
    host.memoryPct ?? percent(host.memoryUsedBytes, host.memoryTotalBytes);
  const swapConfigured = host.swapTotalBytes != null && host.swapTotalBytes > 0;
  const swapPct = swapConfigured
    ? (host.swapPct ?? percent(host.swapUsedBytes, host.swapTotalBytes))
    : null;
  const instruments: Instrument[] = [
    {
      key: 'cpu',
      label: 'CPU',
      value: host.cpuPct ?? null,
      valueText: host.cpuPct == null ? undefined : formatNumber(host.cpuPct),
      unit: '%',
      caption:
        host.load1 == null ? undefined : `load ${formatNumber(host.load1)}`,
      thresholds: thresholds.cpu,
      trend: cpuDirection(cpuSamples),
      href: '/server?metric=cpu',
    },
    {
      key: 'memory',
      label: 'Memory',
      value: memoryPct,
      ...usedOf(host.memoryUsedBytes, host.memoryTotalBytes),
      unit: '',
      thresholds: thresholds.memory,
      trend: trendView(host.memoryTrend),
      href: '/server?metric=memory',
    },
    {
      key: 'swap',
      label: 'Swap',
      value: swapPct,
      ...(swapConfigured
        ? usedOf(host.swapUsedBytes, host.swapTotalBytes)
        : { valueText: undefined, caption: 'no swap configured' }),
      unit: '',
      // No alert rule covers swap; it shares the memory design constants.
      thresholds: fallbackThresholds.memory,
      trend: swapConfigured
        ? noEstimate
        : { text: 'not configured', tone: 'muted' },
      href: '/server?metric=swap',
    },
  ];
  for (const mount of serverFilesystems(snapshot)) {
    const limits = thresholds.filesystem(mount.mountKey);
    instruments.push({
      key: `fs:${mount.mountKey}`,
      label: diskLabel(mount),
      path: mount.mountPoint,
      value: mount.usedPct ?? percent(mount.usedBytes, mount.totalBytes),
      ...usedOf(mount.usedBytes, mount.totalBytes),
      unit: '',
      thresholds: { warn: limits.warn, critical: limits.critical },
      trend: trendView(mount.trend, limits.criticalDays),
      href: `/server?tab=filesystems&mount=${encodeURIComponent(mount.mountKey)}`,
    });
  }
  return instruments;
}

/**
 * Filesystems in the Headroom order (root, Docker data, then by fullness).
 * Older servers report the root disk only on the host object.
 */
export function serverFilesystems(
  snapshot: LiveSnapshot,
): FilesystemObservation[] {
  const host = snapshot.host;
  const reported = snapshot.filesystems ?? [];
  if (!reported.length && host.diskTotalBytes != null)
    return [
      {
        mountKey: '/',
        mountPoint: '/',
        fsType: '',
        role: 'root',
        usedBytes: host.diskUsedBytes,
        totalBytes: host.diskTotalBytes,
        availableBytes:
          host.diskUsedBytes == null
            ? null
            : host.diskTotalBytes - host.diskUsedBytes,
        usedPct: percent(host.diskUsedBytes, host.diskTotalBytes),
      },
    ];
  return orderFilesystems(reported);
}

/** "full in ~6 d" and its tone for a Filesystems row. */
export function filesystemTrend(
  mount: FilesystemObservation,
  thresholds: GaugeThresholds,
): TrendView {
  return trendView(
    mount.trend,
    thresholds.filesystem(mount.mountKey).criticalDays,
  );
}

const collectorTitles: Record<string, string> = {
  host: 'Host collector',
  docker: 'Docker collector',
  coolify: 'Coolify collector',
};

const collectorScopes: Record<string, string> = {
  host: 'CPU, memory, load, filesystems, network, and disk I/O from /proc and /sys.',
  docker: 'Containers, their stats, and lifecycle events from the Docker API.',
  coolify: 'Project and environment metadata from the Coolify API.',
};

const collectorOrder = ['host', 'docker', 'coolify'];

export interface CollectorView {
  name: string;
  title: string;
  state: CollectorState;
  /** The state as a word: Healthy, Degraded, Down or Unknown. */
  word: string;
  reason: string;
  observes: string;
  freshAt?: string;
}

/**
 * One view per collector with its state as a word and a reason, never only
 * a coloured dot (DESIGN §Server, UI-11). Known collectors come first in a
 * fixed order so the cards do not move between snapshots.
 */
export function collectorViews(
  collectors: LiveSnapshot['collectors'] | undefined,
): CollectorView[] {
  const entries = Object.entries(collectors ?? {});
  const rank = (name: string) => {
    const index = collectorOrder.indexOf(name);
    return index < 0 ? collectorOrder.length : index;
  };
  entries.sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b));
  return entries.map(([name, health]) => {
    const reason = health.reason?.trim() ?? '';
    return {
      name,
      title: collectorTitles[name] ?? `${name} collector`,
      state: health.state,
      word: statusLabel(normalizeStatus(health.state)),
      reason:
        reason ||
        (health.state === 'healthy'
          ? 'Operating normally'
          : 'No reason reported'),
      observes: collectorScopes[name] ?? missing,
      freshAt: health.freshAt,
    };
  });
}

/** The marker the server puts where it removed a secret. */
export const redactionMarker = '[REDACTED]';

/**
 * Splits a command line at the redaction markers so the page can set the
 * removed parts apart. The server redacts before the line leaves it; this
 * only renders what came back.
 */
export function commandParts(
  command: string,
): Array<{ text: string; redacted: boolean }> {
  const parts: Array<{ text: string; redacted: boolean }> = [];
  let rest = command;
  for (;;) {
    const at = rest.indexOf(redactionMarker);
    if (at < 0) break;
    if (at > 0) parts.push({ text: rest.slice(0, at), redacted: false });
    parts.push({ text: redactionMarker, redacted: true });
    rest = rest.slice(at + redactionMarker.length);
  }
  if (rest) parts.push({ text: rest, redacted: false });
  return parts;
}
