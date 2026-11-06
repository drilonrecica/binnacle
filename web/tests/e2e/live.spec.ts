import { expect, test, type Page } from '@playwright/test';
import {
  dropLive,
  emitLive,
  mockApp,
  openedStreams,
  snapshot,
} from './fixtures';

// The live stream as the app sees it (UI-2, UI-5, UI-7): the mock keeps it
// open, and each test ends it on purpose to check what the user is shown.

function status(page: Page) {
  // Desktop shows the status in the page title row, phones in the top bar;
  // either way there is exactly one visible status region.
  return page.getByTestId('connection-status').locator('visible=true').first();
}

test('the stream stays open and live frames update the page', async ({
  page,
}) => {
  await mockApp(page);
  await page.goto('/headroom');
  await expect(status(page)).toContainText('Live');
  const cpu = page.getByRole('link', { name: /^CPU 24/ });
  await expect(cpu).toBeVisible();

  // A held stream is never reopened: one stream, no flapping.
  await page.waitForTimeout(1_500);
  expect(await openedStreams(page)).toHaveLength(1);

  await emitLive(
    page,
    'snapshot',
    {
      ...snapshot,
      seq: 2,
      ts: '2026-07-11T12:00:02Z',
      host: { ...snapshot.host, cpuPct: 77 },
    },
    '2',
  );
  await expect(page.getByRole('link', { name: /^CPU 77/ })).toBeVisible();
  await expect(status(page)).toContainText('Live');
  expect(await openedStreams(page)).toHaveLength(1);
});

test('a live event joins the Changes feed without a reload', async ({
  page,
}) => {
  await mockApp(page);
  await page.goto('/changes');
  await expect(
    page.getByRole('heading', { name: 'Changes', level: 1 }),
  ).toBeVisible();
  await expect(status(page)).toContainText('Live');
  await emitLive(
    page,
    'event',
    {
      id: '01J2ZQ6V8M0000000000000043',
      seq: 3,
      at: '2026-07-11T12:00:03Z',
      type: 'container_restart',
      severity: 'warning',
      message: 'worker.production restarted',
      resourceId: 'res_beta',
    },
    '3',
  );
  await expect(page.getByText('worker.production restarted')).toBeVisible();
});

test('quiet data turns stale with its age and never claims an all-clear', async ({
  page,
}) => {
  await page.clock.install();
  await mockApp(page);
  await page.goto('/headroom');
  await expect(status(page)).toContainText('Live');
  await expect(page.getByText('Nothing needs attention')).toBeVisible();

  await page.clock.fastForward(15_000);
  await expect(status(page)).toContainText('Stale');
  await expect(page.getByTestId('instruments')).toHaveClass(/stale/);
  await expect(page.getByText('Nothing needs attention')).toHaveCount(0);
  await expect(page.getByText(/Status unknown since/)).toBeVisible();
});

test('a dropped stream shows the disconnect bar, keeps the last data and resumes from the last id', async ({
  page,
}) => {
  await mockApp(page);
  await page.goto('/server');
  await expect(status(page)).toContainText('Live');
  await expect(
    page.getByRole('meter', { name: 'CPU', exact: true }),
  ).toBeVisible();

  // The server is down: the stream drops and every reopen fails.
  await page.route('**/api/v1/live*', (route) =>
    route.fulfill({ status: 503, body: 'unavailable' }),
  );
  await dropLive(page);

  const bar = page.getByText('Live updates disconnected');
  await expect(bar).toBeVisible();
  await expect(status(page)).toContainText('Disconnected');
  await expect(
    page.getByText(/Last update \d{1,2}:\d{2}/).first(),
  ).toBeVisible();
  // The last snapshot stays on screen instead of an empty page.
  await expect(
    page.getByRole('meter', { name: 'CPU', exact: true }),
  ).toBeVisible();
  // It keeps trying on its own, resuming from the last frame it saw.
  await expect.poll(() => openedStreams(page)).toHaveLength(2);
  expect((await openedStreams(page))[1]).toContain('lastEventId=1');

  // The server is back; "Retry now" does not wait for the backoff.
  await page.unroute('**/api/v1/live*');
  await page.route('**/api/v1/live*', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body: `event: snapshot\nid: 5\ndata: ${JSON.stringify({ ...snapshot, seq: 5 })}\n\n`,
    }),
  );
  // The page keeps its own stale banner with the same action; use the bar's.
  const retries = page.getByRole('button', { name: 'Retry now' });
  await expect(retries).toHaveCount(2);
  await page
    .locator('[data-state="disconnected"]')
    .getByRole('button', { name: 'Retry now' })
    .click();
  await expect(bar).toBeHidden();
  await expect(retries).toHaveCount(0);
  await expect(status(page)).toContainText('Live');
  const opened = await openedStreams(page);
  expect(opened.at(-1)).toContain('lastEventId=1');
});

test('an expired session on the stream sends the user to sign in and back', async ({
  page,
}) => {
  await mockApp(page);
  await page.goto('/server?tab=collectors');
  await expect(status(page)).toContainText('Live');

  // The session expires: the stream closes and the session check says so.
  await page.route('**/api/v1/live*', (route) =>
    route.fulfill({ status: 401, json: { error: { code: 'unauthorized' } } }),
  );
  await page.route('**/api/v1/session', (route) =>
    route.fulfill({ status: 401 }),
  );
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill({ status: 401, json: { error: { code: 'unauthorized' } } }),
  );
  await dropLive(page);

  await expect(page).toHaveURL(/\/login\?next=%2Fserver%3Ftab%3Dcollectors$/);
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  // The shell is gone, not left empty behind the sign-in card.
  await expect(
    page.getByRole('navigation', { name: 'Primary navigation' }),
  ).toHaveCount(0);

  // Signing in again returns to the page the user was on (UI-5).
  await page.unroute('**/api/v1/live*');
  await mockApp(page);
  await page.route('**/api/v1/auth/login', (route) =>
    route.fulfill({ json: { user: { id: 'admin', username: 'admin' } } }),
  );
  await page.getByLabel('Username').fill('admin');
  await page.getByLabel('Password').fill('correct horse battery staple');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/server\?tab=collectors$/);
  await expect(status(page)).toContainText('Live');
});

test('a 401 from a page request signs out only once the session check agrees', async ({
  page,
}) => {
  await mockApp(page);
  // One stray 401 while the session is still good keeps the user here.
  await page.route('**/api/v1/monitor-health', (route) =>
    route.fulfill({ status: 401, json: { error: { code: 'unauthorized' } } }),
  );
  await page.goto('/settings/system');
  await expect(
    page.getByRole('heading', { name: 'Settings', level: 1 }),
  ).toBeVisible();
  await page.waitForTimeout(500);
  await expect(page).toHaveURL(/\/settings\/system$/);

  // Now the session really is gone.
  await page.route('**/api/v1/session', (route) =>
    route.fulfill({ status: 401 }),
  );
  await page.goto('/settings/system');
  await expect(page).toHaveURL(/\/login\?next=%2Fsettings%2Fsystem$/);
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
});
