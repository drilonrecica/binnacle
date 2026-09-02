import type { LiveSnapshot } from './live.svelte';

export type HostSeriesKey = 'cpu' | 'memory' | 'rx' | 'tx' | 'load1' | 'disk';
export type ResourceSeriesKey = 'cpu' | 'memory';

export interface Sample {
  at: number;
  value: number | null;
}

const defaultCapacity = 900; // 30 minutes at the default 2 s cadence

function ring(capacity: number) {
  const values: Sample[] = [];
  return {
    values,
    push(sample: Sample) {
      values.push(sample);
      if (values.length > capacity) values.splice(0, values.length - capacity);
    },
  };
}

/**
 * Keeps short rolling series from the live SSE snapshots so sparklines can
 * animate between history fetches. History seeds the buffer once on load.
 */
class LiveSeriesStore {
  host = $state<Record<HostSeriesKey, Sample[]>>({
    cpu: [],
    memory: [],
    rx: [],
    tx: [],
    load1: [],
    disk: [],
  });
  resources = $state<Record<string, Record<ResourceSeriesKey, Sample[]>>>({});

  private capacity = defaultCapacity;
  private lastSeq = -1;

  /** Replaces a host series with seeded history, keeping newer live samples. */
  seedHost(key: HostSeriesKey, samples: Sample[]) {
    const newest = samples.at(-1)?.at ?? -Infinity;
    const live = this.host[key].filter((sample) => sample.at > newest);
    this.host = {
      ...this.host,
      [key]: [...samples, ...live].slice(-this.capacity),
    };
  }

  seedResource(id: string, key: ResourceSeriesKey, samples: Sample[]) {
    const existing = this.resources[id] ?? { cpu: [], memory: [] };
    const newest = samples.at(-1)?.at ?? -Infinity;
    const live = existing[key].filter((sample) => sample.at > newest);
    this.resources = {
      ...this.resources,
      [id]: { ...existing, [key]: [...samples, ...live].slice(-this.capacity) },
    };
  }

  push(snapshot: LiveSnapshot) {
    if (snapshot.seq === this.lastSeq) return;
    this.lastSeq = snapshot.seq;
    const at = new Date(snapshot.ts).getTime();
    const hostNext = { ...this.host };
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
    for (const key of Object.keys(hostValues) as HostSeriesKey[]) {
      const buffer = ring(this.capacity);
      buffer.values.push(...hostNext[key]);
      buffer.push({ at, value: finite(hostValues[key]) });
      hostNext[key] = buffer.values;
    }
    this.host = hostNext;

    const next: typeof this.resources = {};
    for (const resource of snapshot.resources) {
      const existing = this.resources[resource.id] ?? { cpu: [], memory: [] };
      const cpu = ring(this.capacity);
      cpu.values.push(...existing.cpu);
      cpu.push({ at, value: finite(resource.cpuHostPct) });
      const memory = ring(this.capacity);
      memory.values.push(...existing.memory);
      memory.push({ at, value: finite(resource.memoryBytes) });
      next[resource.id] = { cpu: cpu.values, memory: memory.values };
    }
    this.resources = next;
  }

  private seeded = false;

  /**
   * Seeds the host series with the last hour of history once per session so
   * sparklines are meaningful immediately after opening the app.
   */
  async ensureHostSeeded(
    fetchHistory: (
      metrics: Array<'cpu' | 'memory' | 'network_rx' | 'network_tx'>,
      from: Date,
      to: Date,
    ) => Promise<{
      series: Array<{
        metric: string;
        points: Array<{ at: string; avg: number | null }>;
      }>;
    }>,
    memoryTotal: number | null,
  ) {
    if (this.seeded) return;
    this.seeded = true;
    try {
      const to = new Date();
      const from = new Date(to.getTime() - 3_600_000);
      const history = await fetchHistory(
        ['cpu', 'memory', 'network_rx', 'network_tx'],
        from,
        to,
      );
      for (const series of history.series) {
        const samples = series.points.map((point) => ({
          at: new Date(point.at).getTime(),
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
      this.seeded = false;
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
