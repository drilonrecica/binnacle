import { expect, test } from '@playwright/test';
import { mockApp, snapshot } from './fixtures';

// The shell around every page: navigation (§IA), legacy redirects, the
// command palette, the phone tab bar and the theme.

test('renders the application shell with the primary navigation', async ({
  page,
}, testInfo) => {
  await mockApp(page);
  await page.goto('/headroom');

  await expect(page).toHaveTitle('Binnacle — Headroom');
  await expect(
    page.getByRole('heading', { name: 'Headroom', level: 1 }),
  ).toBeVisible();
  const nav = page.getByRole('navigation', { name: 'Primary navigation' });
  // Phones show five tabs and keep Settings under "More".
  const mobile = testInfo.project.name.includes('mobile');
  for (const label of [
    'Headroom',
    'Resources',
    'Server',
    'Incidents',
    'Changes',
    ...(mobile ? [] : ['Settings']),
  ]) {
    await expect(nav.getByRole('link', { name: label })).toBeVisible();
  }
  if (mobile)
    await expect(nav.getByRole('button', { name: 'More' })).toBeVisible();
  await expect(page.getByText('api.production')).toBeVisible();
});

test('every navigation entry opens its page and moves focus into it', async ({
  page,
}, testInfo) => {
  test.skip(
    testInfo.project.name.includes('mobile'),
    'The phone tab bar is covered by the "More" test below.',
  );
  await mockApp(page);
  await page.goto('/headroom');
  const nav = page.getByRole('navigation', { name: 'Primary navigation' });
  for (const [label, path, heading] of [
    ['Resources', '/resources', 'Resources'],
    ['Server', '/server', 'Server'],
    ['Incidents', '/incidents', 'Incidents'],
    ['Changes', '/changes', 'Changes'],
    ['Settings', '/settings', 'Settings'],
    ['Headroom', '/headroom', 'Headroom'],
  ] as const) {
    await nav.getByRole('link', { name: label }).click();
    await expect(page).toHaveURL(new RegExp(`${path}(/general)?$`));
    await expect(
      page.getByRole('heading', { name: heading, level: 1 }),
    ).toBeVisible();
    await expect(nav.getByRole('link', { name: label })).toHaveAttribute(
      'aria-current',
      'page',
    );
    // Focus follows the route change into the page (UI-10).
    await expect
      .poll(() =>
        page.evaluate(() => !!document.activeElement?.closest('main#content')),
      )
      .toBe(true);
  }
});

test('legacy and unknown routes redirect to their replacements', async ({
  page,
}) => {
  await mockApp(page);
  await page.goto('/watch');
  await expect(page).toHaveURL(/\/headroom$/);
  await page.goto('/overview');
  await expect(page).toHaveURL(/\/headroom$/);
  await page.goto('/host?tab=collectors');
  await expect(page).toHaveURL(/\/server\?tab=collectors$/);
  await page.goto('/alerts?tab=checks');
  await expect(page).toHaveURL(/\/incidents\?tab=checks$/);
  await page.goto('/alerts/incidents/inc-1');
  await expect(page).toHaveURL(/\/incidents\/inc-1$/);
  await page.goto('/activity');
  await expect(page).toHaveURL(/\/changes$/);
  await page.goto('/events');
  await expect(page).toHaveURL(/\/changes$/);
  await page.goto('/watch?inspect=res1');
  await expect(page).toHaveURL(/\/resources\/res1$/);
  await page.goto('/logs?resource=res1');
  await expect(page).toHaveURL(/\/resources\/res1\?tab=logs$/);
  await page.goto('/logs');
  await expect(page).toHaveURL(/\/resources\?prompt=logs$/);
  await expect(page.getByText(/Logs now live on each resource/)).toBeVisible();
  await page.goto('/not-a-real-view');
  await expect(page).toHaveURL(/\/headroom$/);
});

test('the command palette jumps to a resource', async ({ page }) => {
  const postgres = snapshot.resources[2]!;
  await mockApp(page);
  await page.goto('/headroom');
  // The shortcut listener mounts with the shell; wait for it before pressing.
  await expect(page.getByRole('button', { name: 'Search' })).toBeVisible();
  await page.keyboard.press('Control+k');
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  await expect(palette).toBeVisible();
  await palette.getByRole('combobox').fill('postgres');
  // Results come from the live snapshot; wait for the match before choosing.
  await expect(
    palette.getByRole('option', {
      name: new RegExp(`^${postgres.name}`),
    }),
  ).toBeVisible();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/\/resources\/infra1$/);
  await expect(
    page.getByRole('heading', { name: postgres.name, level: 1 }),
  ).toBeVisible();
});

test('the command palette opens logs for a resource', async ({ page }) => {
  await mockApp(page);
  await page.goto('/headroom');
  await expect(page.getByRole('button', { name: 'Search' })).toBeVisible();
  await page.keyboard.press('Control+k');
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  await palette.getByRole('combobox').fill('logs for api');
  await expect(
    palette.getByRole('option', { name: /Logs for api\.production/ }),
  ).toBeVisible();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/\/resources\/res1\?tab=logs$/);
  await expect(page.getByRole('tab', { name: 'Logs' })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await expect
    .poll(() =>
      page.evaluate(() => !!document.activeElement?.closest('main#content')),
    )
    .toBe(true);
});

test('the command palette closes on Escape and returns focus', async ({
  page,
}) => {
  await mockApp(page);
  await page.goto('/headroom');
  const search = page.getByRole('button', { name: 'Search' });
  await search.click();
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  await expect(palette).toBeVisible();
  await expect(palette.getByRole('combobox')).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(palette).toBeHidden();
  await expect(search).toBeFocused();
});

test('the phone shell uses a tab bar with Settings under More', async ({
  page,
}, testInfo) => {
  if (!testInfo.project.name.includes('mobile')) {
    // Touch-device projects already run at phone size; resizing an emulated
    // device after load makes Chromium's hit-testing disagree with layout.
    await page.setViewportSize({ width: 390, height: 844 });
  }
  await mockApp(page);
  await page.goto('/headroom');
  const nav = page.getByRole('navigation', { name: 'Primary navigation' });
  await nav.getByRole('button', { name: 'More' }).click();
  const more = page.getByRole('dialog', { name: 'More' });
  await expect(more).toBeVisible();
  await more.getByRole('link', { name: 'Settings' }).click();
  await expect(page).toHaveURL(/\/settings(\/general)?$/);
  await expect(more).toBeHidden();
  const scrollWidth = await page.evaluate(
    () => document.documentElement.scrollWidth,
  );
  expect(scrollWidth).toBeLessThanOrEqual(page.viewportSize()?.width ?? 0);
});

test('first-time users receive the dark theme', async ({ page }) => {
  await mockApp(page);
  await page.goto('/headroom');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
});

test('a saved light theme applies before first paint and survives reload', async ({
  page,
}) => {
  await mockApp(page, { theme: 'light' });
  // Record the theme the document had before any app code ran (UI-20).
  await page.addInitScript(() => {
    document.addEventListener(
      'readystatechange',
      () => {
        (window as unknown as { firstTheme: string }).firstTheme ??=
          document.documentElement.dataset.theme ?? '';
      },
      { once: true },
    );
  });
  await page.goto('/headroom');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  expect(
    await page.evaluate(
      () => (window as unknown as { firstTheme: string }).firstTheme,
    ),
  ).toBe('light');
});

test('reduced motion stops the live indicator from pulsing', async ({
  browser,
}) => {
  for (const reducedMotion of ['reduce', 'no-preference'] as const) {
    const context = await browser.newContext({ reducedMotion });
    const page = await context.newPage();
    await mockApp(page);
    await page.goto('/headroom');
    const status = page.getByTestId('connection-status').first();
    await expect(status).toHaveAttribute('data-tone', 'ok');
    const animation = await status
      .locator('.dot')
      .evaluate((dot) => getComputedStyle(dot, '::after').animationName);
    if (reducedMotion === 'reduce') expect(animation).toBe('none');
    else expect(animation).not.toBe('none');
    await context.close();
  }
});
