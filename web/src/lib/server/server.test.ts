import { describe, expect, it } from 'vitest';
import type { LiveSnapshot } from '../live.svelte';
import { gaugeThresholds } from '../overview/headroom';
import {
  chartFocus,
  collectorViews,
  commandParts,
  serverFilesystems,
  serverInstruments,
} from './server';

const filling = {
  state: 'filling' as const,
  daysToFull: 3.2,
  fullAt: null,
  confidence: 0.9,
  windowHours: 24,
};

function snapshot(extra: Partial<LiveSnapshot> = {}): LiveSnapshot {
  return {
    seq: 1,
    ts: '2026-11-06T10:00:00Z',
    bootIdentity: 'boot-1',
    host: {
      cpuPct: 20,
      memoryUsedBytes: 4 * 1024 ** 3,
      memoryTotalBytes: 8 * 1024 ** 3,
      swapUsedBytes: 1024 ** 3,
      swapTotalBytes: 2 * 1024 ** 3,
    },
    resources: [],
    collectors: {},
    filesystems: Array.from({ length: 9 }, (_, index) => ({
      mountKey: `m${index}`,
      mountPoint: index === 0 ? '/' : `/mnt/${index}`,
      fsType: 'ext4',
      role: index === 0 ? ('root' as const) : ('other' as const),
      usedPct: 10 + index,
      usedBytes: 10,
      totalBytes: 100,
      trend: index === 0 ? filling : null,
    })),
    ...extra,
  };
}

describe('server instruments', () => {
  it('shows CPU, memory, swap and every filesystem, uncapped', () => {
    const instruments = serverInstruments(snapshot(), gaugeThresholds([]), []);
    expect(instruments.map((item) => item.key).slice(0, 4)).toEqual([
      'cpu',
      'memory',
      'swap',
      'fs:m0',
    ]);
    // Headroom stops at eight; Server shows all nine disks.
    expect(instruments).toHaveLength(12);
    expect(instruments[2]!.value).toBe(50);
    expect(instruments[3]!.trend).toEqual({
      text: 'full in ~3 d',
      tone: 'critical',
    });
  });

  it('says when swap is not configured instead of showing zero', () => {
    const value = snapshot();
    value.host.swapTotalBytes = 0;
    const swap = serverInstruments(value, gaugeThresholds([]), [])[2]!;
    expect(swap.value).toBeNull();
    expect(swap.caption).toBe('no swap configured');
  });

  it('falls back to the root disk reported on the host object', () => {
    const value = snapshot({ filesystems: [] });
    value.host.diskUsedBytes = 30;
    value.host.diskTotalBytes = 120;
    const root = serverFilesystems(value)[0]!;
    expect(root).toMatchObject({ mountPoint: '/', usedPct: 25 });
    expect(root.availableBytes).toBe(90);
  });

  it('maps metric links to chart groups', () => {
    expect(chartFocus('cpu')).toBe('cpu');
    expect(chartFocus('swap')).toBe('memory');
    expect(chartFocus('nope')).toBe('');
  });
});

describe('collector views', () => {
  it('give every collector a state word and a reason in a fixed order', () => {
    const views = collectorViews({
      coolify: { state: 'healthy' },
      docker: { state: 'degraded', reason: 'API responded slowly' },
      host: { state: 'down' },
    });
    expect(views.map((view) => [view.title, view.word, view.reason])).toEqual([
      ['Host collector', 'Down', 'No reason reported'],
      ['Docker collector', 'Degraded', 'API responded slowly'],
      ['Coolify collector', 'Healthy', 'Operating normally'],
    ]);
  });
});

describe('command parts', () => {
  it('sets the server redaction markers apart', () => {
    expect(commandParts('app --token [REDACTED] -p [REDACTED]x')).toEqual([
      { text: 'app --token ', redacted: false },
      { text: '[REDACTED]', redacted: true },
      { text: ' -p ', redacted: false },
      { text: '[REDACTED]', redacted: true },
      { text: 'x', redacted: false },
    ]);
    expect(commandParts('nginx')).toEqual([{ text: 'nginx', redacted: false }]);
  });
});
