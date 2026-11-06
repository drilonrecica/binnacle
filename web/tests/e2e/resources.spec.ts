import { expect, test } from '@playwright/test';
import { mockApp } from './fixtures';

// §Resources: list with filters, sort, grouping and cards; detail with tabs.

test('filters, sorts, and opens a detail with its containers', async ({
  page,
}, testInfo) => {
  const mobile = testInfo.project.name.includes('mobile');
  await mockApp(page);
  await page.goto('/resources');

  const table = page.getByRole(mobile ? 'list' : 'table', {
    name: 'Resources',
  });
  await expect(table).toBeVisible();
  await expect(page.getByText('binnacle / production')).toBeVisible();
  await expect(page.getByText('Infrastructure')).toBeVisible();

  if (!mobile) {
    await page.getByRole('button', { name: 'CPU' }).click();
    await expect(
      page.getByRole('columnheader', { name: 'CPU' }),
    ).toHaveAttribute('aria-sort', 'descending');
    await expect
      .poll(() =>
        page.evaluate(() => localStorage.getItem('binnacle.resources.sort.v2')),
      )
      .toContain('"key":"cpu"');
  }

  await page.getByLabel('Filter resources').fill('worker');
  await expect(page.getByText('api.production')).toHaveCount(0);
  await expect(page.getByText('Starting')).toBeVisible();

  await page.getByRole('link', { name: 'worker.production' }).click();
  await expect(page).toHaveURL(/\/resources\/res_beta$/);
  await expect(
    page.getByRole('heading', { name: 'worker.production', level: 1 }),
  ).toBeVisible();
  // Every detail has a breadcrumb back (§IA).
  await expect(
    page.getByRole('navigation', { name: 'Breadcrumb' }),
  ).toBeVisible();
  await page.getByRole('tab', { name: /Containers/ }).click();
  // Every table is a list of cards on a phone.
  const containers = page.getByRole(mobile ? 'list' : 'table', {
    name: 'Containers',
  });
  await expect(containers).toContainText('worker-1');
  await expect(containers).toContainText(/starting/i);
});

test('switches to cards and reads logs in the detail tab', async ({ page }) => {
  await mockApp(page);
  await page.goto('/resources');
  await page.getByRole('button', { name: 'Cards' }).click();
  await expect(page.getByRole('list', { name: 'Resources' })).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => localStorage.getItem('binnacle.resources.view')),
    )
    .toBe('cards');

  await page.route('**/api/v1/logs?*', (route) =>
    route.fulfill({
      json: {
        entries: [
          {
            timestamp: new Date().toISOString(),
            component: 'c1',
            stream: 'stderr',
            severity: 'error',
            message: 'connection refused',
          },
        ],
        truncated: false,
        redaction: 'best-effort',
      },
    }),
  );
  await page.goto('/resources/res1');
  await page.getByRole('tab', { name: 'Logs' }).click();
  await expect(page).toHaveURL(/\/resources\/res1\?tab=logs$/);
  await expect(page.getByRole('region', { name: 'Log lines' })).toContainText(
    'connection refused',
  );
  // /logs now opens the Logs tab of the resource viewed last.
  await page.goto('/logs');
  await expect(page).toHaveURL(/\/resources\/res1\?tab=logs$/);
});

test('detail tabs follow the URL and the arrow keys', async ({ page }) => {
  await mockApp(page);
  await page.goto('/resources/res1?tab=containers');
  const containers = page.getByRole('tab', { name: /Containers/ });
  await expect(containers).toHaveAttribute('aria-selected', 'true');
  await containers.focus();
  await page.keyboard.press('ArrowRight');
  const next = page.getByRole('tab', { selected: true });
  await expect(next).toBeFocused();
  await expect(next).not.toHaveText(/Containers/);
  await expect(page).toHaveURL(/tab=/);
});

test('an unknown resource says so instead of an empty page', async ({
  page,
}) => {
  await mockApp(page);
  await page.route('**/api/v1/resources/missing*', (route) =>
    route.fulfill({ status: 404, json: { error: { code: 'not_found' } } }),
  );
  await page.goto('/resources/missing');
  await expect(
    page.getByRole('heading', { name: 'Resource not found', level: 1 }),
  ).toBeVisible();
  await expect(
    page.getByRole('link', { name: 'Back to resources' }),
  ).toBeVisible();
  await expect(
    page
      .getByRole('navigation', { name: 'Breadcrumb' })
      .getByRole('link', { name: 'Resources' }),
  ).toBeVisible();
});
