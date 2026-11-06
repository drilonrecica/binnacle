import { expect, test } from '@playwright/test';
import { mockApp } from './fixtures';

// §Changes: timeline with day separators, repeat grouping and filters.

const events = [
  {
    id: '01J2ZQ6V8M0000000000000040',
    ts: '2026-07-11T11:40:00Z',
    type: 'deployment',
    severity: 'info',
    summary: 'api.production deployed',
    resourceId: 'res1',
    source: 'docker',
  },
  {
    id: '01J2ZQ6V8M0000000000000041',
    ts: '2026-07-11T11:58:00Z',
    type: 'container_oom',
    severity: 'critical',
    summary: 'worker.production was OOM killed',
    resourceId: 'res_beta',
    source: 'docker',
  },
];

test('shows the history and narrows it by family', async ({ page }) => {
  await mockApp(page);
  const requests: string[] = [];
  await page.route('**/api/v1/events?*', (route) => {
    requests.push(route.request().url());
    return route.fulfill({ json: events });
  });
  await page.goto('/changes');
  await expect(
    page.getByRole('heading', { name: 'Changes', level: 1 }),
  ).toBeVisible();
  await expect(page.getByText('api.production deployed')).toBeVisible();
  await expect(
    page.getByText('worker.production was OOM killed'),
  ).toBeVisible();
  // Each item links to its resource.
  await expect(
    page.getByRole('link', { name: 'worker.production' }).first(),
  ).toHaveAttribute('href', /\/resources\/res_beta/);

  const before = requests.length;
  await page.getByLabel('Event family').selectOption({ label: 'Deploys' });
  await expect.poll(() => requests.length).toBeGreaterThan(before);
  await expect(page).toHaveURL(/family=deploys/);
  expect(requests.at(-1)).toContain('deployment');
});

test('a failed history request says so instead of an empty feed', async ({
  page,
}) => {
  await mockApp(page);
  await page.route('**/api/v1/events?*', (route) =>
    route.fulfill({ status: 500, json: { error: { code: 'internal' } } }),
  );
  await page.goto('/changes');
  await expect(page.getByRole('alert').first()).toBeVisible();
});
