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
  // One legend per chart: the latest-value list above the plot, not a
  // second copy from the chart primitive below it.
  const cpu = page.locator('.chart-slot').filter({
    has: page.getByRole('heading', { name: 'CPU', exact: true }),
  });
  await expect(cpu.getByText('Busy', { exact: true })).toHaveCount(1);
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

test('charts rescale the time axis when the range changes', async ({
  page,
}) => {
  await mockApp(page);
  // 61 evenly spaced points across whatever window was asked for; each
  // point's value is its own index, so the hover readout says where on the
  // time axis the cursor landed.
  const count = 61;
  const requests: string[] = [];
  await page.route('**/api/v1/metrics?*', (route) => {
    const url = new URL(route.request().url());
    requests.push(url.search);
    const from = new Date(url.searchParams.get('from') ?? '').getTime();
    const to = new Date(url.searchParams.get('to') ?? '').getTime();
    const step = (to - from) / (count - 1);
    const metrics = (url.searchParams.get('metrics') ?? '').split(',');
    return route.fulfill({
      json: {
        scope: 'host',
        from: new Date(from).toISOString(),
        to: new Date(to).toISOString(),
        resolution: '1m',
        series: metrics.map((metric) => ({
          metric,
          unit: 'percent',
          points: Array.from({ length: count }, (_, index) => ({
            at: new Date(from + index * step).toISOString(),
            min: index,
            avg: index,
            max: index,
            count: 1,
          })),
        })),
        gaps: [],
      },
    });
  });
  await page.goto('/server?range=7d');
  const cpu = page.locator('.chart-slot').filter({
    has: page.getByRole('heading', { name: 'CPU', exact: true }),
  });
  const plot = cpu.locator('.u-over');
  await expect(plot).toBeVisible();

  const before = requests.length;
  await page.getByRole('button', { name: '1h', exact: true }).click();
  await expect(page).toHaveURL(/range=1h/);
  await expect.poll(() => requests.length).toBeGreaterThan(before);

  // The middle of the plot is the middle of the hour. An axis left on the
  // first week would put the whole hour at the right edge and snap to 0.
  const value = cpu.locator('.tooltip-value').first();
  await expect(async () => {
    await page.mouse.move(0, 0);
    await plot.hover();
    const hovered = Number.parseFloat((await value.textContent()) ?? '');
    expect(hovered).toBeGreaterThan(25);
    expect(hovered).toBeLessThan(35);
  }).toPass({ timeout: 5000 });
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

test('the Live tab shows the whole machine and refreshes processes', async ({
  page,
}) => {
  await mockApp(page);
  let scans = 0;
  await page.route('**/api/v1/processes?*', (route) => {
    scans++;
    return route.fulfill({
      json: {
        sampled: true,
        sampledAt: '2026-07-11T12:00:00Z',
        tasks: {
          total: 292,
          threads: 1126,
          running: 1,
          sleeping: 219,
          other: 72,
        },
        processes: [
          {
            pid: 2758,
            command: 'python3 -m glances -w',
            cpuPct: 13.7,
            memPct: 7.3,
            rssBytes: 593_494_016,
            virtBytes: 682_622_976,
            user: 'root',
            state: 'S',
            threads: 33,
            nice: 0,
            cpuTimeSeconds: 1_665_724,
          },
        ],
      },
    });
  });
  await page.goto('/server?tab=live');
  const panel = page.getByRole('tabpanel');
  await expect(page.getByTestId('live-identity')).toContainText('vps-01');
  await expect(page.getByTestId('live-identity')).toContainText(
    'Ubuntu 24.04.1 LTS',
  );
  await expect(
    panel
      .getByRole('list', { name: 'Busy share per core' })
      .getByRole('listitem'),
  ).toHaveCount(4);
  await expect(panel).toContainText('eth0');
  await expect(panel).toContainText('sda');
  await expect(panel).toContainText('api-1');
  await expect(panel).toContainText('210 MiB / 8 GiB');
  await expect(panel).toContainText('3000/tcp');
  await expect(panel).toContainText('python3 -m glances -w');
  await expect(panel).toContainText('462:42:04');
  await expect(panel).toContainText('1126');
  // Refreshes while visible, every 5 s.
  const first = scans;
  await expect.poll(() => scans, { timeout: 12_000 }).toBeGreaterThan(first);
});
