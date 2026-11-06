import { expect, test, type Page } from '@playwright/test';
import { mockApp } from './fixtures';

// §Incidents: tabs Open, Resolved, Firing, Checks, Silences; incident detail.

const incident = {
  id: 'inc-1',
  status: 'open',
  severity: 'critical',
  title: 'resource incident on res1',
  targetType: 'resource',
  targetId: 'res1',
  alertCount: 2,
  firingAlertCount: 1,
  openedAt: '2026-07-11T12:00:00Z',
};

async function mockIncident(page: Page) {
  await page.route('**/api/v1/incidents?*', (route) =>
    route.fulfill({ json: [incident] }),
  );
  await page.route('**/api/v1/incidents', (route) =>
    route.fulfill({ json: [incident] }),
  );
  await page.route('**/api/v1/incidents/inc-1', (route) =>
    route.fulfill({
      json: {
        ...incident,
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
        deliveries: [
          {
            id: 'delivery-1',
            channelId: 'channel-existing',
            eventType: 'opened',
            status: 'succeeded',
            attemptCount: 1,
            updatedAt: '2026-07-11T12:01:00Z',
          },
        ],
      },
    }),
  );
  await page.route('**/api/v1/notification-channels', (route) =>
    route.fulfill({
      json: [
        {
          id: 'channel-existing',
          name: 'Existing webhook',
          kind: 'webhook',
          enabled: true,
          minimumSeverity: 'warning',
          notifyResolved: true,
          config: { url: 'https://hooks.example.com/existing' },
          secretConfigured: true,
        },
      ],
    }),
  );
}

test('lists open incidents and opens the detail with alerts and deliveries', async ({
  page,
}) => {
  await mockApp(page);
  await mockIncident(page);
  await page.goto('/incidents');
  await expect(page.getByRole('tab', { name: /Open/ })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  // Titles use the resource name, never its id (§IA).
  await page
    .getByRole('link', { name: 'Resource incident on api.production' })
    .first()
    .click();
  await expect(page).toHaveURL(/\/incidents\/inc-1$/);
  await expect(page.getByText('API is unhealthy').first()).toBeVisible();
  await expect(page.getByText('Existing webhook').first()).toBeVisible();
  await expect(
    page
      .getByRole('navigation', { name: 'Breadcrumb' })
      .getByRole('link', { name: 'Incidents' }),
  ).toBeVisible();

  // The seen marker is local to this browser and survives a reload (UI-21).
  await page.getByRole('button', { name: 'Mark as seen' }).click();
  await expect(page.getByText(/this browser only/)).toBeVisible();
  await page.reload();
  await expect(page.getByText(/this browser only/)).toBeVisible();
  await page.goto('/incidents');
  await expect(page.getByText(/Seen by/).first()).toBeVisible();
});

test('each tab loads on its own and a failed one says so (UI-9)', async ({
  page,
}) => {
  await mockApp(page);
  await mockIncident(page);
  await page.route('**/api/v1/alerts?*', (route) =>
    route.fulfill({ status: 500, json: { error: { code: 'internal' } } }),
  );
  await page.goto('/incidents');
  await expect(
    page.getByRole('link', { name: 'Resource incident on api.production' }),
  ).toBeVisible();
  await page.getByRole('tab', { name: /Firing/ }).click();
  await expect(page).toHaveURL(/tab=firing/);
  await expect(page.getByRole('tabpanel')).toContainText(/could not|failed/i);
  // The other tabs still work.
  await page.getByRole('tab', { name: /Open/ }).click();
  await expect(
    page.getByRole('link', { name: 'Resource incident on api.production' }),
  ).toBeVisible();
  await page.getByRole('tab', { name: /Checks/ }).click();
  await expect(page.getByRole('link', { name: /Manage checks/ })).toBeVisible();
});

test('creates a silence for a resource from the silence dialog', async ({
  page,
}) => {
  await mockApp(page);
  let body: Record<string, unknown> | null = null;
  await page.route('**/api/v1/silences', (route) => {
    if (route.request().method() === 'POST') {
      body = route.request().postDataJSON();
      return route.fulfill({ status: 201, json: { id: 'sil-1', ...body } });
    }
    return route.fulfill({ json: [] });
  });
  await page.goto('/incidents');
  const open = page.getByRole('button', { name: 'Silence…' });
  await open.click();
  const dialog = page.getByRole('dialog', { name: 'Silence notifications' });
  await expect(dialog).toBeVisible();
  // The reason is the first thing to type and is required.
  await expect(dialog.getByLabel('Reason')).toBeFocused();
  await dialog.getByLabel('Scope').selectOption('resource');
  await dialog.getByRole('button', { name: 'Create silence' }).click();
  await expect(dialog.getByRole('alert')).toHaveText('Choose a resource.');
  expect(body).toBeNull();

  await dialog.getByLabel('Resource').selectOption('res1');
  await dialog.getByLabel('Duration').selectOption('4h');
  await dialog.getByLabel('Reason').fill('Planned maintenance');
  await dialog.getByRole('button', { name: 'Create silence' }).click();
  await expect.poll(() => body).not.toBeNull();
  expect(body).toMatchObject({
    scopeType: 'resource',
    scopeId: 'res1',
    preset: '4h',
    reason: 'Planned maintenance',
  });
  await expect(dialog).toBeHidden();
  // Focus returns to the control that opened the dialog.
  await expect(open).toBeFocused();
});
