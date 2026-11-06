import { expect, test, type Page } from '@playwright/test';
import { mockApp } from './fixtures';

async function viewportWidth(page: Page) {
  return page.viewportSize()?.width ?? 0;
}

test('overview renders the headroom row and the grouped resource table', async ({
  page,
}, testInfo) => {
  const mobile = testInfo.project.name.includes('mobile');
  await mockApp(page);
  await page.goto('/headroom');
  await expect(
    page.getByRole('heading', { name: 'Headroom', level: 1 }),
  ).toBeVisible();
  const mark = page.locator('img[src*="binnacle-mark"]:visible').first();
  await expect(mark).toBeVisible();
  expect(
    await mark.evaluate((image) => (image as HTMLImageElement).naturalWidth),
  ).toBeGreaterThan(0);
  // The CPU arc gauge: a meter inside a link to Server named "CPU <value>, <trend>".
  const cpu = page.getByRole('link', { name: /^CPU \d/ });
  await expect(cpu).toBeVisible();
  await expect(
    cpu.getByRole('meter', { name: 'CPU', exact: true }),
  ).toBeVisible();
  const resources = page.getByRole(mobile ? 'list' : 'table', {
    name: 'Resources',
  });
  await expect(resources).toBeVisible();
  await expect(page.getByText('binnacle / production')).toBeVisible();
  const box = await resources.boundingBox();
  expect(box?.width).toBeLessThanOrEqual(await viewportWidth(page));
});

test('login renders the branded sign-in card', async ({ page }) => {
  await mockApp(page, { authenticated: false });
  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  const mark = page.locator('img[src*="binnacle-mark"]:visible').first();
  expect(
    await mark.evaluate((image) => (image as HTMLImageElement).naturalWidth),
  ).toBeGreaterThan(0);
  await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible();
});

test('host renders stat tiles and historical charts', async ({ page }) => {
  await mockApp(page);
  await page.goto('/server');
  await expect(
    page.getByRole('heading', { name: 'Server', level: 1 }),
  ).toBeVisible();
  await expect(
    page.getByRole('meter', { name: 'CPU utilization' }),
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { name: 'Load average' }),
  ).toBeVisible();
  await expect(
    page.getByRole('button', { name: /Inspect CPU/ }),
  ).toBeAttached();
});

test('overview rows link to the resource detail page', async ({ page }) => {
  await mockApp(page);
  await page.goto('/headroom');
  await page.getByRole('link', { name: 'api.production' }).click();
  await expect(page).toHaveURL(/\/resources\/res1$/);
  await expect(
    page.getByRole('heading', { name: 'api.production', level: 1 }),
  ).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open logs' })).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(/\/headroom$/);
});

test('activity and settings render their sections', async ({ page }) => {
  await mockApp(page);
  await page.goto('/changes');
  await expect(
    page.getByRole('heading', { name: 'Changes', level: 1 }),
  ).toBeVisible();
  await page.goto('/settings');
  await expect(
    page.getByRole('heading', { name: 'Collection', exact: true }),
  ).toBeVisible();
  await page.goto('/settings/data');
  await expect(
    page.getByRole('heading', { name: 'Retention', exact: true }),
  ).toBeVisible();
  await page.goto('/settings/access');
  await expect(
    page.getByRole('heading', { name: 'Sessions', exact: true }),
  ).toBeVisible();
});

test('system settings show worker failures and latency', async ({ page }) => {
  await mockApp(page);
  await page.goto('/settings/system');
  await expect(
    page.getByRole('heading', { name: 'Background workers', exact: true }),
  ).toBeVisible();
  const failures = page.getByRole('list', { name: 'Failures since start' });
  await expect(failures.getByText('Docker API calls')).toBeVisible();
  await expect(failures.getByText('2', { exact: true })).toBeVisible();
  const table = page.getByRole('table', { name: /Latency of the latest runs/ });
  const write = table.getByRole('row', { name: /Persistence write/ });
  await expect(write.getByRole('cell', { name: '9.8 ms' })).toBeVisible();
  await expect(write.getByRole('cell', { name: '41.5 ms' })).toBeVisible();
  const vacuum = table.getByRole('row', { name: /Incremental vacuum/ });
  await expect(vacuum.getByRole('cell', { name: '—' }).first()).toBeVisible();
  const scrollWidth = await page.evaluate(
    () => document.documentElement.scrollWidth,
  );
  expect(scrollWidth).toBeLessThanOrEqual(await viewportWidth(page));
});

test('first-time users receive the dark theme', async ({ page }) => {
  await mockApp(page);
  await page.goto('/headroom');
  expect(
    await page.evaluate(() => document.documentElement.dataset.theme),
  ).toBe('dark');
});

test('mobile layout keeps content inside the viewport', async ({ page }) => {
  await mockApp(page);
  await page.goto('/headroom');
  const width = await viewportWidth(page);
  const scrollWidth = await page.evaluate(
    () => document.documentElement.scrollWidth,
  );
  expect(scrollWidth).toBeLessThanOrEqual(width);
  const heading = page.getByRole('heading', { name: 'Headroom', level: 1 });
  const box = await heading.boundingBox();
  expect(box?.width).toBeLessThanOrEqual(width);
});
