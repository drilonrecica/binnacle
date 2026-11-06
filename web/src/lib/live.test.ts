import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  connectTimeoutMs,
  decodeSnapshot,
  LiveStore,
  resumeURL,
  sessionActive,
  staleAfterMs,
  type LiveSource,
  type LiveStoreOptions,
} from './live.svelte';

describe('live response decoding', () => {
  it('rejects malformed payloads instead of presenting them as current', () => {
    expect(() => decodeSnapshot('{"seq":1}')).toThrow(/malformed/);
    expect(
      decodeSnapshot(
        '{"seq":1,"ts":"2026-07-11T12:00:00Z","host":{},"resources":[],"collectors":{}}',
      ).seq,
    ).toBe(1);
  });
});

/** An EventSource stand-in the test drives frame by frame. */
class FakeSource implements LiveSource {
  readyState = 0;
  onerror: ((event: Event) => void) | null = null;
  closed = false;
  private listeners = new Map<string, Array<(event: MessageEvent) => void>>();
  constructor(readonly url: string) {}
  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }
  close() {
    this.closed = true;
    this.readyState = 2;
  }
  emit(type: string, data: unknown, id = '') {
    this.readyState = 1;
    const event = { data: JSON.stringify(data), lastEventId: id };
    for (const listener of this.listeners.get(type) ?? [])
      listener(event as MessageEvent);
  }
  snapshot(seq: number, id = String(seq)) {
    this.emit(
      'snapshot',
      {
        seq,
        ts: new Date().toISOString(),
        host: {},
        resources: [],
        collectors: {},
      },
      id,
    );
  }
  event(id: string, seq: number) {
    this.emit('event', { id, seq, type: 'started', message: id }, String(seq));
  }
  /** The browser lost the stream and is retrying it on its own. */
  drop() {
    this.readyState = 0;
    this.onerror?.(new Event('error'));
  }
  /** The browser gave up (an HTTP error status or a wrong content type). */
  fail() {
    this.readyState = 2;
    this.onerror?.(new Event('error'));
  }
}

function harness(options: LiveStoreOptions = {}) {
  const sources: FakeSource[] = [];
  const session = vi.fn<() => Promise<boolean>>().mockResolvedValue(true);
  const store = new LiveStore({
    createSource: (url) => {
      const source = new FakeSource(url);
      sources.push(source);
      return source;
    },
    sessionActive: session,
    ...options,
  });
  const latest = () => {
    const source = sources.at(-1);
    if (!source) throw new Error('no source opened');
    return source;
  };
  return { store, sources, session, latest };
}

describe('LiveStore', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(Date.parse('2026-11-06T11:32:00Z'));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('is connecting until the first snapshot, then live', () => {
    const { store, latest } = harness();
    store.connect();
    expect(store.state).toBe('connecting');
    expect(latest().url).toBe('/api/v1/live');
    latest().snapshot(1, '7');
    expect(store.state).toBe('live');
    expect(store.lastEventId).toBe('7');
    expect(store.lastReceivedAt).toBe('2026-11-06T11:32:00.000Z');
  });

  it('leaves transient errors to the browser retry and keeps the snapshot', async () => {
    const { store, sources, latest } = harness();
    store.connect();
    latest().snapshot(1);
    latest().drop();
    expect(store.state).toBe('connecting');
    expect(store.snapshot?.seq).toBe(1);
    await vi.advanceTimersByTimeAsync(5_000);
    // The same source recovers; no second EventSource was created.
    latest().snapshot(2);
    expect(sources).toHaveLength(1);
    expect(sources[0]?.closed).toBe(false);
    expect(store.state).toBe('live');
  });

  it('backs off exponentially, capped, when the browser gives up', async () => {
    const { store, sources, latest } = harness({
      backoffBaseMs: 1_000,
      backoffMaxMs: 8_000,
    });
    store.connect();
    const opened: number[] = [];
    let previous = Date.now();
    for (let attempt = 0; attempt < 5; attempt++) {
      latest().fail();
      expect(store.state).toBe('disconnected');
      const count = sources.length;
      while (sources.length === count) await vi.advanceTimersByTimeAsync(250);
      opened.push(Date.now() - previous);
      previous = Date.now();
    }
    expect(opened).toEqual([1_000, 2_000, 4_000, 8_000, 8_000]);
  });

  it('resets the backoff once a snapshot arrives', async () => {
    const { store, sources, latest } = harness();
    store.connect();
    latest().fail();
    await vi.advanceTimersByTimeAsync(1_000);
    latest().fail();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(sources).toHaveLength(3);
    latest().snapshot(1);
    latest().fail();
    await vi.advanceTimersByTimeAsync(1_000);
    expect(sources).toHaveLength(4);
  });

  it('reopens with the last event id so the server replays what was missed (UI-2)', async () => {
    const { store, latest } = harness();
    store.connect();
    latest().snapshot(1, '10');
    latest().event('01J-A', 11);
    latest().fail();
    await vi.advanceTimersByTimeAsync(1_000);
    expect(latest().url).toBe('/api/v1/live?lastEventId=11');
    // The replay repeats an event already shown; it is not listed twice.
    latest().event('01J-A', 11);
    latest().event('01J-B', 12);
    expect(store.events.map((event) => event.id)).toEqual(['01J-A', '01J-B']);
    expect(store.lastEventId).toBe('12');
  });

  it('gives up on a stream that never delivers and says why (UI-7)', async () => {
    const { store, sources, latest } = harness();
    store.connect();
    await vi.advanceTimersByTimeAsync(connectTimeoutMs - 1);
    expect(store.state).toBe('connecting');
    await vi.advanceTimersByTimeAsync(1);
    expect(store.state).toBe('disconnected');
    expect(store.error).toMatch(/did not start/);
    expect(sources[0]?.closed).toBe(true);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(sources).toHaveLength(2);
    expect(latest().url).toBe('/api/v1/live');
  });

  it('gives a browser retry the same time before giving up on it', async () => {
    const { store, sources, latest } = harness();
    store.connect();
    latest().snapshot(1);
    latest().drop();
    await vi.advanceTimersByTimeAsync(connectTimeoutMs);
    expect(store.state).toBe('disconnected');
    expect(sources[0]?.closed).toBe(true);
  });

  it('turns stale with an age when snapshots stop, then reopens', async () => {
    const { store, sources, latest } = harness({ quietReconnectMs: 30_000 });
    store.connect();
    latest().snapshot(1);
    await vi.advanceTimersByTimeAsync(staleAfterMs - 1);
    expect(store.state).toBe('live');
    await vi.advanceTimersByTimeAsync(1);
    expect(store.state).toBe('stale');
    expect(store.ageMs()).toBe(staleAfterMs);
    latest().snapshot(2);
    expect(store.state).toBe('live');
    expect(store.ageMs()).toBe(0);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(store.state).toBe('disconnected');
    expect(store.error).toMatch(/stopped arriving/);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(sources).toHaveLength(2);
  });

  it('stops reconnecting and reports unauthorized when the session is gone', async () => {
    const { store, sources, session, latest } = harness();
    store.connect();
    latest().snapshot(1);
    session.mockResolvedValue(false);
    latest().fail();
    await vi.advanceTimersByTimeAsync(1_000);
    expect(session).toHaveBeenCalledTimes(1);
    expect(store.state).toBe('unauthorized');
    expect(store.error).toMatch(/session expired/);
    await vi.advanceTimersByTimeAsync(120_000);
    expect(sources).toHaveLength(1);
  });

  it('keeps retrying when the session cannot be checked, without signing out', async () => {
    const { store, sources, session, latest } = harness();
    store.connect();
    session.mockRejectedValueOnce(new Error('offline'));
    latest().fail();
    await vi.advanceTimersByTimeAsync(1_000);
    expect(store.state).toBe('disconnected');
    expect(sources).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(session).toHaveBeenCalledTimes(2);
    expect(sources).toHaveLength(2);
    expect(store.state).toBe('connecting');
  });

  it('treats a malformed snapshot as a failed stream', () => {
    const { store, latest } = harness();
    store.connect();
    latest().emit('snapshot', { seq: 'x' }, '1');
    expect(store.state).toBe('disconnected');
    expect(store.error).toMatch(/malformed/);
    expect(latest().closed).toBe(true);
  });

  it('retries now on request and cancels everything on close', async () => {
    const { store, sources, latest } = harness();
    store.connect();
    latest().fail();
    latest().fail();
    store.retry();
    expect(sources).toHaveLength(2);
    expect(store.state).toBe('connecting');
    store.close();
    expect(store.state).toBe('disconnected');
    expect(latest().closed).toBe(true);
    await vi.advanceTimersByTimeAsync(120_000);
    expect(sources).toHaveLength(2);
    expect(store.state).toBe('disconnected');
  });

  it('ignores frames and errors from a source it already replaced', () => {
    const { store, sources } = harness();
    store.connect();
    store.retry();
    sources[0]?.snapshot(9);
    sources[0]?.fail();
    expect(store.snapshot).toBeNull();
    expect(store.state).toBe('connecting');
  });
});

describe('resumeURL', () => {
  it('adds the cursor only when there is one', () => {
    expect(resumeURL('/api/v1/live', '')).toBe('/api/v1/live');
    expect(resumeURL('/api/v1/live', '42')).toBe('/api/v1/live?lastEventId=42');
    expect(resumeURL('/x?a=1', '42')).toBe('/x?a=1&lastEventId=42');
  });
});

describe('sessionActive', () => {
  const answer = (status: number) =>
    vi
      .fn()
      .mockResolvedValue(
        new Response(null, { status }),
      ) as unknown as typeof fetch;
  it('reads 204 as active and 401 as gone', async () => {
    await expect(sessionActive(answer(204))).resolves.toBe(true);
    await expect(sessionActive(answer(401))).resolves.toBe(false);
  });
  it('cannot tell from any other answer', async () => {
    await expect(sessionActive(answer(502))).rejects.toThrow(/502/);
  });
});
