import { SvelteDate } from 'svelte/reactivity';

export type CollectorState = 'healthy' | 'degraded' | 'down' | 'unknown';
export interface LiveSnapshot {
  seq: number;
  ts: string;
  bootIdentity: string;
  host: {
    cpuPct?: number | null;
    memoryUsedBytes?: number | null;
    memoryTotalBytes?: number | null;
    memoryPct?: number | null;
    diskUsedBytes?: number | null;
    diskTotalBytes?: number | null;
    load1?: number | null;
    uptimeSeconds?: number | null;
    networkRxBps?: number | null;
    networkTxBps?: number | null;
  };
  resources: Array<{
    id: string;
    name: string;
    status: string;
    cpuHostPct?: number | null;
    memoryBytes?: number | null;
    lastSeenAt?: string;
    category?: string;
    project?: string;
    environment?: string;
    infrastructure?: boolean;
    components?: Array<{ id: string; name: string; status: string }>;
  }>;
  collectors: Record<
    string,
    { state: CollectorState; reason?: string; freshAt?: string }
  >;
}
export interface LiveEvent {
  id: number;
  type: string;
  message: string;
  resourceId?: string;
}
export type ConnectionState =
  'connecting' | 'connected' | 'disconnected' | 'unauthorized';

export function decodeSnapshot(value: string): LiveSnapshot {
  const parsed = JSON.parse(value) as Partial<LiveSnapshot>;
  if (
    typeof parsed.seq !== 'number' ||
    typeof parsed.ts !== 'string' ||
    !parsed.host ||
    !Array.isArray(parsed.resources) ||
    !parsed.collectors
  )
    throw new Error('The live server response is malformed.');
  return parsed as LiveSnapshot;
}

export class LiveStore {
  snapshot = $state<LiveSnapshot | null>(null);
  events = $state<LiveEvent[]>([]);
  state = $state<ConnectionState>('disconnected');
  error = $state('');
  lastReceivedAt = $state<string | null>(null);
  private source: EventSource | null = null;
  private reconnectTimer: number | null = null;
  private attempts = 0;
  private generation = 0;
  connect(url = '/api/v1/live') {
    this.stopSource();
    const generation = ++this.generation;
    this.state = 'connecting';
    this.error = '';
    const source = new EventSource(url);
    this.source = source;
    source.addEventListener('snapshot', (event) => {
      try {
        this.snapshot = decodeSnapshot((event as MessageEvent).data);
        this.lastReceivedAt = new SvelteDate().toISOString();
        this.state = 'connected';
        this.error = '';
        this.attempts = 0;
      } catch (reason) {
        this.error =
          reason instanceof Error
            ? reason.message
            : 'The live response is malformed.';
        this.reconnect(source, url, generation);
      }
    });
    source.addEventListener('event', (event) => {
      try {
        const value = JSON.parse((event as MessageEvent).data) as LiveEvent;
        if (typeof value.id !== 'number' || typeof value.type !== 'string')
          throw new Error('Malformed event');
        if (!this.events.some((item) => item.id === value.id))
          this.events = [...this.events.slice(-127), value];
      } catch {
        this.error = 'A malformed live event was ignored.';
      }
    });
    source.onerror = () => {
      this.reconnect(source, url, generation);
    };
  }
  private reconnect(source: EventSource, url: string, generation: number) {
    if (this.source !== source || this.generation !== generation) return;
    this.state = 'disconnected';
    if (!this.error)
      this.error = 'Live updates are disconnected. Retrying automatically.';
    source.close();
    const delay = Math.min(1000 * 2 ** this.attempts++, 30000);
    this.reconnectTimer = window.setTimeout(async () => {
      if (this.generation !== generation) return;
      if (!(await sessionActive().catch(() => false))) {
        this.state = 'unauthorized';
        this.error = 'Your session expired. Sign in again.';
        return;
      }
      this.connect(url);
    }, delay);
  }
  retry() {
    this.connect();
  }
  private stopSource() {
    this.source?.close();
    this.source = null;
    if (this.reconnectTimer != null) window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
  }
  close() {
    this.generation++;
    this.stopSource();
    this.state = 'disconnected';
  }
}
export async function sessionActive(fetcher = fetch): Promise<boolean> {
  const response = await fetcher('/api/v1/session', {
    credentials: 'same-origin',
  });
  return response.status === 204;
}
