// @vitest-environment jsdom
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest';
import { flushSync } from 'svelte';
import { cleanup, render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import type { HistoricalEvent } from '../api/events';
import { LiveStore } from '../live.svelte';
import { router } from '../router.svelte';
import { polyfillDom } from '../ui/testing';

vi.mock('../api/events', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/events')>()),
  listEvents: vi.fn(),
}));
const cap = vi.hoisted(() => ({ value: 5 }));
vi.mock('../activity/timeline', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../activity/timeline')>()),
  get timelineCap() {
    return cap.value;
  },
}));

const { default: Activity } = await import('./Activity.svelte');
const eventsApi = await import('../api/events');

function event(id: string, minute: number, summary = `event ${id}`) {
  return {
    id,
    ts: new Date(Date.now() - (60 - minute) * 60_000).toISOString(),
    type: `t_${id}`,
    severity: 'info',
    summary,
    resourceId: 'res1',
    source: 'docker',
  } satisfies HistoricalEvent;
}

function visit(path: string) {
  history.replaceState({}, '', path);
  router.sync();
}

beforeAll(() => {
  polyfillDom();
  window.scrollTo = () => {};
});

beforeEach(() => {
  visit('/changes');
  cap.value = 5;
  vi.mocked(eventsApi.listEvents).mockReset();
});

afterEach(() => cleanup());

describe('Changes page (UI-14)', () => {
  it('shows a live event once when history returns its stored copy', async () => {
    let resolve: (rows: HistoricalEvent[]) => void = () => {};
    vi.mocked(eventsApi.listEvents).mockReturnValue(
      new Promise((done) => (resolve = done)),
    );
    const live = new LiveStore();
    render(Activity, { live });
    // The event arrives on the stream while the history request is open.
    live.events = [
      {
        id: '01JLIVE',
        seq: 1,
        at: new Date().toISOString(),
        type: 'container_oom',
        message: 'worker was OOM killed',
        resourceId: 'res1',
      },
    ];
    flushSync();
    await screen.findByText('worker was OOM killed');
    resolve([{ ...event('01JLIVE', 59, 'worker was OOM killed') }]);
    await waitFor(() => expect(eventsApi.listEvents).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getAllByText('worker was OOM killed')).toHaveLength(1),
    );
    expect(screen.queryByText(/live/i, { selector: '.source' })).toBeNull();
  });

  it('aborts the open request for older events on leaving', async () => {
    cap.value = 1000;
    const signals: AbortSignal[] = [];
    vi.mocked(eventsApi.listEvents).mockImplementation((query, signal) => {
      if (signal) signals.push(signal);
      if (query.before) return new Promise(() => {});
      return Promise.resolve(
        Array.from({ length: 100 }, (_, i) => event(`A${i}`, i % 60)),
      );
    });
    const { unmount } = render(Activity, { live: new LiveStore() });
    await userEvent.click(
      await screen.findByRole('button', { name: /Load older events/ }),
    );
    expect(signals).toHaveLength(2);
    expect(eventsApi.listEvents).toHaveBeenLastCalledWith(
      expect.objectContaining({ before: expect.stringMatching(/^A/) }),
      signals[1],
    );
    unmount();
    expect(signals.map((signal) => signal.aborted)).toEqual([true, true]);
  });

  it('aborts the open request when the filters change', async () => {
    const signals: AbortSignal[] = [];
    vi.mocked(eventsApi.listEvents).mockImplementation((_query, signal) => {
      if (signal) signals.push(signal);
      return signals.length === 1
        ? new Promise(() => {})
        : Promise.resolve([event('B1', 59)]);
    });
    render(Activity, { live: new LiveStore() });
    await waitFor(() => expect(signals).toHaveLength(1));
    await userEvent.click(screen.getByRole('button', { name: '1h' }));
    await screen.findByText('event B1');
    expect(signals.map((signal) => signal.aborted)).toEqual([true, false]);
  });

  it('says when the list is capped and stops loading older events', async () => {
    vi.mocked(eventsApi.listEvents).mockResolvedValue(
      Array.from({ length: 100 }, (_, i) => event(`C${i}`, i % 60)),
    );
    render(Activity, { live: new LiveStore() });
    await screen.findByText(/Showing the newest 5 events/);
    expect(screen.queryByRole('button', { name: /Load older/ })).toBeNull();
  });

  it('filters live events by the same family table as the query', async () => {
    visit('/changes?family=health');
    vi.mocked(eventsApi.listEvents).mockResolvedValue([]);
    const live = new LiveStore();
    render(Activity, { live });
    await waitFor(() =>
      expect(eventsApi.listEvents).toHaveBeenCalledWith(
        expect.objectContaining({ family: 'health' }),
        expect.any(AbortSignal),
      ),
    );
    await screen.findByText('No events match');
    const at = new Date().toISOString();
    live.events = [
      { id: 'L1', seq: 1, at, type: 'container_oom', message: 'oom kill' },
      { id: 'L2', seq: 2, at, type: 'container_start', message: 'started' },
      // Not in the catalogue: matched by prefix before, now by no family.
      { id: 'L3', seq: 3, at, type: 'container_replacement', message: 'swap' },
    ];
    flushSync();
    await screen.findByText('oom kill');
    expect(screen.queryByText('started')).toBeNull();
    expect(screen.queryByText('swap')).toBeNull();
  });

  it('ignores an unknown family in the URL', async () => {
    visit('/changes?family=containers');
    vi.mocked(eventsApi.listEvents).mockResolvedValue([]);
    render(Activity, { live: new LiveStore() });
    await waitFor(() =>
      expect(eventsApi.listEvents).toHaveBeenCalledWith(
        expect.objectContaining({ family: undefined }),
        expect.any(AbortSignal),
      ),
    );
  });
});
