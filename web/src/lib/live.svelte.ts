import { SvelteDate } from 'svelte/reactivity';

export type CollectorState = 'healthy' | 'degraded' | 'down' | 'unknown';
export interface LiveSnapshot {
  seq: number;
  ts: string;
  bootIdentity: string;
  host: {
    cpuPct?: number | null;
    cpuUserPct?: number | null;
    cpuSystemPct?: number | null;
    cpuIOWaitPct?: number | null;
    cpuStealPct?: number | null;
    memoryUsedBytes?: number | null;
    memoryAvailableBytes?: number | null;
    swapUsedBytes?: number | null;
    swapTotalBytes?: number | null;
    swapPct?: number | null;
    load5?: number | null;
    load15?: number | null;
    diskReadBps?: number | null;
    diskWriteBps?: number | null;
    diskReadIops?: number | null;
    diskWriteIops?: number | null;
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
    context?: string;
    project?: string;
    environment?: string;
    infrastructure?: boolean;
    rxBps?: number | null;
    txBps?: number | null;
    blockReadBps?: number | null;
    blockWriteBps?: number | null;
    signalStatus?: string;
    components?: Array<{
      id: string;
      name: string;
      status: string;
      runtimeState?: string;
      healthStatus?: string;
      cpuHostPct?: number | null;
      memoryBytes?: number | null;
      rxBps?: number | null;
      txBps?: number | null;
      blockReadBps?: number | null;
      blockWriteBps?: number | null;
      pids?: number | null;
    }>;
  }>;
  collectors: Record<
    string,
    { state: CollectorState; reason?: string; freshAt?: string }
  >;
  filesystems?: FilesystemObservation[];
}

export interface FilesystemObservation {
  at?: string;
  mountKey: string;
  mountPoint: string;
  fsType: string;
  totalBytes?: number | null;
  usedBytes?: number | null;
  availableBytes?: number | null;
  usedPct?: number | null;
  inodesTotal?: number | null;
  inodesUsed?: number | null;
  inodesUsedPct?: number | null;
}
export interface LiveEvent {
  /** ULID, unique across restarts; equals the stored event id. */
  id: string;
  /** Per-process stream sequence; increases with every live message. */
  seq: number;
  at?: string;
  type: string;
  message: string;
  severity?: string;
  resourceId?: string;
  containerInstanceId?: string;
}
/**
 * Where the live stream stands, in the words the shell shows:
 * - connecting: a stream is opening (first open or the browser's own retry)
 *   and no snapshot has arrived on it yet;
 * - live: snapshots are arriving;
 * - stale: the stream is open but no snapshot came for `staleAfterMs`;
 * - disconnected: the stream is closed and a reconnect is scheduled;
 * - unauthorized: the session ended, nothing reconnects.
 */
export type ConnectionState =
  'connecting' | 'live' | 'stale' | 'disconnected' | 'unauthorized';

/** The part of EventSource the store uses, so tests can drive a fake. */
export interface LiveSource {
  readonly readyState: number;
  addEventListener(type: string, listener: (event: MessageEvent) => void): void;
  onerror: ((event: Event) => void) | null;
  close(): void;
}

export interface LiveStoreOptions {
  url?: string;
  createSource?: (url: string) => LiveSource;
  /** Resolves false when the session is gone; rejects when it cannot tell. */
  sessionActive?: () => Promise<boolean>;
  now?: () => number;
  /** A stream that sends no snapshot this long after opening has failed. */
  connectTimeoutMs?: number;
  /** An open stream with no snapshot for this long is stale. */
  staleAfterMs?: number;
  /** A stale stream quiet for this long in total is reopened. */
  quietReconnectMs?: number;
  backoffBaseMs?: number;
  backoffMaxMs?: number;
}

/** EventSource.readyState values. */
const SOURCE_CONNECTING = 0;
const SOURCE_CLOSED = 2;

/** Snapshots arrive every 2 s; five missed ones make the data stale. */
export const staleAfterMs = 10_000;
/** UI-7: a proxy that buffers SSE never delivers; give up opening after this. */
export const connectTimeoutMs = 10_000;
const eventCapacity = 128;

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

function decodeEvent(value: string): LiveEvent {
  const parsed = JSON.parse(value) as Partial<LiveEvent>;
  if (
    typeof parsed.id !== 'string' ||
    typeof parsed.seq !== 'number' ||
    typeof parsed.type !== 'string'
  )
    throw new Error('Malformed event');
  return parsed as LiveEvent;
}

/** Adds the resume cursor for a stream the store opens itself (UI-2). */
export function resumeURL(url: string, lastEventId: string): string {
  if (!lastEventId) return url;
  const separator = url.includes('?') ? '&' : '?';
  return `${url}${separator}lastEventId=${encodeURIComponent(lastEventId)}`;
}

/**
 * The live SSE stream. Transient errors are left to the browser's own retry,
 * which resumes with Last-Event-ID; when the browser gives up (an HTTP error,
 * a stream that never delivers) the store backs off, checks the session and
 * opens a new stream with `?lastEventId=` so missed events are replayed.
 */
export class LiveStore {
  snapshot = $state<LiveSnapshot | null>(null);
  events = $state<LiveEvent[]>([]);
  state = $state<ConnectionState>('disconnected');
  error = $state('');
  lastReceivedAt = $state<string | null>(null);
  /** The id of the newest frame received; what a new stream resumes from. */
  lastEventId = '';

  private readonly url: string;
  private readonly createSource: (url: string) => LiveSource;
  private readonly checkSession: () => Promise<boolean>;
  private readonly now: () => number;
  private readonly connectTimeoutMs: number;
  private readonly staleAfterMs: number;
  private readonly quietReconnectMs: number;
  private readonly backoffBaseMs: number;
  private readonly backoffMaxMs: number;

  private source: LiveSource | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private watchdog: ReturnType<typeof setTimeout> | null = null;
  private attempts = 0;
  private generation = 0;

  constructor(options: LiveStoreOptions = {}) {
    this.url = options.url ?? '/api/v1/live';
    this.createSource =
      options.createSource ?? ((url) => new EventSource(url) as LiveSource);
    this.checkSession = options.sessionActive ?? (() => sessionActive());
    this.now = options.now ?? (() => Date.now());
    this.connectTimeoutMs = options.connectTimeoutMs ?? connectTimeoutMs;
    this.staleAfterMs = options.staleAfterMs ?? staleAfterMs;
    this.quietReconnectMs = options.quietReconnectMs ?? 30_000;
    this.backoffBaseMs = options.backoffBaseMs ?? 1_000;
    this.backoffMaxMs = options.backoffMaxMs ?? 30_000;
  }

  /** How old the newest snapshot is, or null before the first one. */
  ageMs(now = this.now()): number | null {
    if (!this.lastReceivedAt) return null;
    return Math.max(0, now - Date.parse(this.lastReceivedAt));
  }

  connect() {
    this.stopSource();
    const generation = ++this.generation;
    this.state = 'connecting';
    const source = this.createSource(resumeURL(this.url, this.lastEventId));
    this.source = source;
    this.arm(this.connectTimeoutMs, () =>
      this.fail(
        source,
        'Live updates did not start. A proxy may be buffering the stream.',
      ),
    );
    source.addEventListener('snapshot', (event) => {
      if (this.source !== source) return;
      let snapshot: LiveSnapshot;
      try {
        snapshot = decodeSnapshot(event.data as string);
      } catch (reason) {
        this.fail(
          source,
          reason instanceof Error
            ? reason.message
            : 'The live response is malformed.',
        );
        return;
      }
      this.track(event);
      this.snapshot = snapshot;
      this.lastReceivedAt = new SvelteDate(this.now()).toISOString();
      this.state = 'live';
      this.error = '';
      this.attempts = 0;
      this.armStale(source);
    });
    source.addEventListener('event', (event) => {
      if (this.source !== source) return;
      try {
        const value = decodeEvent(event.data as string);
        this.track(event);
        if (!this.events.some((item) => item.id === value.id))
          this.events = [...this.events.slice(1 - eventCapacity), value];
      } catch {
        this.error = 'A malformed live event was ignored.';
      }
    });
    source.onerror = () => {
      if (this.source !== source || this.generation !== generation) return;
      if (source.readyState === SOURCE_CONNECTING) {
        // The browser retries on its own and sends Last-Event-ID; keep the
        // last snapshot and give the retry the same time to deliver.
        this.state = 'connecting';
        this.arm(this.connectTimeoutMs, () =>
          this.fail(source, 'Live updates are not reconnecting.'),
        );
        return;
      }
      if (source.readyState === SOURCE_CLOSED) this.fail(source);
    };
  }

  /** Reconnects now, without waiting for the backoff. */
  retry() {
    this.attempts = 0;
    this.connect();
  }

  close() {
    this.generation++;
    this.stopSource();
    this.state = 'disconnected';
  }

  private track(event: MessageEvent) {
    if (event.lastEventId) this.lastEventId = event.lastEventId;
  }

  /** Live until a snapshot is overdue; then stale; then reopen the stream. */
  private armStale(source: LiveSource) {
    this.arm(this.staleAfterMs, () => {
      if (this.source !== source) return;
      this.state = 'stale';
      this.arm(Math.max(0, this.quietReconnectMs - this.staleAfterMs), () =>
        this.fail(source, 'Live updates stopped arriving.'),
      );
    });
  }

  private arm(delay: number, then: () => void) {
    if (this.watchdog != null) clearTimeout(this.watchdog);
    this.watchdog = setTimeout(() => {
      this.watchdog = null;
      then();
    }, delay);
  }

  /** The stream is gone: close it, then back off and reopen. */
  private fail(source: LiveSource, message?: string) {
    if (this.source !== source) return;
    this.stopSource();
    this.state = 'disconnected';
    this.error =
      message ?? 'Live updates are disconnected. Retrying automatically.';
    this.scheduleReconnect(this.generation);
  }

  /** Waits out the backoff, checks the session, then opens a new stream. */
  private scheduleReconnect(generation: number) {
    const delay = Math.min(
      this.backoffBaseMs * 2 ** this.attempts,
      this.backoffMaxMs,
    );
    this.attempts++;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      void this.resume(generation);
    }, delay);
  }

  private async resume(generation: number) {
    let active: boolean;
    try {
      active = await this.checkSession();
    } catch {
      // The server is unreachable, which says nothing about the session.
      if (this.generation === generation) this.scheduleReconnect(generation);
      return;
    }
    if (this.generation !== generation) return;
    if (!active) {
      this.state = 'unauthorized';
      this.error = 'Your session expired. Sign in again.';
      return;
    }
    this.connect();
  }

  private stopSource() {
    this.source?.close();
    this.source = null;
    if (this.reconnectTimer != null) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    if (this.watchdog != null) clearTimeout(this.watchdog);
    this.watchdog = null;
  }
}

export async function sessionActive(fetcher = fetch): Promise<boolean> {
  const response = await fetcher('/api/v1/session', {
    credentials: 'same-origin',
  });
  if (response.status === 204) return true;
  if (response.status === 401) return false;
  throw new Error(`Session check failed with ${response.status}.`);
}
