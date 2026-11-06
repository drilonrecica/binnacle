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
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/svelte';
import type { LiveStore } from '../live.svelte';
import type { SettingsSnapshot } from '../api/settings';
import { leaveGuard } from '../leave-guard.svelte';
import { router } from '../router.svelte';
import { polyfillDom } from '../ui/testing';

const setting = (value: string) => ({
  value,
  source: 'Default',
  applyMode: 'live' as const,
});

function settings(): SettingsSnapshot {
  return {
    revision: 3,
    values: {
      'retention.preset': setting('balanced'),
      'retention.raw': setting('48h0m0s'),
      'retention.one_minute': setting('720h0m0s'),
      'retention.fifteen_minute': setting('8760h0m0s'),
      'retention.one_hour': setting('0s'),
      'database.target_budget_bytes': setting(String(1024 ** 3)),
    },
    features: { advancedAuth: false, portability: false },
  };
}

vi.mock('../api/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/settings')>()),
  loadSettings: vi.fn(),
  saveSettings: vi.fn(),
}));
vi.mock('../api/access', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/access')>()),
  monitorHealth: vi.fn().mockResolvedValue({ at: '', metrics: [] }),
  previewDeletion: vi.fn(),
}));
vi.mock('../api/alerts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/alerts')>()),
  listRules: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/checks', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/checks')>()),
  listChecks: vi.fn().mockResolvedValue([]),
}));

const { default: Settings } = await import('./Settings.svelte');
const settingsApi = await import('../api/settings');
const accessApi = await import('../api/access');
const alertsApi = await import('../api/alerts');
const checksApi = await import('../api/checks');

const live = { snapshot: { resources: [] } } as unknown as LiveStore;

function visit(path: string) {
  history.replaceState({}, '', path);
  router.sync();
}

function show(section: string) {
  visit(`/settings/${section}`);
  return render(Settings, { live, section, onsignedout: () => {} });
}

/** The card whose title is `title`. */
function card(title: string): HTMLElement {
  const heading = screen.getByText(title, { selector: 'h2, h3' });
  const root = heading.closest('section, article, .card');
  if (!(root instanceof HTMLElement)) throw new Error(`No card ${title}`);
  return root;
}

beforeAll(() => {
  polyfillDom();
  window.scrollTo = () => {};
});

beforeEach(() => {
  vi.mocked(settingsApi.loadSettings).mockResolvedValue(settings());
  vi.mocked(settingsApi.saveSettings).mockImplementation(
    async (_revision, changes) => {
      const next = settings();
      next.revision = 4;
      for (const [key, value] of Object.entries(changes))
        next.values[key] = setting(value);
      return next;
    },
  );
  vi.mocked(accessApi.previewDeletion).mockResolvedValue({
    token: 't',
    confirmation: 'DELETE HISTORY BEFORE 2026-11-05',
    totalRows: 12_345,
    expiresAt: '2026-11-06T12:00:00Z',
  });
});

afterEach(() => {
  cleanup();
  leaveGuard.settle();
  vi.clearAllMocks();
});

async function editBudget() {
  const input = await screen.findByLabelText('Database budget');
  await fireEvent.input(input, { target: { value: '2' } });
}

describe('Settings › Data (UI-8)', () => {
  it('keeps one form per card: editing the budget leaves retention clean', async () => {
    show('data');
    await editBudget();
    expect(within(card('Storage budget')).getByText('Unsaved changes'));
    expect(within(card('Retention')).queryByText('Unsaved changes')).toBeNull();
    // Saving the budget sends only its own key.
    await fireEvent.click(
      within(card('Storage budget')).getByRole('button', {
        name: 'Save changes',
      }),
    );
    await waitFor(() =>
      expect(settingsApi.saveSettings).toHaveBeenCalledWith(3, {
        'database.target_budget_bytes': String(2 * 1024 ** 3),
      }),
    );
  });

  it('confirms a shortening with the deletion preview before saving', async () => {
    show('data');
    await fireEvent.click(
      await screen.findByRole('radio', { name: /Minimal/ }),
    );
    await fireEvent.click(
      within(card('Retention')).getByRole('button', { name: 'Save changes' }),
    );
    const dialog = await screen.findByRole('dialog', {
      name: 'Shorten retention?',
    });
    expect(dialog.textContent).toContain('12,345');
    expect(accessApi.previewDeletion).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'before' }),
    );
    expect(settingsApi.saveSettings).not.toHaveBeenCalled();
    await fireEvent.click(
      within(dialog).getByRole('button', { name: 'Shorten and delete' }),
    );
    await waitFor(() =>
      expect(settingsApi.saveSettings).toHaveBeenCalledWith(3, {
        'retention.preset': 'minimal',
      }),
    );
  });

  it('saves a longer retention without asking', async () => {
    show('data');
    await fireEvent.click(
      await screen.findByRole('radio', { name: /Long-term/ }),
    );
    await fireEvent.click(
      within(card('Retention')).getByRole('button', { name: 'Save changes' }),
    );
    await waitFor(() => expect(settingsApi.saveSettings).toHaveBeenCalled());
    expect(accessApi.previewDeletion).not.toHaveBeenCalled();
  });

  it('never saves a shortening when the preview fails', async () => {
    vi.mocked(accessApi.previewDeletion).mockRejectedValue(
      new Error('Storage is busy.'),
    );
    show('data');
    await fireEvent.click(
      await screen.findByRole('radio', { name: /Minimal/ }),
    );
    await fireEvent.click(
      within(card('Retention')).getByRole('button', { name: 'Save changes' }),
    );
    expect(
      (await within(card('Retention')).findByRole('alert')).textContent,
    ).toContain('Nothing was saved');
    expect(settingsApi.saveSettings).not.toHaveBeenCalled();
  });
});

describe('Settings unsaved-changes guard (UI-21)', () => {
  it('holds navigation while a card is dirty and leaves only on confirm', async () => {
    show('data');
    await editBudget();
    router.navigate('/headroom');
    expect(router.path).toBe('/settings/data');
    const dialog = await screen.findByRole('dialog', {
      name: 'Leave without saving?',
    });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Stay' }));
    expect(router.path).toBe('/settings/data');
    expect(leaveGuard.pending).toBeNull();

    router.navigate('/headroom');
    await fireEvent.click(
      within(
        await screen.findByRole('dialog', { name: 'Leave without saving?' }),
      ).getByRole('button', { name: 'Discard and leave' }),
    );
    await waitFor(() => expect(router.path).toBe('/headroom'));
  });

  it('lets query-only changes and clean pages through', async () => {
    show('data');
    await screen.findByLabelText('Database budget');
    router.navigate('/headroom');
    expect(router.path).toBe('/headroom');
  });
});

describe('Settings › Alerting', () => {
  it('holds the rules and checks moved from Incidents', async () => {
    vi.mocked(alertsApi.listRules).mockResolvedValue([
      {
        id: 'rule-cpu',
        family: 'host',
        name: 'host_cpu_high',
        builtIn: true,
        enabled: true,
        severity: 'warning',
        scopeType: 'host',
        threshold: 80,
        triggerSeconds: 300,
        recoverySeconds: 300,
        suppressDuringDeployment: false,
      },
    ]);
    show('alerting');
    expect(
      await screen.findByRole('link', { name: /Alerting/, current: 'page' }),
    ).toBeTruthy();
    expect(card('Alert rules')).toBeTruthy();
    expect(await screen.findByRole('switch', { name: /enabled/ })).toBeTruthy();
    expect(card('Health checks')).toBeTruthy();
    expect(await screen.findByText('No health checks yet')).toBeTruthy();
    expect(checksApi.listChecks).toHaveBeenCalled();
  });

  it('says when rules fail to load instead of showing none', async () => {
    vi.mocked(alertsApi.listRules).mockRejectedValue(
      new Error('Alert rules are unavailable.'),
    );
    show('alerting');
    expect(
      await screen.findByText('Alert rules could not be loaded'),
    ).toBeTruthy();
  });
});
