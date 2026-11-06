// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import type { LiveStore } from '../live.svelte';
import { polyfillDom } from '../ui/testing';

vi.mock('../api/incidents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/incidents')>()),
  getIncident: vi.fn(),
}));
vi.mock('../api/channels', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/channels')>()),
  listChannels: vi.fn().mockResolvedValue([]),
}));

const { default: Incident } = await import('./Incident.svelte');
const incidentsApi = await import('../api/incidents');
const { seen } = await import('../alerts/seen.svelte');
const { seenStorageKey } = await import('../alerts/seen');

const incident = {
  id: 'inc-1',
  title: 'resource incident on res_api',
  status: 'open' as const,
  severity: 'critical' as const,
  targetType: 'resource',
  targetId: 'res_api',
  alertCount: 1,
  firingAlertCount: 1,
  openedAt: '2026-11-06T10:00:00Z',
  alerts: [
    {
      id: 'al-1',
      family: 'resource_down',
      severity: 'critical' as const,
      targetType: 'resource',
      targetId: 'res_api',
      status: 'firing' as const,
      startedAt: '2026-11-06T10:00:00Z',
      message: 'api.production is down',
    },
  ],
  deliveries: [],
};

const live = {
  snapshot: { resources: [{ id: 'res_api', name: 'api.production' }] },
} as unknown as LiveStore;

beforeAll(() => {
  polyfillDom();
  window.scrollTo = () => {};
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  seen.clear('inc-1');
});

describe('Incident detail', () => {
  it('shows member alerts under the resource name, not its id', async () => {
    vi.mocked(incidentsApi.getIncident).mockResolvedValue(incident);
    render(Incident, { live, id: 'inc-1', viewer: 'alice' });
    expect(
      await screen.findByRole('heading', {
        name: 'Resource incident on api.production',
      }),
    ).toBeTruthy();
    expect(screen.getAllByText('api.production is down')).not.toHaveLength(0);
  });

  it('marks the incident as seen by the viewer in this browser (UI-21)', async () => {
    vi.mocked(incidentsApi.getIncident).mockResolvedValue(incident);
    render(Incident, { live, id: 'inc-1', viewer: 'alice' });

    await fireEvent.click(
      await screen.findByRole('button', { name: 'Mark as seen' }),
    );

    const region = screen.getByRole('region', { name: 'Seen by' });
    expect(region.textContent).toContain('Seen by alice');
    expect(region.textContent).toContain('this browser only');
    expect(localStorage.getItem(seenStorageKey)).toContain('"by":"alice"');

    await fireEvent.click(screen.getByRole('button', { name: 'Clear' }));
    expect(screen.getByRole('button', { name: 'Mark as seen' })).toBeTruthy();
  });

  it('keeps the incident on screen when a refresh fails', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.mocked(incidentsApi.getIncident)
      .mockResolvedValueOnce(incident)
      .mockRejectedValue(new Error('The incident is unavailable.'));
    render(Incident, { live, id: 'inc-1' });
    await screen.findByRole('heading', {
      name: 'Resource incident on api.production',
    });

    await vi.advanceTimersByTimeAsync(15_000);

    expect(
      await screen.findByText(
        /could not be refreshed: The incident is unavailable/,
      ),
    ).toBeTruthy();
    expect(
      screen.getByRole('heading', {
        name: 'Resource incident on api.production',
      }),
    ).toBeTruthy();
  });

  it('states a failed first load with a retry', async () => {
    vi.mocked(incidentsApi.getIncident).mockRejectedValue(
      new Error('The incident is unavailable.'),
    );
    render(Incident, { live, id: 'inc-1' });
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('This incident could not be loaded');
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy();
  });
});
