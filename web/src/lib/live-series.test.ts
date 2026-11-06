import { describe, expect, it, vi } from 'vitest';
import type { LiveSnapshot } from './live.svelte';
import {
  defaultWindowMs,
  LiveSeriesStore,
  reseedGapMs,
  sparklineSamples,
} from './live-series.svelte';

const start = Date.parse('2026-11-06T10:00:00Z');

function snapshot(at: number, cpu = 10): LiveSnapshot {
  return {
    seq: at,
    ts: new Date(at).toISOString(),
    bootIdentity: 'b',
    host: { cpuPct: cpu },
    resources: [{ id: 'r1', name: 'web', status: 'running', cpuHostPct: cpu }],
    collectors: {},
  };
}

describe('LiveSeriesStore buffers (UI-6)', () => {
  it('holds a full hour of 2 s snapshots, not half of it', () => {
    const store = new LiveSeriesStore();
    const count = defaultWindowMs / 2_000 + 50;
    for (let index = 0; index < count; index++)
      store.push(snapshot(start + index * 2_000));
    const newest = start + (count - 1) * 2_000;
    const hour = store.hostWindow('cpu', defaultWindowMs, newest);
    expect(hour[0]?.at).toBe(newest - defaultWindowMs);
    expect(hour).toHaveLength(defaultWindowMs / 2_000 + 1);
    // Nothing older than the window is kept.
    expect(store.host.cpu[0]?.at).toBe(newest - defaultWindowMs);
  });

  it('widens with the window it is asked for and stays bounded', () => {
    const store = new LiveSeriesStore();
    store.ensureWindow(3 * 3_600_000);
    expect(store.retentionMs).toBe(3 * 3_600_000);
    store.ensureWindow(60_000);
    expect(store.retentionMs).toBe(3 * 3_600_000);
    store.ensureWindow(48 * 3_600_000);
    expect(store.retentionMs).toBe(6 * 3_600_000);
    // Even at one snapshot per 100 ms the count stays capped.
    const fast = new LiveSeriesStore();
    for (let index = 0; index < fast.capacity + 500; index++)
      fast.push(snapshot(start + index * 100));
    expect(fast.host.cpu.length).toBe(fast.capacity);
  });

  it('ignores a repeated or older snapshot', () => {
    const store = new LiveSeriesStore();
    store.push(snapshot(start + 2_000));
    store.push(snapshot(start + 2_000));
    store.push(snapshot(start));
    expect(store.host.cpu).toHaveLength(1);
  });
});

describe('server seeding and merging', () => {
  it('places sparkline slots at their middles on the live time axis', () => {
    expect(sparklineSamples([1, null, 3], start, start + 150_000, 60)).toEqual([
      { at: start + 30_000, value: 1 },
      { at: start + 90_000, value: null },
      { at: start + 150_000, value: 3 },
    ]);
  });

  it('seeds a resource hour from the server and keeps newer live samples', () => {
    const store = new LiveSeriesStore();
    const to = start + defaultWindowMs;
    store.push(snapshot(to + 2_000, 50));
    store.seedResources({
      from: new Date(start).toISOString(),
      to: new Date(to).toISOString(),
      stepSeconds: 60,
      resources: { r1: { cpu: Array.from({ length: 60 }, () => 20) } },
    });
    const line = store.resourceSparkline(
      'r1',
      'cpu',
      defaultWindowMs,
      to + 2_000,
    );
    expect(line).toHaveLength(60);
    // The whole hour is drawn, not only the last two minutes of live samples.
    expect(line.filter((value) => value != null).length).toBeGreaterThan(55);
    expect(line.at(-1)).toBeGreaterThan(20);
    expect(store.resources.r1?.cpu.at(-1)).toEqual({
      at: to + 2_000,
      value: 50,
    });
  });

  it('seeds once, and again after a gap in the stream', async () => {
    const store = new LiveSeriesStore();
    const fetchSparklines = vi.fn().mockResolvedValue({
      from: new Date(start).toISOString(),
      to: new Date(start + 1).toISOString(),
      stepSeconds: 60,
      resources: {},
    });
    store.push(snapshot(start));
    await store.ensureResourcesSeeded(fetchSparklines);
    store.push(snapshot(start + 2_000));
    await store.ensureResourcesSeeded(fetchSparklines);
    expect(fetchSparklines).toHaveBeenCalledTimes(1);
    expect(fetchSparklines).toHaveBeenCalledWith(defaultWindowMs);
    store.push(snapshot(start + 2_000 + reseedGapMs + 1));
    await store.ensureResourcesSeeded(fetchSparklines);
    expect(fetchSparklines).toHaveBeenCalledTimes(2);
  });

  it('tries again after a failed seed', async () => {
    const store = new LiveSeriesStore();
    const fetchHistory = vi
      .fn()
      .mockRejectedValueOnce(new Error('down'))
      .mockResolvedValue({ series: [] });
    await store.ensureHostSeeded(fetchHistory, null);
    await store.ensureHostSeeded(fetchHistory, null);
    await store.ensureHostSeeded(fetchHistory, null);
    expect(fetchHistory).toHaveBeenCalledTimes(2);
  });

  it('forgets everything on reset', () => {
    const store = new LiveSeriesStore();
    store.push(snapshot(start));
    store.reset();
    expect(store.host.cpu).toEqual([]);
    expect(store.resources).toEqual({});
    store.push(snapshot(start));
    expect(store.host.cpu).toHaveLength(1);
  });
});
