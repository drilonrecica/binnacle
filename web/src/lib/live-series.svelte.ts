import { SvelteDate } from 'svelte/reactivity';
import type { LiveSnapshot } from './live.svelte';
import type { SparklineResponse } from './api/metrics';
import { bucketize } from './series';

export type HostSeriesKey = 'cpu' | 'memory' | 'rx' | 'tx' | 'load1' | 'disk';
export type ResourceSeriesKey = 'cpu' | 'memory';

export interface Sample {
  at: number;
  value: number | null;
}

/** The longest window a sparkline draws by default (UI-6). */
export const defaultWindowMs = 3_600_000;
/** Windows longer than this are history charts, not live buffers. */
const maxWindowMs = 6 * 3_600_000;
/**
 * The fastest cadence a sample can arrive at. It bounds how many samples one
 * window can hold even if snapshots arrive faster than the 2 s default.
 */
const minCadenceMs = 1_000;
/** A pause between two snapshots longer than this is a gap to re-seed. */
export const reseedGapMs = 60_000;

type HostHistoryFetcher = (
  metrics: Array<'cpu' | 'memory' | 'network_rx' | 'network_tx'>,
  from: Date,
  to: Date,
) => Promise<{
  series: Array<{
    metric: string;
    points: Array<{ at: string; avg: number | null }>;
  }>;
}>;

/**
 * Turns the server's fixed-slot sparkline into samples at each slot's middle,
 * so they merge with live samples on one time axis.
 */
export function sparklineSamples(
  values: Array<number | null>,
  fromMs: number,
  toMs: number,
  stepSeconds: number,
): Sample[] {
  const stepMs = stepSeconds * 1000;
  if (stepMs <= 0) return [];
  return values.map((value, index) => ({
    at: Math.min(fromMs + index * stepMs + stepMs / 2, toMs),
    value: value != null && Number.isFinite(value) ? value : null,
  }));
}

/**
 * Rolling series from the live SSE snapshots. The buffers hold one full
 * window of time (UI-6), not a fixed sample count, so a one-hour sparkline
 * really draws an hour. History from the server seeds them on load and again
 * after a gap; live samples newer than the seed are kept on top of it.
 */
export class LiveSeriesStore {
  host = $state<Record<HostSeriesKey, Sample[]>>(emptyHost());
  resources = $state<Record<string, Record<ResourceSeriesKey, Sample[]>>>({});

  private windowMs = defaultWindowMs;
  private lastAt = -Infinity;
  private hostSeeded = false;
  private resourcesSeeded = false;

  /** How much time each buffer keeps. */
  get retentionMs() {
    return this.windowMs;
  }

  /** The most samples one buffer may hold for the current window. */
  get capacity() {
    return Math.ceil(this.windowMs / minCadenceMs) + 1;
  }

  /**
   * Widens the buffers to hold `windowMs`. Buffers only grow (up to six
   * hours), so a view asking for a shorter window never cuts another's.
   */
  ensureWindow(windowMs: number) {
    const next = Math.min(Math.max(windowMs, 0), maxWindowMs);
    if (next > this.windowMs) {
      this.windowMs = next;
      this.hostSeeded = false;
      this.resourcesSeeded = false;
    }
  }

  /** Drops everything, e.g. when the session ends. */
  reset() {
    this.host = emptyHost();
    this.resources = {};
    this.windowMs = defaultWindowMs;
    this.lastAt = -Infinity;
    this.hostSeeded = false;
    this.resourcesSeeded = false;
  }

  /** Replaces a host series with seeded history, keeping newer live samples. */
  seedHost(key: HostSeriesKey, samples: Sample[]) {
    this.host = { ...this.host, [key]: this.merge(this.host[key], samples) };
  }

  seedResource(id: string, key: ResourceSeriesKey, samples: Sample[]) {
    const existing = this.resources[id] ?? { cpu: [], memory: [] };
    this.resources = {
      ...this.resources,
      [id]: { ...existing, [key]: this.merge(existing[key], samples) },
    };
  }

  /** Seeds every resource from one sparkline response (the list endpoint). */
  seedResources(response: SparklineResponse) {
    const from = Date.parse(response.from);
    const to = Date.parse(response.to);
    const next = { ...this.resources };
    for (const [id, metrics] of Object.entries(response.resources)) {
      const existing = next[id] ?? { cpu: [], memory: [] };
      const seeded = { ...existing };
      for (const key of ['cpu', 'memory'] as const) {
        const values = metrics[key];
        if (!values) continue;
        seeded[key] = this.merge(
          existing[key],
          sparklineSamples(values, from, to, response.stepSeconds),
        );
      }
      next[id] = seeded;
    }
    this.resources = next;
  }

  push(snapshot: LiveSnapshot) {
    const at = Date.parse(snapshot.ts);
    if (!Number.isFinite(at) || at <= this.lastAt) return;
    if (at - this.lastAt > reseedGapMs && this.lastAt !== -Infinity) {
      // The stream was away long enough to leave a hole; fetch it again.
      this.hostSeeded = false;
      this.resourcesSeeded = false;
    }
    this.lastAt = at;
    const hostValues: Record<HostSeriesKey, number | null | undefined> = {
      cpu: snapshot.host.cpuPct,
      memory:
        snapshot.host.memoryPct ??
        percent(snapshot.host.memoryUsedBytes, snapshot.host.memoryTotalBytes),
      rx: snapshot.host.networkRxBps,
      tx: snapshot.host.networkTxBps,
      load1: snapshot.host.load1,
      disk: percent(snapshot.host.diskUsedBytes, snapshot.host.diskTotalBytes),
    };
    const hostNext = { ...this.host };
    for (const key of Object.keys(hostValues) as HostSeriesKey[]) {
      hostNext[key] = this.append(hostNext[key], {
        at,
        value: finite(hostValues[key]),
      });
    }
    this.host = hostNext;

    const next: typeof this.resources = {};
    for (const resource of snapshot.resources) {
      const existing = this.resources[resource.id] ?? { cpu: [], memory: [] };
      next[resource.id] = {
        cpu: this.append(existing.cpu, {
          at,
          value: finite(resource.cpuHostPct),
        }),
        memory: this.append(existing.memory, {
          at,
          value: finite(resource.memoryBytes),
        }),
      };
    }
    this.resources = next;
  }

  /**
   * Seeds the host series with one window of history so sparklines are
   * meaningful immediately; again after a gap or a wider window.
   */
  async ensureHostSeeded(
    fetchHistory: HostHistoryFetcher,
    memoryTotal: number | null,
  ) {
    if (this.hostSeeded) return;
    this.hostSeeded = true;
    try {
      const to = new SvelteDate();
      const from = new SvelteDate(to.getTime() - this.windowMs);
      const history = await fetchHistory(
        ['cpu', 'memory', 'network_rx', 'network_tx'],
        from,
        to,
      );
      for (const series of history.series) {
        const samples = series.points.map((point) => ({
          at: Date.parse(point.at),
          value: point.avg,
        }));
        if (series.metric === 'cpu') this.seedHost('cpu', samples);
        if (series.metric === 'memory' && memoryTotal)
          this.seedHost(
            'memory',
            samples.map((sample) => ({
              at: sample.at,
              value:
                sample.value == null
                  ? null
                  : (sample.value / memoryTotal) * 100,
            })),
          );
        if (series.metric === 'network_rx') this.seedHost('rx', samples);
        if (series.metric === 'network_tx') this.seedHost('tx', samples);
      }
    } catch {
      this.hostSeeded = false;
    }
  }

  /** Seeds every resource's CPU and memory from the sparkline endpoint. */
  async ensureResourcesSeeded(
    fetchSparklines: (windowMs: number) => Promise<SparklineResponse>,
  ) {
    if (this.resourcesSeeded) return;
    this.resourcesSeeded = true;
    try {
      this.seedResources(await fetchSparklines(this.windowMs));
    } catch {
      this.resourcesSeeded = false;
    }
  }

  /** Values from the last `windowMs`, oldest first. */
  hostWindow(key: HostSeriesKey, windowMs: number, now = Date.now()): Sample[] {
    const since = now - windowMs;
    return this.host[key].filter((sample) => sample.at >= since);
  }

  resourceWindow(
    id: string,
    key: ResourceSeriesKey,
    windowMs: number,
    now = Date.now(),
  ): Sample[] {
    const since = now - windowMs;
    return (this.resources[id]?.[key] ?? []).filter(
      (sample) => sample.at >= since,
    );
  }

  /** A resource sparkline of `slots` averages over the whole window. */
  resourceSparkline(
    id: string,
    key: ResourceSeriesKey,
    windowMs: number,
    now = Date.now(),
    slots = 60,
  ): Array<number | null> {
    return bucketize(
      this.resourceWindow(id, key, windowMs, now),
      now - windowMs,
      now,
      slots,
    );
  }

  /** Seeded history, then the live samples newer than its last sample. */
  private merge(current: Sample[], seed: Sample[]): Sample[] {
    const newest = seed.at(-1)?.at ?? -Infinity;
    const live = current.filter((sample) => sample.at > newest);
    return this.trim([...seed, ...live]);
  }

  private append(current: Sample[], sample: Sample): Sample[] {
    return this.trim([...current, sample]);
  }

  /** Keeps one window of time behind the newest sample, and no more. */
  private trim(samples: Sample[]): Sample[] {
    const newest = samples.at(-1)?.at;
    if (newest == null) return samples;
    const since = newest - this.windowMs;
    let start = 0;
    while (start < samples.length && (samples[start]?.at ?? since) < since)
      start++;
    start = Math.max(start, samples.length - this.capacity);
    return start > 0 ? samples.slice(start) : samples;
  }
}

function emptyHost(): Record<HostSeriesKey, Sample[]> {
  return { cpu: [], memory: [], rx: [], tx: [], load1: [], disk: [] };
}

function finite(value: number | null | undefined): number | null {
  return value != null && Number.isFinite(value) ? value : null;
}

export function percent(
  used?: number | null,
  total?: number | null,
): number | null {
  if (used == null || total == null || total <= 0) return null;
  return (used / total) * 100;
}

export const liveSeries = new LiveSeriesStore();
