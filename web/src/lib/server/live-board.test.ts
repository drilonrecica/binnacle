import { describe, expect, it } from 'vitest';
import type { LiveSnapshot } from '../live.svelte';
import {
  activityReadings,
  containerRows,
  formatCpuTime,
  loadReadings,
  memoryOfLimit,
  rwRates,
  swapReadings,
} from './live-board';

const now = Date.parse('2026-11-08T12:00:00Z');

function snapshot(): LiveSnapshot {
  return {
    seq: 1,
    ts: new Date(now).toISOString(),
    bootIdentity: 'b',
    host: {
      load1: 2,
      load5: 1.5,
      load15: 1,
      swapTotalBytes: 2048,
      swapUsedBytes: 512,
      contextSwitchesPs: 13870.4,
      procsRunning: 3,
    },
    resources: [
      {
        id: 'r1',
        name: 'shop',
        status: 'healthy',
        components: [
          {
            id: 'c1',
            name: 'shop-web',
            status: 'healthy',
            runtimeState: 'running',
            memoryBytes: 1024,
          },
          {
            id: 'c2',
            name: 'shop-worker',
            status: 'down',
            runtimeState: 'exited',
          },
        ],
      },
      { id: 'r2', name: 'empty', status: 'healthy' },
    ],
    collectors: {},
  };
}

describe('Server › Live view model', () => {
  it('flattens containers and merges their detail', () => {
    const details = new Map([
      [
        'c1',
        {
          id: 'c1',
          startedAt: '2026-11-08T11:00:00Z',
          memoryLimitBytes: 4096,
          image: 'shop:1',
          ports: ['80/tcp'],
        },
      ],
      ['c2', { id: 'c2', startedAt: '2026-11-08T10:00:00Z' }],
    ]);
    const rows = containerRows(snapshot(), now, details);
    expect(rows.map((row) => [row.name, row.resourceName])).toEqual([
      ['shop-web', 'shop'],
      ['shop-worker', 'shop'],
    ]);
    expect(rows[0]?.uptimeSeconds).toBe(3600);
    expect(rows[0]?.memoryPct).toBe(25);
    expect(rows[0]?.image).toBe('shop:1');
    // A stopped container has no uptime, even with a remembered start.
    expect(rows[1]?.uptimeSeconds).toBeNull();
    expect(rows[1]?.memoryPct).toBeNull();
    // Before the detail arrives the live columns still render.
    expect(containerRows(snapshot(), now)[0]?.memoryPct).toBeNull();
  });

  it('derives swap free and load per core', () => {
    expect(swapReadings(snapshot().host).at(-1)?.value).toBe('1.5 KiB');
    const load = loadReadings(snapshot().host, 4);
    expect(load.map((reading) => reading.value)).toEqual([
      '2.00',
      '1.50',
      '1.00',
      '0.50',
    ]);
    expect(loadReadings(snapshot().host, undefined)).toHaveLength(3);
  });

  it('formats activity rates and leaves unknowns as the missing glyph', () => {
    const readings = activityReadings(snapshot().host);
    expect(readings[0]?.value).toBe(`${(13870).toLocaleString()}/s`);
    expect(readings[1]?.value).toBe('—');
    expect(readings[3]?.value).toBe('3');
  });

  it('shows memory against the limit only when one is known', () => {
    expect(memoryOfLimit({ memoryBytes: 1024, memoryLimitBytes: 4096 })).toBe(
      '1 KiB / 4 KiB',
    );
    expect(memoryOfLimit({ memoryBytes: 1024 })).toBe('1 KiB');
    expect(rwRates(null, undefined)).toBe('—');
    expect(rwRates(2048, null)).toBe('2 KiB · —');
  });

  it('formats cumulative CPU time as h:mm:ss', () => {
    expect(formatCpuTime(1_665_724)).toBe('462:42:04');
    expect(formatCpuTime(65)).toBe('0:01:05');
    expect(formatCpuTime(null)).toBe('—');
  });
});
