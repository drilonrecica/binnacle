// @vitest-environment jsdom
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render, screen, waitFor } from '@testing-library/svelte';
import type { LiveSnapshot, LiveStore } from '../live.svelte';
import { listIncidents } from '../api/incidents';
import { FakeLive } from '../shell/testing/fake-live.svelte';
import { polyfillDom } from '../ui/testing';

vi.mock('../api/incidents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/incidents')>()),
  listIncidents: vi.fn(),
}));
vi.mock('../api/alerts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/alerts')>()),
  listRules: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/events', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/events')>()),
  listEvents: vi.fn().mockResolvedValue([]),
}));

const { default: Overview } = await import('./Overview.svelte');

const at = new Date();
const snapshot: LiveSnapshot = {
  seq: 1,
  ts: at.toISOString(),
  bootIdentity: 'boot-7f3a9c1e',
  host: {
    cpuPct: 31,
    memoryUsedBytes: 4 * 1024 ** 3,
    memoryTotalBytes: 8 * 1024 ** 3,
    memoryPct: 50,
    uptimeSeconds: 86_400,
  },
  resources: [
    {
      id: 'res_01HZXQ9ABC',
      name: 'api.production',
      status: 'healthy',
      project: 'shop',
      lastSeenAt: at.toISOString(),
    },
  ],
  collectors: { docker: { state: 'healthy' } },
  filesystems: [
    {
      mountKey: '/',
      mountPoint: '/',
      fsType: 'ext4',
      role: 'root',
      usedBytes: 40 * 1024 ** 3,
      totalBytes: 100 * 1024 ** 3,
      usedPct: 40,
      trend: {
        state: 'filling',
        daysToFull: 12,
        fullAt: null,
        confidence: 0.9,
        windowHours: 24,
      },
    },
  ],
};

function liveStore(state: FakeLive['state'], lastReceivedAt: string | null) {
  const live = new FakeLive();
  live.snapshot = structuredClone(snapshot);
  live.state = state;
  live.lastReceivedAt = lastReceivedAt;
  return live as unknown as LiveStore;
}

beforeAll(() => {
  polyfillDom();
  window.scrollTo = () => {};
});

beforeEach(() => {
  vi.mocked(listIncidents).mockReset();
});

describe('Headroom page', () => {
  it('shows the all-clear only with live data and loaded incidents', async () => {
    vi.mocked(listIncidents).mockResolvedValue([]);
    render(Overview, { live: liveStore('live', new Date().toISOString()) });
    expect(await screen.findByTestId('attention-clear')).toBeTruthy();
    expect(screen.queryByTestId('attention-unknown')).toBeNull();
  });

  it('says status unknown instead of all-clear while the stream is stale (UI-1)', async () => {
    vi.mocked(listIncidents).mockResolvedValue([]);
    const last = new Date(Date.now() - 5 * 60_000).toISOString();
    render(Overview, { live: liveStore('stale', last) });
    const chip = await screen.findByTestId('attention-unknown');
    expect(chip.textContent).toMatch(/Status unknown since/);
    expect(chip.textContent).toMatch(/last update/);
    expect(screen.queryByTestId('attention-clear')).toBeNull();
    expect(screen.getByTestId('instruments').classList.contains('stale')).toBe(
      true,
    );
  });

  it('says status unknown when incidents failed to load (UI-1)', async () => {
    vi.mocked(listIncidents).mockRejectedValue(new Error('boom'));
    render(Overview, { live: liveStore('live', new Date().toISOString()) });
    const chip = await screen.findByTestId('attention-unknown');
    expect(chip.textContent).toMatch(/Incidents could not be loaded/);
    expect(screen.queryByTestId('attention-clear')).toBeNull();
  });

  it('draws gauges with the estimator trend and no raw ids or boot hash (UI-18)', async () => {
    vi.mocked(listIncidents).mockResolvedValue([
      {
        id: 'inc-1',
        title: 'resource incident on res_01HZXQ9ABC',
        status: 'open',
        severity: 'warning',
        targetType: 'resource',
        targetId: 'res_01HZXQ9ABC',
        alertCount: 1,
        firingAlertCount: 1,
        openedAt: new Date().toISOString(),
      },
    ]);
    const { container } = render(Overview, {
      live: liveStore('live', new Date().toISOString()),
    });
    await waitFor(() =>
      expect(
        screen.getByText('Warning incident · 1 alert firing'),
      ).toBeTruthy(),
    );
    flushSync();
    expect(
      screen.getByRole('link', {
        name: 'Root disk / 40 GiB of 100 GiB, full in ~12 d, 40% used',
      }),
    ).toBeTruthy();
    expect(screen.getByRole('meter', { name: 'CPU' })).toBeTruthy();
    const text = container.textContent ?? '';
    expect(text).not.toContain('boot-7f3a9c1e');
    expect(text).not.toContain('res_01HZXQ9ABC');
  });

  it('states that live data failed instead of loading forever (UI-7)', () => {
    const live = new FakeLive();
    live.state = 'disconnected';
    live.error = 'Live updates did not start.';
    render(Overview, { live: live as unknown as LiveStore });
    expect(screen.getByRole('alert').textContent).toMatch(
      /Live data could not be loaded/,
    );
  });
});
