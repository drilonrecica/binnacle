import { expect, test } from '@playwright/test';
import { mockApp } from './fixtures';

// §Server: instruments, charts with gaps and ranges, and its tabs.

test('renders instruments and historical charts', async ({ page }) => {
  await mockApp(page);
  await page.goto('/server');
  await expect(
    page.getByRole('heading', { name: 'Server', level: 1 }),
  ).toBeVisible();
  await expect(
    page.getByRole('meter', { name: 'CPU', exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { name: 'Load average' }),
  ).toBeVisible();
  await expect(
    page.getByRole('button', { name: /Inspect CPU/ }),
  ).toBeAttached();
});

test('charts switch ranges and surface data gaps', async ({ page }) => {
  await mockApp(page);
  const requests: string[] = [];
  await page.route('**/api/v1/metrics?*', (route) => {
    requests.push(route.request().url());
    return route.fulfill({
      json: {
        scope: 'host',
        from: '2026-07-11T11:00:00Z',
        to: '2026-07-11T12:00:00Z',
        resolution: '1m',
        series: [
          {
            metric: 'cpu',
            unit: 'percent',
            points: [
              { at: '2026-07-11T11:00:00Z', min: 1, avg: 2, max: 3, count: 1 },
            ],
          },
        ],
        gaps: [
          {
            from: '2026-07-11T11:05:00Z',
            to: '2026-07-11T11:25:00Z',
            reason: 'collector_unavailable',
          },
        ],
      },
    });
  });
  await page.goto('/server');
  await expect(
    page.getByRole('heading', { name: 'Server', level: 1 }),
  ).toBeVisible();
  await expect(page.getByText('1 gap')).toBeVisible();
  const before = requests.length;
  await page.getByRole('button', { name: '6h', exact: true }).click();
  await expect.poll(() => requests.length).toBeGreaterThan(before);
  await expect(page).toHaveURL(/range=6h/);

  await page.getByRole('button', { name: 'Custom' }).click();
  const popover = page.getByRole('dialog', { name: 'Custom time range' });
  await popover.getByLabel('From').fill('2026-07-11T10:00');
  await popover.getByLabel('To').fill('2026-07-11T12:00');
  await popover.getByRole('button', { name: 'Apply range' }).click();
  await expect(page).toHaveURL(/range=custom/);
});

test('tabs show filesystems, processes and collectors', async ({ page }) => {
  await mockApp(page);
  await page.route('**/api/v1/processes?*', (route) =>
    route.fulfill({
      json: {
        sampled: true,
        processes: [
          {
            pid: 4242,
            name: 'postgres',
            command: 'postgres -D [redacted]',
            cpuPct: 3.5,
            memoryBytes: 104_857_600,
          },
        ],
      },
    }),
  );
  await page.goto('/server');
  await page.getByRole('tab', { name: /Filesystems/ }).click();
  await expect(page).toHaveURL(/tab=filesystems/);
  await expect(page.getByRole('tabpanel')).toContainText('ext4');
  await page.getByRole('tab', { name: 'Processes' }).click();
  await expect(page.getByRole('tabpanel')).toContainText('postgres');
  await page.getByRole('tab', { name: /Collectors/ }).click();
  // Collectors read as a word and a reason, never only a dot.
  await expect(page.getByRole('tabpanel')).toContainText(/healthy/i);
});
