import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { startPoller } from './poller';

class FakeDocument {
  visible = true;
  private listeners = new Set<() => void>();
  addEventListener(_type: string, listener: () => void) {
    this.listeners.add(listener);
  }
  removeEventListener(_type: string, listener: () => void) {
    this.listeners.delete(listener);
  }
  setVisible(value: boolean) {
    this.visible = value;
    for (const listener of this.listeners) listener();
  }
  get listenerCount() {
    return this.listeners.size;
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

describe('startPoller', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('runs now, then every interval after each run settles', async () => {
    const doc = new FakeDocument();
    const results: number[] = [];
    let calls = 0;
    const stop = startPoller({
      intervalMs: 5_000,
      run: async () => ++calls,
      onResult: (value) => results.push(value),
      onError: () => {},
      isVisible: () => doc.visible,
      events: doc as unknown as Document,
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(results).toEqual([1]);
    await vi.advanceTimersByTimeAsync(4_999);
    expect(calls).toBe(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(results).toEqual([1, 2]);
    stop();
    await vi.advanceTimersByTimeAsync(20_000);
    expect(calls).toBe(2);
    expect(doc.listenerCount).toBe(0);
  });

  it('never overlaps: a slow run delays the next one', async () => {
    const doc = new FakeDocument();
    const pending = deferred<string>();
    let calls = 0;
    const stop = startPoller({
      intervalMs: 5_000,
      run: () => {
        calls++;
        return calls === 1 ? pending.promise : Promise.resolve('next');
      },
      onResult: () => {},
      onError: () => {},
      isVisible: () => doc.visible,
      events: doc as unknown as Document,
    });
    await vi.advanceTimersByTimeAsync(30_000);
    expect(calls).toBe(1);
    pending.resolve('slow');
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(5_000);
    expect(calls).toBe(2);
    stop();
  });

  it('pauses while hidden and refreshes as soon as it is visible', async () => {
    const doc = new FakeDocument();
    let calls = 0;
    const stop = startPoller({
      intervalMs: 5_000,
      run: async () => ++calls,
      onResult: () => {},
      onError: () => {},
      isVisible: () => doc.visible,
      events: doc as unknown as Document,
    });
    await vi.advanceTimersByTimeAsync(0);
    doc.setVisible(false);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(calls).toBe(1);
    doc.setVisible(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(calls).toBe(2);
    stop();
  });

  it('reports errors and keeps polling', async () => {
    const doc = new FakeDocument();
    const errors: unknown[] = [];
    let calls = 0;
    const stop = startPoller({
      intervalMs: 5_000,
      run: async () => {
        calls++;
        if (calls === 1) throw new Error('scan_busy');
        return calls;
      },
      onResult: () => {},
      onError: (reason) => errors.push(reason),
      isVisible: () => doc.visible,
      events: doc as unknown as Document,
    });
    await vi.advanceTimersByTimeAsync(5_000);
    expect(errors).toHaveLength(1);
    expect(calls).toBe(2);
    stop();
  });

  it('aborts a run in flight on stop and drops its result', async () => {
    const doc = new FakeDocument();
    const pending = deferred<number>();
    let signal: AbortSignal | undefined;
    const results: number[] = [];
    const stop = startPoller({
      intervalMs: 5_000,
      run: (value) => {
        signal = value;
        return pending.promise;
      },
      onResult: (value) => results.push(value),
      onError: () => {},
      isVisible: () => doc.visible,
      events: doc as unknown as Document,
    });
    stop();
    expect(signal?.aborted).toBe(true);
    pending.resolve(1);
    await vi.advanceTimersByTimeAsync(0);
    expect(results).toEqual([]);
  });
});
