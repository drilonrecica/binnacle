import { expect, test, type Locator, type Page } from '@playwright/test';
import {
  dropLive,
  fixtureNow,
  mockApp,
  mockPopulated,
  scan,
  themes,
  type MockOptions,
} from './fixtures';

// axe on every page, tab and dialog, in both themes (DESIGN, "Accessibility
// rules carried forward"). Pages are scanned with populated lists so every
// row, menu and dialog exists; empty states get their own scan.

async function open(
  page: Page,
  path: string,
  options: MockOptions & { clock?: 'fixed' | 'installed' } = {},
) {
  const { clock = 'fixed', ...mock } = options;
  if (clock === 'fixed') await page.clock.setFixedTime(fixtureNow);
  else await page.clock.install({ time: fixtureNow });
  await mockApp(page, mock);
  await mockPopulated(page);
  await page.goto(path);
}

/** Opens a dialog, scans it, and closes it with Escape. */
async function scanDialog(page: Page, trigger: Locator, name: string) {
  await trigger.click();
  const dialog = page.getByRole('dialog', { name });
  await expect(dialog).toBeVisible();
  await scan(page);
  await page.keyboard.press('Escape');
  await expect(dialog).toBeHidden();
}

/** Selects each tab in turn and scans its panel. */
async function scanTabs(page: Page, names: RegExp[]) {
  for (const name of names) {
    const tab = page.getByRole('tab', { name });
    await tab.click();
    await expect(tab).toHaveAttribute('aria-selected', 'true');
    await expect(page.getByRole('tabpanel')).toBeVisible();
    await scan(page);
  }
}

function mobile(testInfo: { project: { name: string } }) {
  return testInfo.project.name.includes('mobile');
}

for (const theme of themes) {
  test.describe(`${theme} theme`, () => {
    test('sign-in, setup and onboarding', async ({ page }) => {
      await mockApp(page, { theme, authenticated: false });
      await page.route('**/api/v1/auth/login', (route) =>
        route.fulfill({
          status: 401,
          json: { error: { message: 'The username or password is wrong.' } },
        }),
      );
      await page.goto('/login');
      await expect(
        page.getByRole('heading', { name: 'Sign in' }),
      ).toBeVisible();
      await scan(page);
      // The error state too.
      await page.getByLabel('Username').fill('admin');
      await page.getByLabel('Password').fill('wrong');
      await page.getByRole('button', { name: 'Sign in' }).click();
      await expect(page.getByRole('alert')).toBeVisible();
      await scan(page);

      await page.unrouteAll();
      await mockApp(page, {
        theme,
        authenticated: false,
        setupAvailable: true,
      });
      await page.route('**/api/v1/setup/verify', (route) =>
        route.fulfill({ status: 204 }),
      );
      await page.goto('/setup');
      await expect(
        page.getByRole('heading', { name: 'Set up Binnacle' }),
      ).toBeVisible();
      await scan(page);
      await page.getByLabel('Setup token').fill('token');
      await page.getByRole('button', { name: 'Verify token' }).click();
      await expect(
        page.getByRole('heading', { name: 'Create the administrator' }),
      ).toBeVisible();
      await scan(page);

      await page.unrouteAll();
      await mockApp(page, { theme, onboarded: false });
      await page.goto('/onboarding');
      await expect(
        page.getByRole('heading', { name: 'Before you start' }),
      ).toBeVisible();
      await scan(page);
    });

    test('headroom, live and stale', async ({ page }) => {
      await open(page, '/headroom', { theme, clock: 'installed' });
      await expect(page.getByText('api.production').first()).toBeVisible();
      await expect(
        page.getByRole('region', { name: 'Needs attention' }),
      ).toContainText('worker.production');
      await scan(page);
      // Stale instruments keep readable contrast (DSC-19).
      await page.clock.fastForward(30_000);
      await expect(page.getByTestId('instruments')).toHaveClass(/stale/);
      await scan(page);
    });

    test('headroom and server while disconnected', async ({ page }) => {
      await open(page, '/headroom', { theme });
      await expect(page.getByText('api.production').first()).toBeVisible();
      await page.route('**/api/v1/live*', (route) =>
        route.fulfill({ status: 503, body: '' }),
      );
      await dropLive(page);
      await expect(page.getByText('Live updates disconnected')).toBeVisible();
      await scan(page);
      await page
        .getByRole('navigation', { name: 'Primary navigation' })
        .getByRole('link', { name: 'Server' })
        .click();
      await expect(
        page.getByRole('heading', { name: 'Server', level: 1 }),
      ).toBeVisible();
      await scan(page);
    });

    test('resources: list, cards, archived and the logs prompt', async ({
      page,
    }, testInfo) => {
      await open(page, '/resources', { theme });
      await expect(
        page.getByRole(mobile(testInfo) ? 'list' : 'table', {
          name: 'Resources',
        }),
      ).toBeVisible();
      await scan(page);
      if (!mobile(testInfo)) {
        await page.getByRole('button', { name: 'Cards' }).click();
        await expect(
          page.getByRole('list', { name: 'Resources' }),
        ).toBeVisible();
        await scan(page);
      }
      await scanTabs(page, [/Archived/]);
      await page.goto('/logs');
      await expect(
        page.getByText(/Logs now live on each resource/),
      ).toBeVisible();
      await scan(page);
    });

    test('resource detail: every tab and the silence dialog', async ({
      page,
    }) => {
      await open(page, '/resources/res1', { theme });
      await expect(
        page.getByRole('heading', { name: 'api.production', level: 1 }),
      ).toBeVisible();
      await scan(page);
      await scanTabs(page, [
        /Containers/,
        /Incidents/,
        /Changes/,
        /^Logs/,
        /Metrics/,
      ]);
      await page.getByRole('tab', { name: /Incidents/ }).click();
      await scanDialog(
        page,
        page.getByRole('button', { name: /^Silence/ }).first(),
        'Silence notifications',
      );
    });

    test('server: every tab and the custom range popover', async ({ page }) => {
      await open(page, '/server', { theme });
      await expect(
        page.getByRole('meter', { name: 'CPU', exact: true }),
      ).toBeVisible();
      await scan(page);
      await page.getByRole('button', { name: 'Custom' }).click();
      await expect(
        page.getByRole('dialog', { name: 'Custom time range' }),
      ).toBeVisible();
      await scan(page);
      await page.keyboard.press('Escape');
      await scanTabs(page, [/Filesystems/, /Processes/, /Collectors/]);
    });

    test('incidents: every tab, row menus and dialogs', async ({ page }) => {
      await open(page, '/incidents', { theme });
      await expect(
        page.getByRole('heading', { name: 'Incidents', level: 1 }),
      ).toBeVisible();
      await scan(page);
      await scanTabs(page, [/Resolved/, /Firing/, /Checks/, /Silences/]);

      await page.getByRole('tab', { name: /Firing/ }).click();
      await page
        .getByRole('button', { name: /Alert actions|Actions/ })
        .first()
        .click();
      await expect(page.getByRole('menu')).toBeVisible();
      await scan(page);
      await page.keyboard.press('Escape');

      await page.getByRole('tab', { name: /Silences/ }).click();
      await scanDialog(
        page,
        page.getByRole('button', { name: /^End/ }).first(),
        'End this silence now?',
      );
      await scanDialog(
        page,
        page.getByRole('button', { name: 'Silence…' }),
        'Silence notifications',
      );
    });

    test('incident detail, before and after marking it seen', async ({
      page,
    }) => {
      await open(page, '/incidents/inc-1', { theme });
      await expect(page.getByText(/restarted 4 times/).first()).toBeVisible();
      await scan(page);
      await page.getByRole('button', { name: 'Mark as seen' }).click();
      await expect(page.getByText(/this browser only/)).toBeVisible();
      await scan(page);
    });

    test('changes with events and filters', async ({ page }) => {
      await open(page, '/changes', { theme });
      await expect(
        page.getByText('worker.production was OOM killed'),
      ).toBeVisible();
      await scan(page);
    });

    test('empty and failed states', async ({ page }) => {
      await mockApp(page, { theme });
      await page.route('**/api/v1/alerts?*', (route) =>
        route.fulfill({ status: 500, json: { error: { code: 'internal' } } }),
      );
      await page.goto('/incidents');
      await expect(
        page.getByRole('heading', { name: 'Incidents', level: 1 }),
      ).toBeVisible();
      await scan(page);
      await page.getByRole('tab', { name: /Firing/ }).click();
      await expect(page.getByRole('tabpanel')).toContainText(
        /could not|failed/i,
      );
      await scan(page);
      await page.goto('/changes');
      await expect(
        page.getByRole('heading', { name: 'Changes', level: 1 }),
      ).toBeVisible();
      await scan(page);
      await page.goto('/resources/missing');
      await scan(page);
    });

    test('settings: every section', async ({ page }) => {
      await open(page, '/settings/general', { theme });
      for (const section of [
        'general',
        'data',
        'access',
        'notifications',
        'alerting',
        'appearance',
        'integrations',
        'system',
      ]) {
        await page.goto(`/settings/${section}`);
        await expect(
          page.getByRole('heading', { name: 'Settings', level: 1 }),
        ).toBeVisible();
        await scan(page);
      }
    });

    test('settings: alerting dialogs', async ({ page }) => {
      await open(page, '/settings/alerting', { theme });
      await scanDialog(
        page,
        page.getByRole('button', { name: 'New check' }).first(),
        'New health check',
      );
      await scanDialog(
        page,
        page.getByRole('button', { name: 'Edit Host CPU' }),
        'Edit Host CPU',
      );
      await page.getByRole('button', { name: 'Check actions' }).click();
      await expect(page.getByRole('menu')).toBeVisible();
      await scan(page);
      await page.getByRole('menuitem', { name: 'Edit…' }).click();
      const edit = page.getByRole('dialog', { name: 'Edit health check' });
      await expect(edit).toBeVisible();
      // Focus lands on the Resource combobox, which opens its listbox.
      await expect(page.getByRole('listbox')).toBeVisible();
      await scan(page);
      // Escape closes the listbox first, then the dialog.
      await page.keyboard.press('Escape');
      await expect(page.getByRole('listbox')).toBeHidden();
      await expect(edit).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(edit).toBeHidden();
      await page.getByRole('button', { name: 'Check actions' }).click();
      await page.getByRole('menuitem', { name: /Delete/ }).click();
      await expect(
        page.getByRole('dialog', { name: 'Delete this check?' }),
      ).toBeVisible();
      await scan(page);
    });

    test('settings: notifications dialogs', async ({ page }) => {
      await open(page, '/settings/notifications', { theme });
      await scanDialog(
        page,
        page.getByRole('button', { name: 'New channel' }),
        'New notification channel',
      );
      await page.getByRole('button', { name: 'Channel actions' }).click();
      await page.getByRole('menuitem', { name: /Edit/ }).click();
      await expect(
        page.getByRole('dialog', { name: 'Edit channel' }),
      ).toBeVisible();
      await scan(page);
      await page.keyboard.press('Escape');
      await page.getByRole('button', { name: 'Channel actions' }).click();
      await page.getByRole('menuitem', { name: /Delete/ }).click();
      await expect(
        page.getByRole('dialog', { name: 'Delete this channel?' }),
      ).toBeVisible();
      await scan(page);
    });

    test('settings: access dialogs', async ({ page }) => {
      await open(page, '/settings/access', { theme, advancedAuth: true });
      await page.route('**/api/v1/auth/mfa', (route) =>
        route.fulfill({ json: { enabled: false } }),
      );
      await page.reload();
      await scanDialog(
        page,
        page.getByRole('button', { name: 'Change password…' }),
        'Change password',
      );
      await scanDialog(
        page,
        page.getByRole('button', { name: 'Set up two-factor' }),
        'Set up two-factor authentication',
      );
    });

    test('settings: data dialogs and the unsaved-changes guard', async ({
      page,
    }) => {
      await open(page, '/settings/data', { theme });
      await page.route('**/api/v1/history/deletion-previews', (route) =>
        route.fulfill({
          json: {
            token: 'preview-token',
            confirmation: 'DELETE ALL HISTORY',
            totalRows: 9000,
            expiresAt: '2026-07-11T12:10:00Z',
          },
        }),
      );
      await page.getByRole('radio', { name: /Everything/ }).check();
      await page.getByRole('button', { name: 'Preview deletion' }).click();
      const deletion = page.getByRole('dialog', {
        name: 'Delete history permanently?',
      });
      await expect(deletion).toBeVisible();
      await scan(page);
      await page.keyboard.press('Escape');
      await expect(deletion).toBeHidden();

      await page.getByRole('radio', { name: /Minimal/ }).check();
      await page.getByRole('button', { name: 'Save changes' }).first().click();
      await expect(
        page.getByRole('dialog', { name: 'Shorten retention?' }),
      ).toBeVisible();
      await scan(page);
      await page.keyboard.press('Escape');

      await page.getByRole('link', { name: /^General/ }).click();
      await expect(
        page.getByRole('dialog', { name: 'Leave without saving?' }),
      ).toBeVisible();
      await scan(page);
    });

    test('settings: integrations and appearance dialogs', async ({ page }) => {
      await open(page, '/settings/integrations', { theme, portability: true });
      await scanDialog(
        page,
        page.getByRole('button', { name: 'New token' }),
        'New API token',
      );
      await scanDialog(
        page,
        page.getByRole('button', { name: /Revoke/ }).first(),
        'Revoke this token?',
      );
      await page.goto('/settings/appearance');
      const combobox = page.getByRole('combobox', { name: 'Resource to pin' });
      await combobox.click();
      await expect(page.getByRole('listbox')).toBeVisible();
      await scan(page);
    });

    test('shell: palette, account menu and sign-out confirmation', async ({
      page,
    }, testInfo) => {
      await open(page, '/headroom', { theme });
      await expect(page.getByRole('button', { name: 'Search' })).toBeVisible();
      await page.keyboard.press('Control+k');
      const palette = page.getByRole('dialog', { name: 'Command palette' });
      await expect(palette).toBeVisible();
      await scan(page);
      await palette.getByRole('combobox').fill('api');
      await expect(palette.getByRole('option').first()).toBeVisible();
      await scan(page);
      await page.keyboard.press('Escape');

      // Phones keep account actions in the "More" sheet; desktop has a menu.
      if (mobile(testInfo)) {
        await scanDialog(
          page,
          page
            .getByRole('navigation', { name: 'Primary navigation' })
            .getByRole('button', { name: 'More' }),
          'More',
        );
        return;
      }
      await page
        .getByRole('button', { name: /^admin/ })
        .first()
        .click();
      await expect(page.getByRole('menu')).toBeVisible();
      await scan(page);
      await page
        .getByRole('menuitem', { name: 'Sign out everywhere…' })
        .click();
      await expect(
        page.getByRole('dialog', { name: 'Sign out everywhere?' }),
      ).toBeVisible();
      await scan(page);
    });
  });
}
