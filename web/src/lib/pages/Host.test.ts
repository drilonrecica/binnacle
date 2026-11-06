// @vitest-environment jsdom
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/svelte';
import type { LiveSnapshot, LiveStore } from '../live.svelte';
import { listEvents } from '../api/events';
import { annotationTypeFilter } from '../annotations';
import { router } from '../router.svelte';
import { FakeLive } from '../shell/testing/fake-live.svelte';
import { polyfillDom } from '../ui/testing';

vi.mock('../api/events', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/events')>()),
  listEvents: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/metrics')>()),
  fetchMetrics: vi.fn().mockResolvedValue({
    resolution: '10s',
    series: [],
    gaps: [],
  }),
}));
// uPlot needs a canvas and ResizeObserver, which jsdom lacks.
vi.mock('../ui/Chart.svelte', async () => ({
  default: (await import('../charts/ChartStub.test.svelte')).default,
}));
vi.mock('../api/alerts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/alerts')>()),
  listRules: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/processes', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/processes')>()),
  sampleProcesses: vi.fn().mockResolvedValue({
    sampled: true,
    processes: [
      { pid: 7, command: 'app --password=[REDACTED]', cpuPct: 1, user: 'app' },
    ],
  }),
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

const { default: Host } = await import('./Host.svelte');

const at = new Date();
const snapshot: LiveSnapshot = {
  seq: 1,
  ts: at.toISOString(),
  bootIdentity: '6f1c2a9e-boot',
  host: {
    cpuPct: 12,
    memoryUsedBytes: 2 * 1024 ** 3,
    memoryTotalBytes: 8 * 1024 ** 3,
    swapUsedBytes: 0,
    swapTotalBytes: 1024 ** 3,
    uptimeSeconds: 3 * 86_400 + 3600,
  },
  resources: [],
  collectors: {
    host: { state: 'healthy', freshAt: at.toISOString() },
    docker: { state: 'degraded', reason: 'Docker API responded slowly' },
  },
  filesystems: [
    {
      mountKey: '/',
      mountPoint: '/',
      fsType: 'ext4',
      role: 'root',
      usedPct: 61,
      usedBytes: 61,
      totalBytes: 100,
      availableBytes: 39,
      trend: {
        state: 'filling',
        daysToFull: 12,
        fullAt: null,
        confidence: 0.8,
        windowHours: 24,
      },
    },
  ],
};

function live() {
  const store = new FakeLive();
  store.snapshot = structuredClone(snapshot);
  store.state = 'live';
  store.lastReceivedAt = at.toISOString();
  return store as unknown as LiveStore;
}

function open(path: string) {
  history.replaceState({}, '', path);
  router.sync();
  return render(Host, { live: live() });
}

beforeAll(() => {
  polyfillDom();
  window.scrollTo = () => {};
});

describe('Server page', () => {
  it('shows uptime and the boot id in a copyable chip in the title row', async () => {
    open('/server');
    expect(
      await screen.findByRole('button', {
        name: 'boot 6f1c2a9e-boot, copy the boot id',
      }),
    ).toBeTruthy();
    expect(screen.getByText('3d 1h', { exact: false })).toBeTruthy();
  });

  it('has instruments for CPU, memory, swap and each filesystem with time to full', async () => {
    open('/server');
    const section = await screen.findByTestId('instruments');
    const links = within(section).getAllByRole('link');
    expect(links.map((link) => link.getAttribute('aria-label'))).toEqual([
      expect.stringMatching(/^CPU 12%/),
      expect.stringMatching(/^Memory 2 GiB/),
      expect.stringMatching(/^Swap/),
      expect.stringMatching(/^Root disk \/ .*full in ~12 d/),
    ]);
  });

  it('asks the events API for the backend marker types only (UI-11)', async () => {
    open('/server');
    await waitFor(() => expect(listEvents).toHaveBeenCalled());
    const [query] = vi.mocked(listEvents).mock.calls[0];
    expect(query.type).toBe(annotationTypeFilter);
    expect(query.type?.split(',')).toContain('host_reboot');
    expect(query.type?.split(',')).not.toContain('boot');
  });

  it('lists filesystems with time to full', async () => {
    open('/server?tab=filesystems');
    const table = await screen.findByRole('table', { name: 'Filesystems' });
    expect(within(table).getByText('Time to full')).toBeTruthy();
    expect(within(table).getByText('full in ~12 d')).toBeTruthy();
    expect(within(table).getByText('Root disk')).toBeTruthy();
  });

  it('marks redacted parts of command lines', async () => {
    const { container } = open('/server?tab=processes');
    await screen.findByText('7');
    const redacted = container.querySelector('td .redacted');
    expect(redacted?.textContent).toBe('[REDACTED]');
  });

  it('gives every collector a state word and a reason, never only a dot', async () => {
    open('/server?tab=collectors');
    const panel = await screen.findByRole('tabpanel');
    expect(within(panel).getByText('Docker collector')).toBeTruthy();
    expect(panel.textContent).toContain('Degraded');
    expect(panel.textContent).toContain('Docker API responded slowly');
    expect(panel.textContent).toContain('Operating normally');
    // The degraded collector is named with its state in the title row too.
    expect(screen.getByText('Docker collector degraded')).toBeTruthy();
  });
});
