import { expect, test, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { mockApp } from './fixtures';

async function scan(page: Page) {
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
    .analyze();
  expect(results.violations).toEqual([]);
}

test('login page has no detectable a11y violations', async ({ page }) => {
  await mockApp(page, { authenticated: false });
  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  await expect(page.getByLabel('Authentication code')).toHaveCount(0);
  await scan(page);
});

test('setup page has no detectable a11y violations', async ({ page }) => {
  await mockApp(page, { authenticated: false, setupAvailable: true });
  await page.goto('/setup');
  await expect(
    page.getByRole('heading', { name: 'Set up Binnacle' }),
  ).toBeVisible();
  await scan(page);
});

test('onboarding page has no detectable a11y violations', async ({ page }) => {
  await mockApp(page, { onboarded: false });
  await page.goto('/onboarding');
  await expect(
    page.getByRole('navigation', { name: 'Primary navigation' }),
  ).toHaveCount(0);
  await expect(
    page.getByRole('heading', { name: 'Before you start' }),
  ).toBeVisible();
  await scan(page);
});

test('overview page has no detectable a11y violations', async ({ page }) => {
  await mockApp(page);
  await page.goto('/overview');
  await expect(page.getByText('api.production')).toBeVisible();
  await scan(page);
});

test('host page has no detectable a11y violations', async ({ page }) => {
  await mockApp(page);
  await page.goto('/host');
  await expect(
    page.getByRole('heading', { name: 'CPU' }).first(),
  ).toBeVisible();
  await scan(page);
  await page.getByRole('tab', { name: /Filesystems/ }).click();
  await scan(page);
  await page.getByRole('tab', { name: 'Processes' }).click();
  await scan(page);
});

test('resources page has no detectable a11y violations', async ({
  page,
}, testInfo) => {
  await mockApp(page);
  await page.goto('/resources');
  await expect(
    page.getByRole(
      testInfo.project.name.includes('mobile') ? 'list' : 'table',
      {
        name: 'Resources',
      },
    ),
  ).toBeVisible();
  await scan(page);
});

test('resource detail has no detectable a11y violations', async ({ page }) => {
  await mockApp(page);
  await page.goto('/resources/res1');
  await expect(
    page.getByRole('heading', { name: 'api.production', level: 1 }),
  ).toBeVisible();
  await scan(page);
  await page.getByRole('tab', { name: /Containers/ }).click();
  await scan(page);
  await page.getByRole('tab', { name: 'Alerts' }).click();
  await scan(page);
});

test('activity page has no detectable a11y violations', async ({ page }) => {
  await mockApp(page);
  await page.goto('/activity');
  await expect(
    page.getByRole('heading', { name: 'Activity', level: 1 }),
  ).toBeVisible();
  await scan(page);
});

test('logs page has no detectable a11y violations', async ({ page }) => {
  await mockApp(page);
  await page.goto('/logs?resource=res1');
  await expect(
    page.getByRole('heading', { name: 'Logs', level: 1 }),
  ).toBeVisible();
  await scan(page);
});

test('alerts tabs have no detectable a11y violations', async ({ page }) => {
  await mockApp(page);
  await page.route('**/api/v1/alert-rules', (route) =>
    route.fulfill({
      json: [
        {
          id: 'builtin-host-cpu-warning',
          family: 'host_cpu_warning',
          name: 'Host CPU warning',
          builtIn: true,
          enabled: true,
          severity: 'warning',
          scopeType: 'global',
          threshold: 90,
          recoveryThreshold: 80,
          triggerSeconds: 300,
          recoverySeconds: 120,
          suppressDuringDeployment: false,
        },
      ],
    }),
  );
  await page.goto('/alerts');
  await expect(
    page.getByRole('heading', { name: 'Alerts', level: 1 }),
  ).toBeVisible();
  await scan(page);
  for (const tab of ['Firing', 'Rules', 'Checks', 'Silences']) {
    await page.getByRole('tab', { name: new RegExp(tab) }).click();
    await scan(page);
  }
});

test('incident detail has no detectable a11y violations', async ({ page }) => {
  await mockApp(page);
  await page.route('**/api/v1/incidents/inc-1', (route) =>
    route.fulfill({
      json: {
        id: 'inc-1',
        status: 'open',
        severity: 'critical',
        title: 'resource incident on res1',
        targetType: 'resource',
        targetId: 'res1',
        alertCount: 1,
        firingAlertCount: 1,
        openedAt: '2026-07-11T12:00:00Z',
        alerts: [
          {
            id: 'alert-1',
            family: 'required_check_failure',
            severity: 'critical',
            status: 'firing',
            message: 'API is unhealthy',
            startedAt: '2026-07-11T12:00:00Z',
            targetType: 'resource',
            targetId: 'res1',
          },
        ],
        deliveries: [],
      },
    }),
  );
  await page.goto('/alerts/incidents/inc-1');
  await expect(page.getByText('API is unhealthy')).toBeVisible();
  await scan(page);
});

test('settings sections have no detectable a11y violations', async ({
  page,
}) => {
  await mockApp(page);
  for (const section of [
    'general',
    'data',
    'access',
    'notifications',
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
  await expect(page.getByRole('button', { name: 'New token' })).toHaveCount(0);
});

test('command palette and dialogs have no detectable a11y violations', async ({
  page,
}) => {
  await mockApp(page);
  await page.goto('/overview');
  await page.keyboard.press('Control+k');
  await expect(
    page.getByRole('dialog', { name: 'Command palette' }),
  ).toBeVisible();
  await scan(page);
  await page.keyboard.press('Escape');
  await page.goto('/alerts?tab=checks');
  await page.getByRole('button', { name: 'New check' }).first().click();
  await expect(
    page.getByRole('dialog', { name: 'New health check' }),
  ).toBeVisible();
  await scan(page);
});
