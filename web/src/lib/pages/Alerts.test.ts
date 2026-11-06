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
import { cleanup } from '@testing-library/svelte';
import { render, screen, waitFor, within } from '@testing-library/svelte';
import type { LiveStore } from '../live.svelte';
import { router } from '../router.svelte';
import { polyfillDom } from '../ui/testing';

vi.mock('../api/alerts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/alerts')>()),
  listRules: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/checks', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/checks')>()),
  listChecks: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/incidents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/incidents')>()),
  listIncidents: vi.fn().mockResolvedValue([]),
  listAlerts: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/silences', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/silences')>()),
  listSilences: vi.fn().mockResolvedValue([]),
}));

const { default: Alerts } = await import('./Alerts.svelte');
const incidentsApi = await import('../api/incidents');
const silencesApi = await import('../api/silences');
const checksApi = await import('../api/checks');

const openIncident = {
  id: 'inc-1',
  title: 'resource incident on res_api',
  status: 'open' as const,
  severity: 'critical' as const,
  targetType: 'resource',
  targetId: 'res_api',
  alertCount: 2,
  firingAlertCount: 1,
  openedAt: '2026-11-06T10:00:00Z',
};

const live = {
  snapshot: { resources: [{ id: 'res_api', name: 'api.production' }] },
} as unknown as LiveStore;

function visit(path: string) {
  history.replaceState({}, '', path);
  router.sync();
}

beforeEach(() => {
  vi.mocked(incidentsApi.listIncidents).mockResolvedValue([]);
  vi.mocked(silencesApi.listSilences).mockResolvedValue([]);
  vi.mocked(checksApi.listChecks).mockResolvedValue([]);
});

beforeAll(() => {
  polyfillDom();
  window.scrollTo = () => {};
});

afterEach(() => cleanup());

describe('Incidents page', () => {
  it('keeps the other tabs when one request fails (UI-9)', async () => {
    vi.mocked(incidentsApi.listIncidents).mockImplementation(async (query) =>
      query?.status === 'open' ? [openIncident] : [],
    );
    vi.mocked(silencesApi.listSilences).mockRejectedValue(
      new Error('Silences are unavailable.'),
    );
    visit('/incidents');
    render(Alerts, { live });

    // The open incident still shows, named, not emptied by the failure.
    expect(
      await screen.findAllByText('Resource incident on api.production'),
    ).not.toHaveLength(0);

    router.navigate('/incidents?tab=silences');
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('Silences could not be loaded');
    expect(alert.textContent).toContain('Silences are unavailable.');
    expect(screen.queryByText('No active silences')).toBeNull();
  });

  it('says a failed incident list failed instead of showing an all-clear', async () => {
    vi.mocked(incidentsApi.listIncidents).mockRejectedValue(
      new Error('Incidents are unavailable.'),
    );
    visit('/incidents');
    render(Alerts, { live });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('Open incidents could not be loaded');
    expect(screen.queryByText('No open incidents')).toBeNull();
  });

  it('shows check status read-only with a link to Settings › Alerting', async () => {
    visit('/incidents?tab=checks');
    render(Alerts, { live });
    const panel = await waitFor(() => {
      const found = document.getElementById('incidents-panel-checks');
      if (!found || found.hidden) throw new Error('checks panel not shown');
      return found;
    });
    await within(panel).findByText('No health checks yet');
    const manage = within(panel).getByRole('link', { name: 'Manage checks' });
    expect(manage.getAttribute('href')).toBe('/settings/alerting');
    expect(
      within(panel).queryByRole('button', { name: /New check/ }),
    ).toBeNull();
    expect(screen.queryByRole('tab', { name: /Rules/ })).toBeNull();
  });

  it('opens the silence dialog when the palette links here while it is open (UI-20)', async () => {
    visit('/incidents');
    render(Alerts, { live: { snapshot: null } as unknown as LiveStore });
    expect(document.querySelector('dialog[open]')).toBeNull();

    router.navigate('/incidents?tab=silences&new=1');

    await waitFor(() =>
      expect(document.querySelector('dialog[open]')).not.toBeNull(),
    );
    expect(router.param('new')).toBe('');
    expect(router.param('tab')).toBe('silences');
  });
});
