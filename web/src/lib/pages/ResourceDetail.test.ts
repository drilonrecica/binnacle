// @vitest-environment jsdom
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import type { LiveSnapshot, LiveStore } from '../live.svelte';
import { deleteSilence } from '../api/silences';
import { router } from '../router.svelte';
import { FakeLive } from '../shell/testing/fake-live.svelte';
import { polyfillDom } from '../ui/testing';

vi.mock('../api/incidents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/incidents')>()),
  listIncidents: vi.fn().mockResolvedValue([
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
  ]),
  listAlerts: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/silences', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/silences')>()),
  listSilences: vi.fn().mockResolvedValue([
    {
      id: 'sil-1',
      scopeType: 'resource',
      scopeId: 'res_01HZXQ9ABC',
      reason: 'Planned maintenance',
      startsAt: new Date(Date.now() - 60_000).toISOString(),
      endsAt: new Date(Date.now() + 3_600_000).toISOString(),
    },
  ]),
  deleteSilence: vi.fn().mockResolvedValue(undefined),
}));
vi.mock('../api/events', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/events')>()),
  listEvents: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/metrics')>()),
  fetchMetrics: vi.fn(() => new Promise(() => {})),
}));

// uPlot reads matchMedia when it loads; jsdom has none.
window.matchMedia ??= (query: string) =>
  ({
    matches: false,
    media: query,
    addEventListener() {},
    removeEventListener() {},
    addListener() {},
    removeListener() {},
    onchange: null,
    dispatchEvent: () => false,
  }) as MediaQueryList;

const { default: ResourceDetail } = await import('./ResourceDetail.svelte');

const at = new Date();
const snapshot: LiveSnapshot = {
  seq: 1,
  ts: at.toISOString(),
  bootIdentity: 'boot-1',
  host: { cpuPct: 10, memoryUsedBytes: 1, memoryTotalBytes: 2, memoryPct: 50 },
  resources: [
    {
      id: 'res_01HZXQ9ABC',
      name: 'api.production',
      status: 'healthy',
      project: 'shop',
      lastSeenAt: at.toISOString(),
      cpuHostPct: 12,
      memoryBytes: 256 * 1024 ** 2,
      rxBps: 1024,
      txBps: 4 * 1024 ** 2,
      blockReadBps: 0,
      blockWriteBps: 2048,
      components: [],
    },
  ],
  collectors: { docker: { state: 'healthy' } },
  filesystems: [],
};

function live() {
  const store = new FakeLive();
  store.snapshot = structuredClone(snapshot);
  store.state = 'live';
  return store as unknown as LiveStore;
}

beforeAll(() => {
  polyfillDom();
  window.scrollTo = () => {};
});

beforeEach(() => {
  vi.mocked(deleteSilence).mockClear();
  localStorage.clear();
});

describe('Resource detail', () => {
  it('has the five sections of the design spec and remembers the resource for /logs', async () => {
    history.replaceState({}, '', '/resources/res_01HZXQ9ABC?tab=changes');
    router.sync();
    render(ResourceDetail, { live: live(), id: 'res_01HZXQ9ABC' });
    const tabs = await screen.findAllByRole('tab');
    expect(
      tabs.map((tab) => tab.textContent?.replace(/\d+/g, '').trim()),
    ).toEqual(['Metrics', 'Containers', 'Incidents', 'Changes', 'Logs']);
    expect(localStorage.getItem('binnacle.logs.resource')).toBe(
      'res_01HZXQ9ABC',
    );
  });

  it('labels both I/O directions without dangling arrows (UI-18)', async () => {
    history.replaceState({}, '', '/resources/res_01HZXQ9ABC?tab=changes');
    router.sync();
    const { container } = render(ResourceDetail, {
      live: live(),
      id: 'res_01HZXQ9ABC',
    });
    await screen.findAllByRole('tab');
    const text = container.textContent ?? '';
    expect(text).not.toMatch(/↓ receive/);
    expect(text).toMatch(/in · 4 MiB\/s out/);
    expect(text).toMatch(/read · 2 KiB\/s written/);
  });

  it('opens the incidents tab from a retired link and names the resource in titles', async () => {
    history.replaceState({}, '', '/resources/res_01HZXQ9ABC?tab=alerts');
    router.sync();
    const { container } = render(ResourceDetail, {
      live: live(),
      id: 'res_01HZXQ9ABC',
    });
    const link = await screen.findByRole('link', {
      name: 'Resource incident on api.production',
    });
    expect(link.getAttribute('href')).toBe('/incidents/inc-1');
    expect(location.search).toBe('?tab=incidents');
    // The id appears once, in the copyable chip, and nowhere else.
    const text = container.textContent ?? '';
    expect(text.split('res_01HZXQ9ABC')).toHaveLength(2);
  });

  it('asks before ending a silence', async () => {
    history.replaceState({}, '', '/resources/res_01HZXQ9ABC?tab=incidents');
    router.sync();
    render(ResourceDetail, { live: live(), id: 'res_01HZXQ9ABC' });
    await fireEvent.click(
      await screen.findByRole('button', { name: 'End now…' }),
    );
    expect(deleteSilence).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/Notifications for api\.production resume now/),
    ).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'End silence' }));
    await waitFor(() => expect(deleteSilence).toHaveBeenCalledWith('sil-1'));
  });
});
