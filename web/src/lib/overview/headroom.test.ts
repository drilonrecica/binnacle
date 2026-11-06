import { describe, expect, it } from 'vitest';
import type { Rule } from '../api/alerts';
import type {
  FilesystemObservation,
  LiveSnapshot,
  Trend,
} from '../live.svelte';
import {
  cpuDirection,
  gaugeThresholds,
  headroomInstruments,
  headroomResources,
  instrumentName,
  orderFilesystems,
  pinnedGroup,
  trendView,
} from './headroom';

function rule(partial: Partial<Rule>): Rule {
  return {
    id: partial.id ?? `${partial.family}-${partial.severity}`,
    family: 'host_cpu_warning',
    name: 'rule',
    builtIn: true,
    enabled: true,
    severity: 'warning',
    scopeType: 'global',
    triggerSeconds: 60,
    recoverySeconds: 60,
    suppressDuringDeployment: false,
    ...partial,
  };
}

function trend(partial: Partial<Trend>): Trend {
  return {
    state: 'unknown',
    daysToFull: null,
    fullAt: null,
    confidence: 0,
    windowHours: 24,
    ...partial,
  };
}

function mount(
  mountPoint: string,
  usedPct: number,
  extra: Partial<FilesystemObservation> = {},
): FilesystemObservation {
  return {
    mountKey: mountPoint,
    mountPoint,
    fsType: 'ext4',
    usedBytes: usedPct * 1e9,
    totalBytes: 100e9,
    usedPct,
    role: 'other',
    ...extra,
  };
}

function snapshot(partial: Partial<LiveSnapshot> = {}): LiveSnapshot {
  return {
    seq: 1,
    ts: '2026-11-06T12:00:00Z',
    bootIdentity: 'boot-abc123',
    host: {
      cpuPct: 42.4,
      memoryUsedBytes: 6 * 1024 ** 3,
      memoryTotalBytes: 8 * 1024 ** 3,
      memoryPct: 75,
    },
    resources: [],
    collectors: {},
    filesystems: [],
    ...partial,
  };
}

describe('gauge thresholds', () => {
  it('fall back to the design constants without rules', () => {
    const t = gaugeThresholds([]);
    expect(t.cpu).toEqual({ warn: 80, critical: 90 });
    expect(t.memory).toEqual({ warn: 85, critical: 95 });
    expect(t.filesystem('/')).toEqual({
      warn: 80,
      critical: 95,
      criticalDays: 7,
    });
  });

  it('take host thresholds from enabled rules by severity', () => {
    const t = gaugeThresholds([
      rule({ family: 'host_cpu_warning', severity: 'warning', threshold: 70 }),
      rule({
        family: 'host_cpu_warning',
        severity: 'critical',
        threshold: 85,
        enabled: false,
      }),
      rule({
        family: 'host_memory_warning',
        severity: 'critical',
        scopeType: 'host',
        threshold: 92,
      }),
    ]);
    expect(t.cpu).toEqual({ warn: 70, critical: 90 });
    expect(t.memory).toEqual({ warn: 85, critical: 92 });
  });

  it('prefer a rule scoped to the mount over the global one', () => {
    const t = gaugeThresholds([
      rule({ family: 'filesystem_warning', threshold: 75 }),
      rule({
        family: 'filesystem_warning',
        scopeType: 'filesystem',
        scopeId: '/data',
        threshold: 60,
      }),
      rule({ family: 'filesystem_critical', threshold: 90 }),
      rule({
        family: 'filesystem_full_within_days',
        scopeType: 'filesystem',
        scopeId: '/data',
        threshold: 14,
      }),
    ]);
    expect(t.filesystem('/data')).toEqual({
      warn: 60,
      critical: 90,
      criticalDays: 14,
    });
    expect(t.filesystem('/')).toEqual({
      warn: 75,
      critical: 90,
      criticalDays: 7,
    });
  });
});

describe('trend words', () => {
  it('reads the estimator', () => {
    expect(trendView(trend({ state: 'filling', daysToFull: 6.2 }))).toEqual({
      text: 'full in ~6 d',
      tone: 'critical',
    });
    expect(trendView(trend({ state: 'filling', daysToFull: 30 }), 7)).toEqual({
      text: 'full in ~30 d',
      tone: 'accent',
    });
    expect(trendView(trend({ state: 'filling', daysToFull: 0.4 })).text).toBe(
      'full in < 1 d',
    );
    expect(trendView(trend({ state: 'steady' })).text).toBe('steady');
    expect(trendView(trend({ state: 'shrinking' })).text).toBe('shrinking');
    expect(trendView(trend({ state: 'unknown' })).text).toBe(
      'not enough history',
    );
    expect(trendView(null).text).toBe('not enough history');
  });

  it('gives CPU a direction from the ring buffer', () => {
    const start = Date.parse('2026-11-06T11:00:00Z');
    const series = (fn: (i: number) => number) =>
      Array.from({ length: 60 }, (_, i) => ({
        at: start + i * 60_000,
        value: fn(i),
      }));
    expect(cpuDirection(series((i) => 10 + i)).text).toBe('rising');
    expect(cpuDirection(series((i) => 80 - i)).text).toBe('falling');
    expect(cpuDirection(series((i) => 30 + (i % 2)))).toEqual({
      text: 'steady',
      tone: 'muted',
    });
    // Two minutes of samples are not enough to call a direction.
    expect(cpuDirection(series((i) => i).slice(0, 3)).text).toBe(
      'not enough history',
    );
  });
});

describe('instruments', () => {
  it('order disks root, Docker data root, then by fullness', () => {
    const ordered = orderFilesystems([
      mount('/mnt/a', 40),
      mount('/var/lib/docker', 30, { role: 'docker' }),
      mount('/mnt/b', 90),
      mount('/', 50, { role: 'root' }),
    ]);
    expect(ordered.map((item) => item.mountPoint)).toEqual([
      '/',
      '/var/lib/docker',
      '/mnt/b',
      '/mnt/a',
    ]);
  });

  it('show CPU, memory and every disk up to eight instruments', () => {
    const disks = Array.from({ length: 6 }, (_, i) => mount(`/d${i}`, i));
    const set = headroomInstruments(
      snapshot({ filesystems: disks }),
      gaugeThresholds([]),
      [],
    );
    expect(set.instruments).toHaveLength(8);
    expect(set.moreDisks).toBe(0);
    expect(set.instruments.map((item) => item.key).slice(0, 2)).toEqual([
      'cpu',
      'memory',
    ]);
  });

  it('collapse disks beyond the eighth instrument into "more disks"', () => {
    const disks = Array.from({ length: 9 }, (_, i) => mount(`/d${i}`, i));
    const set = headroomInstruments(
      snapshot({ filesystems: disks }),
      gaugeThresholds([]),
      [],
    );
    // Seven instruments and the "N more disks" card make eight places.
    expect(set.instruments).toHaveLength(7);
    expect(set.moreDisks).toBe(4);
  });

  it('show used of total and the estimator trend for disks and memory', () => {
    const set = headroomInstruments(
      snapshot({
        host: {
          cpuPct: 12,
          memoryUsedBytes: 6 * 1024 ** 3,
          memoryTotalBytes: 8 * 1024 ** 3,
          memoryPct: 75,
          memoryTrend: trend({ state: 'steady' }),
        },
        filesystems: [
          mount('/', 88, {
            role: 'root',
            usedBytes: 88 * 1024 ** 3,
            totalBytes: 100 * 1024 ** 3,
            trend: trend({ state: 'filling', daysToFull: 3 }),
          }),
        ],
      }),
      gaugeThresholds([]),
      [],
    );
    const [, memory, root] = set.instruments;
    expect(memory?.valueText).toBe('6 GiB');
    expect(memory?.caption).toBe('of 8 GiB');
    expect(memory?.trend.text).toBe('steady');
    expect(root?.label).toBe('Root disk');
    expect(root?.trend).toEqual({ text: 'full in ~3 d', tone: 'critical' });
    expect(root && instrumentName(root)).toBe(
      'Root disk / 88 GiB Above 80% of 100 GiB, full in ~3 d, 88% used',
    );
    expect(set.instruments[0] && instrumentName(set.instruments[0])).toBe(
      'CPU 12%, not enough history',
    );
  });

  it('fall back to the host root disk on servers without filesystems', () => {
    const set = headroomInstruments(
      snapshot({
        host: { diskUsedBytes: 50, diskTotalBytes: 100 },
        filesystems: undefined,
      }),
      gaugeThresholds([]),
      [],
    );
    expect(set.instruments[2]?.value).toBe(50);
  });
});

describe('resource ordering', () => {
  const ts = '2026-11-06T12:00:00Z';
  const fresh = '2026-11-06T11:59:58Z';
  const res = (
    id: string,
    status: string,
    project?: string,
    lastSeenAt = fresh,
  ) => ({ id, name: id, status, project, lastSeenAt });

  it('puts the worst health first and pinned resources in their own group', () => {
    const { rows, groupOf, groupOrder } = headroomResources(
      [
        res('a', 'healthy', 'shop'),
        res('b', 'down', 'blog'),
        res('c', 'healthy', 'shop', '2026-11-06T11:00:00Z'),
        res('d', 'healthy', 'shop'),
        res('e', 'archived', 'shop'),
        res('f', 'healthy'),
      ],
      ['d'],
      ts,
    );
    expect(rows.map((row) => row.id)).toEqual(['b', 'c', 'a', 'd', 'f']);
    expect(groupOf(rows[3]!)).toBe(pinnedGroup);
    expect(groupOrder).toEqual([pinnedGroup, 'blog', 'shop', 'Ungrouped']);
  });
});
