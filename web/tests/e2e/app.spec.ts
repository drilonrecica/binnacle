import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'admin', username: 'admin' },
  expiresAt: '2026-07-11T13:00:00Z',
  absoluteExpiresAt: '2026-07-11T14:00:00Z',
};

async function mockAuthSession(page: Page) {
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill({ json: session }),
  );
}

async function mockOnboarding(page: Page) {
  await page.route('**/api/v1/onboarding', (route) =>
    route.fulfill({
      json: {
        checklistDismissed: true,
        completedAt: '2026-07-11T11:00:00Z',
      },
    }),
  );
}

async function mockSettings(page: Page) {
  const values: Record<
    string,
    { value: string; source: string; applyMode: string }
  > = {
    'collection.host_interval': {
      value: '10s',
      source: 'Default',
      applyMode: 'live',
    },
    'collection.container_interval': {
      value: '10s',
      source: 'Default',
      applyMode: 'live',
    },
    'persistence.raw_interval': {
      value: '10s',
      source: 'Default',
      applyMode: 'live',
    },
    'retention.preset': {
      value: 'balanced',
      source: 'Default',
      applyMode: 'live',
    },
    'retention.raw': { value: '24h', source: 'Default', applyMode: 'live' },
    'retention.one_minute': {
      value: '7d',
      source: 'Default',
      applyMode: 'live',
    },
    'retention.fifteen_minute': {
      value: '30d',
      source: 'Default',
      applyMode: 'live',
    },
    'retention.one_hour': {
      value: '365d',
      source: 'Default',
      applyMode: 'live',
    },
    'database.target_budget_bytes': {
      value: '1073741824',
      source: 'Default',
      applyMode: 'live',
    },
    'sessions.idle_timeout': {
      value: '15m',
      source: 'Default',
      applyMode: 'live',
    },
    'sessions.absolute_lifetime': {
      value: '24h',
      source: 'Default',
      applyMode: 'live',
    },
    'http.listen_address': {
      value: ':8080',
      source: 'Default',
      applyMode: 'restart_required',
    },
    'docker.socket_path': {
      value: '/var/run/docker.sock',
      source: 'Default',
      applyMode: 'restart_required',
    },
    'paths.host_proc': {
      value: '/host/proc',
      source: 'Default',
      applyMode: 'restart_required',
    },
    'paths.host_sys': {
      value: '/host/sys',
      source: 'Default',
      applyMode: 'restart_required',
    },
    'paths.data_dir': {
      value: '/var/lib/binnacle',
      source: 'Default',
      applyMode: 'restart_required',
    },
  };
  await page.route('**/api/v1/settings', (route) =>
    route.fulfill({ json: { revision: 1, values } }),
  );
}

test('renders the Binnacle application shell', async ({ page }) => {
  await mockAuthSession(page);
  await mockOnboarding(page);
  await page.route('**/api/v1/live', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: '' }),
  );
  await page.goto('/watch');

  await expect(page).toHaveTitle('Binnacle — Watch');
  await expect(page.getByRole('link', { name: 'Binnacle' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Watch' })).toBeVisible();
});

test('creates a health check from the Alerts console', async ({ page }) => {
  await mockAuthSession(page);
  await mockOnboarding(page);
  await page.route('**/api/v1/live', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: '' }),
  );
  await page.route('**/api/v1/alerts?*', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/alert-rules', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/silences', (route) =>
    route.fulfill({ json: [] }),
  );
  let created = false;
  await page.route('**/api/v1/checks', async (route) => {
    if (route.request().method() === 'POST') {
      created = true;
      return route.fulfill({ status: 201, json: { id: 'check' } });
    }
    return route.fulfill({ json: [] });
  });
  await page.goto('/alerts');
  await expect(page.getByRole('heading', { name: 'Alerts', exact: true })).toBeVisible();
  await page.getByRole('tab', { name: 'checks' }).click();
  await page.getByLabel('Resource ID').fill('res_demo_1');
  await page.getByLabel('Name').fill('Public health');
  await page.getByLabel('HTTP/HTTPS URL').fill('https://example.com/health');
  await page.getByRole('button', { name: 'Create check' }).click();
  await expect.poll(() => created).toBe(true);
});

test('removed and unknown routes fall back to Watch', async ({ page }) => {
  await mockAuthSession(page);
  await mockOnboarding(page);
  await page.route('**/api/v1/live', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: '' }),
  );
  await page.goto('/overview');
  await expect(page).toHaveURL(/\/watch$/);
  await expect(page.getByRole('link', { name: 'Checks' })).toHaveCount(0);
  await page.goto('/not-a-real-view');
  await expect(page).toHaveURL(/\/watch$/);
});

test('switches every historical range without hiding gaps', async ({
  page,
}) => {
  await mockAuthSession(page);
  await mockOnboarding(page);
  await page.route('**/api/v1/live', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body: `event: snapshot\nid: 1\ndata: {"seq":1,"ts":"2026-07-11T12:00:00Z","bootIdentity":"boot","host":{"cpuPct":10,"memoryUsedBytes":1024,"load1":0.1,"networkRxBps":2,"networkTxBps":3},"resources":[],"collectors":{}}\n\n`,
    }),
  );
  await page.route('**/api/v1/events?*', (route) =>
    route.fulfill({
      json: [
        {
          ts: '2026-07-11T11:30:00Z',
          type: 'deployment',
          summary: 'Deployment',
        },
      ],
    }),
  );
  await page.route('**/api/v1/metrics?*', (route) => {
    const query = new URL(route.request().url()).searchParams;
    const span =
      new Date(query.get('to')!).getTime() -
      new Date(query.get('from')!).getTime();
    return route.fulfill({
      json: {
        scope: 'host',
        from: '2026-07-11T11:00:00Z',
        to: '2026-07-11T12:00:00Z',
        resolution: span > 10 * 24 * 60 * 60 * 1000 ? '1h' : '10s',
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
            from: '2026-07-11T11:10:00Z',
            to: '2026-07-11T11:20:00Z',
            reason: 'collector_unavailable',
          },
        ],
      },
    });
  });
  await page.goto('/');
  await page.getByRole('link', { name: 'Server', exact: true }).click();
  await expect(
    page.getByRole('heading', { name: 'Historical telemetry' }),
  ).toBeVisible();
  for (const range of ['1h', '6h', '24h', '7d', '30d'])
    await page.getByRole('button', { name: range, exact: true }).click();
  await expect(page.getByText('Resolution: 1h.')).toBeVisible();
  await expect(page.getByText('1 explicit data gap.')).toBeVisible();
  await expect(page.getByText('1 event annotation')).toBeVisible();
  await page.getByText('1 data gap', { exact: true }).click();
  await expect(page.getByText(/collector unavailable/)).toBeVisible();
  const inspector = page.getByRole('button', {
    name: 'CPU (host-normalized %) chart inspection',
  });
  await inspector.focus();
  await page.keyboard.press('ArrowRight');
  await expect(inspector).toContainText('Selected point');
  await page.getByRole('button', { name: 'Custom' }).click();
  await page
    .getByRole('textbox', { name: 'From', exact: true })
    .fill('2026-07-11T12:00');
  await page
    .getByRole('textbox', { name: 'To', exact: true })
    .fill('2026-07-10T12:00');
  await page.getByRole('button', { name: 'Apply range' }).click();
  await expect(page.getByRole('alert')).toContainText(
    'end time after the start',
  );
  await page.setViewportSize({ width: 390, height: 844 });
  const box = await page
    .getByRole('heading', { name: 'Historical telemetry' })
    .boundingBox();
  expect(box?.width).toBeLessThanOrEqual(390);
});

test('requires typed confirmation for history deletion', async ({ page }) => {
  await mockAuthSession(page);
  await mockOnboarding(page);
  await mockSettings(page);
  await page.route('**/api/v1/live', (route) =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: '' }),
  );
  await page.route('**/api/v1/history/deletion-previews', (route) =>
    route.fulfill({
      json: {
        token: 'preview',
        confirmation: 'RESET ALL HISTORY',
        totalRows: 42,
        expiresAt: '2026-07-11T13:00:00Z',
      },
    }),
  );
  await page.route('**/api/v1/history/deletion-jobs', (route) =>
    route.fulfill({
      status: 202,
      json: { id: 'del_test', state: 'queued', totalRows: 42, deletedRows: 0 },
    }),
  );
  await page.route('**/api/v1/history/deletion-jobs/del_test', (route) =>
    route.fulfill({
      json: {
        id: 'del_test',
        state: 'completed',
        totalRows: 42,
        deletedRows: 42,
      },
    }),
  );
  await page.goto('/');
  await page.getByRole('link', { name: 'Settings', exact: true }).click();
  await page.getByLabel('Scope').selectOption('all');
  await page.getByRole('button', { name: 'Preview deletion' }).click();
  const remove = page.getByRole('button', { name: 'Delete history' });
  await expect(remove).toBeDisabled();
  await page.getByLabel('Confirmation').fill('RESET ALL HISTORY');
  await remove.click();
  await expect(
    page.getByText('completed: 42 of 42 rows deleted.'),
  ).toBeVisible();
});
