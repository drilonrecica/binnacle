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
  await page.goto('/overview');
  await expect(
    page.getByRole('heading', { name: 'Overview', level: 1 }),
  ).toBeVisible();
  const mark = page.locator('img[src*="binnacle-mark"]:visible').first();
  await expect(mark).toBeVisible();
  expect(
    await mark.evaluate((image) => (image as HTMLImageElement).naturalWidth),
  ).toBeGreaterThan(0);
  await expect(
    page.getByRole('progressbar', { name: 'CPU utilization' }),
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
  await page.goto('/host');
  await expect(
    page.getByRole('heading', { name: 'Host', level: 1 }),
  ).toBeVisible();
  await expect(
    page.getByRole('progressbar', { name: 'CPU utilization' }),
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
  await page.goto('/overview');
  await page.getByRole('link', { name: 'api.production' }).click();
  await expect(page).toHaveURL(/\/resources\/res1$/);
  await expect(
    page.getByRole('heading', { name: 'api.production', level: 1 }),
  ).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open logs' })).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(/\/overview$/);
});

test('activity and settings render their sections', async ({ page }) => {
  await mockApp(page);
  await page.goto('/activity');
  await expect(
    page.getByRole('heading', { name: 'Activity', level: 1 }),
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

test('first-time users receive the dark theme', async ({ page }) => {
  await mockApp(page);
  await page.goto('/overview');
  expect(
    await page.evaluate(() => document.documentElement.dataset.theme),
  ).toBe('dark');
});

test('mobile layout keeps content inside the viewport', async ({ page }) => {
  await mockApp(page);
  await page.goto('/overview');
  const width = await viewportWidth(page);
  const scrollWidth = await page.evaluate(
    () => document.documentElement.scrollWidth,
  );
  expect(scrollWidth).toBeLessThanOrEqual(width);
  const heading = page.getByRole('heading', { name: 'Overview', level: 1 });
  const box = await heading.boundingBox();
  expect(box?.width).toBeLessThanOrEqual(width);
});
