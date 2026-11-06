import { expect, test } from '@playwright/test';
import { mockApp, snapshot } from './fixtures';

// §Headroom: title row, attention strip, instruments, resources, changes.

test('renders the instruments and the grouped resource table', async ({
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
  expect(box?.width).toBeLessThanOrEqual(page.viewportSize()?.width ?? 0);
  // Raw ids never appear as titles (§IA).
  await expect(page.getByText('res_beta', { exact: true })).toHaveCount(0);
});

test('a gauge links to Server and a row links to its resource', async ({
  page,
}) => {
  await mockApp(page);
  await page.goto('/headroom');
  await page.getByRole('link', { name: /^CPU \d/ }).click();
  await expect(page).toHaveURL(/\/server/);
  await page.goBack();
  await page.getByRole('link', { name: 'api.production' }).click();
  await expect(page).toHaveURL(/\/resources\/res1$/);
  await expect(
    page.getByRole('heading', { name: 'api.production', level: 1 }),
  ).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(/\/headroom$/);
});

test('open incidents and unhealthy resources fill the attention strip', async ({
  page,
}) => {
  const value = structuredClone(snapshot);
  value.resources[1]!.status = 'degraded';
  await mockApp(page, { liveSnapshot: value });
  await page.route('**/api/v1/incidents?*', (route) =>
    route.fulfill({
      json: [
        {
          id: 'inc-1',
          status: 'open',
          severity: 'critical',
          title: 'resource incident on res_beta',
          targetType: 'resource',
          targetId: 'res_beta',
          alertCount: 1,
          firingAlertCount: 1,
          openedAt: '2026-07-11T11:50:00Z',
        },
      ],
    }),
  );
  await page.goto('/headroom');
  const strip = page.getByRole('region', { name: /attention/i });
  await expect(strip).toContainText('worker.production');
  await expect(page.getByText('Nothing needs attention')).toHaveCount(0);
});

test('the layout stays inside a phone viewport', async ({ page }) => {
  await mockApp(page);
  await page.goto('/headroom');
  const width = page.viewportSize()?.width ?? 0;
  const scrollWidth = await page.evaluate(
    () => document.documentElement.scrollWidth,
  );
  expect(scrollWidth).toBeLessThanOrEqual(width);
  const heading = page.getByRole('heading', { name: 'Headroom', level: 1 });
  const box = await heading.boundingBox();
  expect(box?.width).toBeLessThanOrEqual(width);
});
