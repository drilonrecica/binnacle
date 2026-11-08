import type { ContainerDetail } from '../api/server';
import { formatBytes, formatNumber, missing } from '../i18n';
import type { LiveSnapshot } from '../live.svelte';

type Host = LiveSnapshot['host'];
type Resource = LiveSnapshot['resources'][number];
type Component = NonNullable<Resource['components']>[number];

/** One container on Server › Live, with the resource it belongs to. */
export interface ContainerRow extends Component {
  resourceId: string;
  resourceName: string;
  memoryLimitBytes?: number | null;
  /** Working set as a share of the memory limit. */
  memoryPct: number | null;
  startedAt?: string;
  image?: string;
  ports?: string[];
  /** Seconds since the container started, or null when not running. */
  uptimeSeconds: number | null;
}

/**
 * Every container of every resource, one row each, with its detail from
 * /api/v1/server/containers when known. Memory % is worked out here from the
 * live working set and the limit, so it moves with every frame.
 */
export function containerRows(
  snapshot: LiveSnapshot,
  now: number,
  details: ReadonlyMap<string, ContainerDetail> = new Map(),
): ContainerRow[] {
  const rows: ContainerRow[] = [];
  for (const resource of snapshot.resources) {
    for (const component of resource.components ?? []) {
      const detail = details.get(component.id);
      const running = component.runtimeState !== 'exited';
      const started =
        running && detail?.startedAt ? Date.parse(detail.startedAt) : NaN;
      const limit = detail?.memoryLimitBytes;
      rows.push({
        ...component,
        resourceId: resource.id,
        resourceName: resource.name,
        memoryLimitBytes: limit,
        memoryPct:
          component.memoryBytes != null && limit
            ? (component.memoryBytes * 100) / limit
            : null,
        startedAt: detail?.startedAt,
        image: detail?.image,
        ports: detail?.ports,
        uptimeSeconds: Number.isFinite(started)
          ? Math.max(0, (now - started) / 1000)
          : null,
      });
    }
  }
  return rows;
}

export interface Reading {
  label: string;
  value: string;
  /** Explains the reading; used as a tooltip. */
  hint?: string;
}

function pct(value: number | null | undefined) {
  return value == null ? missing : `${formatNumber(value)}%`;
}

function bytes(value: number | null | undefined) {
  return value == null ? missing : formatBytes(value);
}

function perSecond(value: number | null | undefined) {
  return value == null ? missing : `${formatNumber(Math.round(value))}/s`;
}

/** Where CPU time went, as shares of all CPU time across cores. */
export function cpuReadings(host: Host): Reading[] {
  return [
    { label: 'user', value: pct(host.cpuUserPct), hint: 'Includes nice' },
    { label: 'system', value: pct(host.cpuSystemPct) },
    { label: 'nice', value: pct(host.cpuNicePct) },
    { label: 'iowait', value: pct(host.cpuIOWaitPct) },
    { label: 'irq', value: pct(host.cpuIrqPct) },
    { label: 'softirq', value: pct(host.cpuSoftirqPct) },
    { label: 'steal', value: pct(host.cpuStealPct) },
    { label: 'idle', value: pct(host.cpuIdlePct) },
  ];
}

/** System activity from /proc/stat. */
export function activityReadings(host: Host): Reading[] {
  return [
    { label: 'ctx switches', value: perSecond(host.contextSwitchesPs) },
    { label: 'interrupts', value: perSecond(host.interruptsPs) },
    { label: 'soft irqs', value: perSecond(host.softInterruptsPs) },
    {
      label: 'runnable',
      value: host.procsRunning == null ? missing : String(host.procsRunning),
    },
    {
      label: 'blocked',
      value: host.procsBlocked == null ? missing : String(host.procsBlocked),
      hint: 'Tasks waiting on I/O',
    },
  ];
}

export function memoryReadings(host: Host): Reading[] {
  return [
    { label: 'total', value: bytes(host.memoryTotalBytes) },
    {
      label: 'used',
      value: bytes(host.memoryUsedBytes),
      hint: 'Total minus available',
    },
    { label: 'available', value: bytes(host.memoryAvailableBytes) },
    { label: 'free', value: bytes(host.memoryFreeBytes) },
    { label: 'active', value: bytes(host.memoryActiveBytes) },
    { label: 'inactive', value: bytes(host.memoryInactiveBytes) },
    { label: 'cached', value: bytes(host.memoryCachedBytes) },
    { label: 'buffers', value: bytes(host.memoryBuffersBytes) },
  ];
}

export function swapReadings(host: Host): Reading[] {
  const total = host.swapTotalBytes;
  const used = host.swapUsedBytes;
  return [
    { label: 'total', value: bytes(total) },
    { label: 'used', value: bytes(used) },
    {
      label: 'free',
      value: bytes(total == null || used == null ? null : total - used),
    },
  ];
}

export function loadReadings(host: Host, cores: number | undefined): Reading[] {
  const load = (value: number | null | undefined) =>
    value == null ? missing : value.toFixed(2);
  const readings: Reading[] = [
    { label: '1 min', value: load(host.load1) },
    { label: '5 min', value: load(host.load5) },
    { label: '15 min', value: load(host.load15) },
  ];
  if (cores)
    readings.push({
      label: 'per core',
      value: host.load1 == null ? missing : (host.load1 / cores).toFixed(2),
      hint: '1-minute load divided by the core count',
    });
  return readings;
}

/** "used / limit" for a container's memory, the limit only when known. */
export function memoryOfLimit(
  row: Pick<ContainerRow, 'memoryBytes' | 'memoryLimitBytes'>,
): string {
  if (row.memoryBytes == null) return missing;
  if (row.memoryLimitBytes == null) return formatBytes(row.memoryBytes);
  return `${formatBytes(row.memoryBytes)} / ${formatBytes(row.memoryLimitBytes)}`;
}

/**
 * Two per-second rates as one compact cell, "12 KiB · 3 KiB"; the column
 * header carries the "/s" so the cell stays narrow.
 */
export function rwRates(
  read: number | null | undefined,
  write: number | null | undefined,
): string {
  if (read == null && write == null) return missing;
  const rate = (value: number | null | undefined) =>
    value == null ? missing : formatBytes(value);
  return `${rate(read)} · ${rate(write)}`;
}

/** Cumulative CPU time as h:mm:ss, the way top and Glances show it. */
export function formatCpuTime(seconds: number | null | undefined): string {
  if (seconds == null || !Number.isFinite(seconds) || seconds < 0)
    return missing;
  const whole = Math.floor(seconds);
  const hours = Math.floor(whole / 3600);
  const minutes = Math.floor((whole % 3600) / 60);
  const rest = whole % 60;
  const two = (value: number) => String(value).padStart(2, '0');
  return `${hours}:${two(minutes)}:${two(rest)}`;
}
