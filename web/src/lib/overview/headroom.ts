import type { Rule } from '../api/alerts';
import { resourceGroup, type LiveResource } from '../api/resources';
import { formatBytes, formatNumber } from '../i18n';
import type {
  FilesystemObservation,
  LiveSnapshot,
  Trend,
} from '../live.svelte';
import { percent, type Sample } from '../live-series.svelte';
import { staleResource } from '../watch';

/** Design constants that fill in where no enabled rule sets a threshold. */
export const fallbackThresholds = {
  cpu: { warn: 80, critical: 90 },
  memory: { warn: 85, critical: 95 },
  filesystem: { warn: 80, critical: 95 },
  criticalDays: 7,
} as const;

export interface Thresholds {
  warn: number;
  critical: number;
}

export interface GaugeThresholds {
  cpu: Thresholds;
  memory: Thresholds;
  /** Thresholds and critical days for one mount, by mount key. */
  filesystem(mountKey: string): Thresholds & { criticalDays: number };
}

/**
 * The rule that applies to a target, the way the evaluator picks it: an
 * enabled rule scoped to the target wins over an enabled global rule.
 */
function effectiveRule(
  rules: Rule[],
  family: string,
  scopeType: string,
  scopeId: string,
): Rule | undefined {
  let global: Rule | undefined;
  for (const rule of rules) {
    if (!rule.enabled || rule.family !== family) continue;
    if (rule.scopeType === scopeType && rule.scopeId === scopeId) return rule;
    if (rule.scopeType === 'global') global = rule;
  }
  return global;
}

function threshold(rule: Rule | undefined): number | null {
  const value = rule?.threshold;
  return value != null && Number.isFinite(value) ? value : null;
}

/**
 * Host CPU and memory have one rule family each; its severity says whether
 * its threshold is the warn or the critical line. The other line falls back.
 */
function hostThresholds(
  rules: Rule[],
  family: string,
  fallback: Thresholds,
): Thresholds {
  const candidates = rules.filter(
    (rule) =>
      rule.enabled &&
      rule.family === family &&
      (rule.scopeType === 'host' || rule.scopeType === 'global'),
  );
  // A host-scoped rule replaces the global one of the same severity.
  const pick = (severity: Rule['severity']) =>
    threshold(
      candidates.find(
        (rule) => rule.severity === severity && rule.scopeType === 'host',
      ) ??
        candidates.find(
          (rule) => rule.severity === severity && rule.scopeType === 'global',
        ),
    );
  return {
    warn: pick('warning') ?? fallback.warn,
    critical: pick('critical') ?? fallback.critical,
  };
}

/**
 * Gauge thresholds from the enabled alert rules (DESIGN §Headroom ›
 * Thresholds), so the page and the alerts never disagree. Missing or
 * disabled rules fall back to the design constants.
 */
export function gaugeThresholds(rules: Rule[]): GaugeThresholds {
  return {
    cpu: hostThresholds(rules, 'host_cpu_warning', fallbackThresholds.cpu),
    memory: hostThresholds(
      rules,
      'host_memory_warning',
      fallbackThresholds.memory,
    ),
    filesystem(mountKey) {
      const scoped = (family: string) =>
        threshold(effectiveRule(rules, family, 'filesystem', mountKey));
      return {
        warn:
          scoped('filesystem_warning') ?? fallbackThresholds.filesystem.warn,
        critical:
          scoped('filesystem_critical') ??
          fallbackThresholds.filesystem.critical,
        criticalDays:
          scoped('filesystem_full_within_days') ??
          fallbackThresholds.criticalDays,
      };
    },
  };
}

export interface TrendView {
  text: string;
  tone: 'accent' | 'critical' | 'muted';
}

/** "full in ~6 d", "steady", "shrinking" or "not enough history". */
export function trendView(
  trend: Trend | null | undefined,
  criticalDays: number = fallbackThresholds.criticalDays,
): TrendView {
  switch (trend?.state) {
    case 'filling': {
      const days = trend.daysToFull;
      if (days == null || !Number.isFinite(days))
        return { text: 'filling', tone: 'muted' };
      const text =
        days < 1 ? 'full in < 1 d' : `full in ~${Math.round(days)} d`;
      return { text, tone: days < criticalDays ? 'critical' : 'accent' };
    }
    case 'steady':
      return { text: 'steady', tone: 'muted' };
    case 'shrinking':
      return { text: 'shrinking', tone: 'muted' };
    default:
      return { text: 'not enough history', tone: 'muted' };
  }
}

/**
 * Direction of CPU over the ring buffer: the least-squares slope projected
 * over the covered span. A move of under five points is steady; fewer than
 * ten minutes of samples is not enough to say.
 */
export function cpuDirection(samples: Sample[]): TrendView {
  const points = samples.filter(
    (sample): sample is { at: number; value: number } =>
      sample.value != null && Number.isFinite(sample.value),
  );
  const first = points[0];
  const last = points.at(-1);
  if (points.length < 3 || !first || !last || last.at - first.at < 600_000)
    return { text: 'not enough history', tone: 'muted' };
  const meanAt = points.reduce((sum, p) => sum + p.at, 0) / points.length;
  const meanValue = points.reduce((sum, p) => sum + p.value, 0) / points.length;
  let numerator = 0;
  let denominator = 0;
  for (const point of points) {
    numerator += (point.at - meanAt) * (point.value - meanValue);
    denominator += (point.at - meanAt) ** 2;
  }
  const change = denominator
    ? (numerator / denominator) * (last.at - first.at)
    : 0;
  if (change >= 5) return { text: 'rising', tone: 'muted' };
  if (change <= -5) return { text: 'falling', tone: 'muted' };
  return { text: 'steady', tone: 'muted' };
}

export interface Instrument {
  key: string;
  label: string;
  /** Mount point, shown in mono under the label for disks. */
  path?: string;
  value: number | null;
  valueText?: string;
  unit: string;
  caption?: string;
  thresholds: Thresholds;
  trend: TrendView;
  href: string;
}

export interface InstrumentSet {
  instruments: Instrument[];
  /** Disks beyond the eight-instrument limit. */
  moreDisks: number;
}

/** CPU, memory and up to six disks; eight instruments at most. */
export const maxInstruments = 8;

/**
 * Disk order on Headroom: the root filesystem, the Docker data root, then
 * the rest by fullness (DESIGN §Headroom, item 3).
 */
export function orderFilesystems(
  filesystems: FilesystemObservation[],
): FilesystemObservation[] {
  const rank = (mount: FilesystemObservation) =>
    mount.role === 'root' || (!mount.role && mount.mountPoint === '/')
      ? 0
      : mount.role === 'docker'
        ? 1
        : 2;
  return [...filesystems].sort(
    (a, b) =>
      rank(a) - rank(b) ||
      (b.usedPct ?? -1) - (a.usedPct ?? -1) ||
      a.mountPoint.localeCompare(b.mountPoint),
  );
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

/** The instrument name of a disk, from the role the host collector set. */
export function diskLabel(mount: FilesystemObservation): string {
  if (mount.role === 'root' || mount.mountPoint === '/') return 'Root disk';
  if (mount.role === 'docker') return 'Docker data';
  if (mount.role === 'data') return 'Binnacle data';
  return 'Disk';
}

export function headroomInstruments(
  snapshot: LiveSnapshot,
  thresholds: GaugeThresholds,
  cpuSamples: Sample[],
): InstrumentSet {
  const host = snapshot.host;
  const memoryPct =
    host.memoryPct ?? percent(host.memoryUsedBytes, host.memoryTotalBytes);
  const instruments: Instrument[] = [
    {
      key: 'cpu',
      label: 'CPU',
      value: host.cpuPct ?? null,
      valueText: host.cpuPct == null ? undefined : formatNumber(host.cpuPct),
      unit: '%',
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
  ];

  let filesystems = snapshot.filesystems ?? [];
  if (!filesystems.length && host.diskTotalBytes != null) {
    // Older servers report the root disk only on the host object.
    filesystems = [
      {
        mountKey: '/',
        mountPoint: '/',
        fsType: '',
        role: 'root',
        usedBytes: host.diskUsedBytes,
        totalBytes: host.diskTotalBytes,
        usedPct: percent(host.diskUsedBytes, host.diskTotalBytes),
      },
    ];
  }
  const disks = orderFilesystems(filesystems);
  const room = maxInstruments - instruments.length;
  // Collapsing one disk into "1 more" would save nothing; show it instead.
  const shown = disks.length > room ? disks.slice(0, room - 1) : disks;
  for (const mount of shown) {
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
  return { instruments, moreDisks: disks.length - shown.length };
}

/**
 * The instrument link's accessible name, "<label> <value>, <trend>". It
 * follows the gauge's visible text in order, so the visible words are part
 * of the name (WCAG 2.5.3), and adds the percentage for readings in bytes.
 */
export function instrumentName(instrument: Instrument): string {
  const label = instrument.path
    ? `${instrument.label} ${instrument.path}`
    : instrument.label;
  if (instrument.value == null) return `${label} no data`;
  const value = instrument.value;
  const { warn, critical } = instrument.thresholds;
  const over =
    value >= critical
      ? ` Above ${Math.round(critical)}%`
      : value >= warn
        ? ` Above ${Math.round(warn)}%`
        : '';
  const reading =
    instrument.unit === '%'
      ? `${instrument.valueText ?? Math.round(value)}%`
      : (instrument.valueText ?? `${Math.round(value)}%`);
  const caption = instrument.caption ? ` ${instrument.caption}` : '';
  const share = instrument.unit === '%' ? '' : `, ${Math.round(value)}% used`;
  return `${label} ${reading}${over}${caption}, ${instrument.trend.text}${share}`;
}

const healthRank: Record<string, number> = {
  down: 5,
  degraded: 4,
  stale: 3,
  unknown: 3,
  paused: 2,
  healthy: 1,
};

export const pinnedGroup = 'Pinned';

function rankOf(resource: LiveResource, snapshotAt: string): number {
  if (resource.status !== 'archived' && staleResource(resource, snapshotAt))
    return healthRank.stale ?? 0;
  return healthRank[resource.status] ?? healthRank.unknown ?? 0;
}

/**
 * Resources for Headroom: archived ones left out, the worst health first
 * (stale telemetry counts as unknown), then by name. Pinned resources form
 * their own group at the top; the other groups (project / environment) are
 * ordered by their worst member, with Ungrouped and Infrastructure last.
 */
export function headroomResources(
  resources: LiveResource[],
  pinned: string[],
  snapshotAt: string,
) {
  const pins = new Set(pinned);
  const rows = resources
    .filter((resource) => resource.status !== 'archived')
    .sort(
      (a, b) =>
        rankOf(b, snapshotAt) - rankOf(a, snapshotAt) ||
        a.name.localeCompare(b.name),
    );
  const groupOf = (resource: LiveResource) =>
    pins.has(resource.id) ? pinnedGroup : resourceGroup(resource);
  const worst = new Map<string, number>();
  for (const resource of rows) {
    const key = groupOf(resource);
    worst.set(key, Math.max(worst.get(key) ?? 0, rankOf(resource, snapshotAt)));
  }
  const trailing = ['Ungrouped', 'Infrastructure'];
  const groupOrder = [
    ...(worst.has(pinnedGroup) ? [pinnedGroup] : []),
    ...[...worst.keys()]
      .filter((key) => key !== pinnedGroup && !trailing.includes(key))
      .sort(
        (a, b) =>
          (worst.get(b) ?? 0) - (worst.get(a) ?? 0) || a.localeCompare(b),
      ),
    ...trailing.filter((key) => worst.has(key)),
  ];
  return { rows, groupOf, groupOrder };
}
