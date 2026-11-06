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
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import type { LogEntry } from '../api/logs';
import { fetchLogs, followLogs } from '../api/logs';
import { polyfillDom } from '../ui/testing';
import { router } from '../router.svelte';

vi.mock('../api/logs', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/logs')>()),
  fetchLogs: vi.fn(),
  followLogs: vi.fn(),
}));

const { default: LogsTab } = await import('./LogsTab.svelte');

function entry(n: number): LogEntry {
  return {
    timestamp: new Date(1_700_000_000_000 + n * 1000).toISOString(),
    component: 'c1',
    stream: 'stdout',
    severity: 'info',
    message: `line ${n}`,
  };
}

beforeAll(() => {
  polyfillDom();
  window.scrollTo = () => {};
});

beforeEach(() => {
  history.replaceState({}, '', '/resources/res1?tab=logs');
  router.sync();
  vi.mocked(fetchLogs).mockReset();
  vi.mocked(followLogs).mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('Logs tab', () => {
  it('loads this resource on open and aborts the request when closed (UI-15)', async () => {
    let signal: AbortSignal | undefined;
    vi.mocked(fetchLogs).mockImplementation((query, given) => {
      signal = given;
      expect(query.resource).toBe('res1');
      return new Promise(() => {});
    });
    const view = render(LogsTab, { resourceId: 'res1' });
    await waitFor(() => expect(fetchLogs).toHaveBeenCalledTimes(1));
    expect(signal?.aborted).toBe(false);
    view.unmount();
    expect(signal?.aborted).toBe(true);
  });

  it('aborts the previous request when loading again', async () => {
    const signals: AbortSignal[] = [];
    vi.mocked(fetchLogs).mockImplementation((_query, given) => {
      if (given) signals.push(given);
      return new Promise(() => {});
    });
    render(LogsTab, { resourceId: 'res1' });
    await waitFor(() => expect(signals).toHaveLength(1));
    await fireEvent.keyDown(
      screen.getByRole('searchbox', { name: /Search log lines/ }),
      { key: 'Enter' },
    );
    await waitFor(() => expect(signals).toHaveLength(2));
    expect(signals[0]?.aborted).toBe(true);
    expect(signals[1]?.aborted).toBe(false);
  });

  it('renders a window of the newest lines and reveals earlier ones on request', async () => {
    vi.mocked(fetchLogs).mockResolvedValue({
      entries: Array.from({ length: 1_000 }, (_, n) => entry(n)),
      truncated: false,
      redaction: 'best-effort',
    });
    const { container } = render(LogsTab, { resourceId: 'res1' });
    await waitFor(() =>
      expect(container.querySelectorAll('li.line')).toHaveLength(400),
    );
    expect(container.textContent).toContain('line 999');
    // The server always reports "best-effort"; the note says it once.
    expect(container.textContent).toContain('Redaction is best-effort;');
    expect(container.textContent).not.toContain('line 599 ');
    screen
      .getByRole('button', { name: /Show earlier lines \(600 more\)/ })
      .click();
    await waitFor(() =>
      expect(container.querySelectorAll('li.line')).toHaveLength(800),
    );
  });

  it('batches streamed lines into a bounded ring (UI-15)', async () => {
    let push: ((entry: LogEntry) => void) | undefined;
    const close = vi.fn();
    vi.mocked(followLogs).mockImplementation((_query, handlers) => {
      push = handlers.onentry;
      return close;
    });
    vi.mocked(fetchLogs).mockResolvedValue({
      entries: [],
      truncated: false,
      redaction: '',
    });
    const view = render(LogsTab, { resourceId: 'res1' });
    await waitFor(() => expect(fetchLogs).toHaveBeenCalled());
    screen.getByRole('switch', { name: 'Follow new lines' }).click();
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Follow' })).toBeTruthy(),
    );
    screen.getByRole('button', { name: 'Follow' }).click();
    await waitFor(() => expect(push).toBeDefined());
    for (let n = 0; n < 6_000; n++) push?.(entry(n));
    await waitFor(() =>
      expect(view.container.textContent).toContain('line 5999'),
    );
    expect(view.container.querySelectorAll('li.line')).toHaveLength(400);
    expect(view.container.textContent).toMatch(
      /5000\s+lines \(newest 5000 kept\)/,
    );
    view.unmount();
    expect(close).toHaveBeenCalled();
  });
});
