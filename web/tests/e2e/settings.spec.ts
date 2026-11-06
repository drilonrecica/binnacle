import { expect, test } from '@playwright/test';
import { mockApp, settingsValues } from './fixtures';

// §Settings: eight sections, one form per card with explicit save and a
// guard against leaving with unsaved changes; dialogs for every editor.

const sections = [
  ['general', 'Collection'],
  ['data', 'Retention'],
  ['access', 'Sessions'],
  ['notifications', 'Channels'],
  ['alerting', 'Alert rules'],
  ['appearance', 'Theme'],
  ['integrations', 'Coolify'],
  ['system', 'Background workers'],
] as const;

test('every section opens from its link and the URL', async ({ page }) => {
  await mockApp(page);
  for (const [section, heading] of sections) {
    await page.goto(`/settings/${section}`);
    await expect(
      page.getByRole('heading', { name: 'Settings', level: 1 }),
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: heading, exact: true }).first(),
    ).toBeVisible();
  }
});

test('saves a card explicitly and only that card', async ({ page }) => {
  await mockApp(page);
  let body: { revision: number; changes: Record<string, string> } | null = null;
  await page.route('**/api/v1/settings', (route) => {
    if (route.request().method() === 'PATCH') {
      body = route.request().postDataJSON();
      const values = settingsValues();
      values['collection.host_interval'].value = '5s';
      return route.fulfill({
        json: {
          revision: 2,
          values,
          features: { advancedAuth: false, portability: false },
        },
      });
    }
    return route.fulfill({
      json: {
        revision: 1,
        values: settingsValues(),
        features: { advancedAuth: false, portability: false },
      },
    });
  });
  await page.goto('/settings/general');
  const save = page.getByRole('button', { name: 'Save changes' });
  await expect(save).toBeDisabled();
  await page.getByLabel('Host interval').fill('5s');
  await expect(page.getByText('Unsaved changes')).toBeVisible();
  await save.click();
  await expect.poll(() => body).not.toBeNull();
  expect(body).toEqual({
    revision: 1,
    changes: { 'collection.host_interval': '5s' },
  });
  await expect(page.getByText('Unsaved changes')).toHaveCount(0);
});

test('asks before leaving with unsaved changes', async ({ page }) => {
  await mockApp(page);
  await page.goto('/settings/general');
  await page.getByLabel('Host interval').fill('5s');
  await page.getByRole('link', { name: /^Data & retention/ }).click();
  const dialog = page.getByRole('dialog', { name: 'Leave without saving?' });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Stay' }).click();
  await expect(page).toHaveURL(/\/settings\/general$/);
  await expect(page.getByLabel('Host interval')).toHaveValue('5s');

  await page.getByRole('link', { name: /^Data & retention/ }).click();
  await dialog.getByRole('button', { name: 'Discard and leave' }).click();
  await expect(page).toHaveURL(/\/settings\/data$/);
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
  expect(scrollWidth).toBeLessThanOrEqual(page.viewportSize()?.width ?? 0);
});

// Checks are read-only on Incidents; they are created under Settings › Alerting.
test('creates a health check from Settings › Alerting', async ({ page }) => {
  await mockApp(page);
  let body: Record<string, unknown> | null = null;
  await page.route('**/api/v1/checks', async (route) => {
    if (route.request().method() === 'POST') {
      body = route.request().postDataJSON();
      return route.fulfill({
        status: 201,
        json: { id: 'check', name: 'Public health', interval: 30e9 },
      });
    }
    return route.fulfill({ json: [] });
  });
  await page.goto('/settings/alerting');
  await expect(
    page.getByRole('heading', { name: 'Settings', level: 1 }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'New check' }).first().click();
  const dialog = page.getByRole('dialog', { name: 'New health check' });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('combobox', { name: 'Resource' }).fill('api');
  await dialog.getByRole('option', { name: /api\.production/ }).click();
  await dialog.getByLabel('Name').fill('Public health');
  await dialog.getByLabel('URL').fill('https://example.com/health');
  await dialog.getByRole('button', { name: 'Create check' }).click();
  await expect.poll(() => body).not.toBeNull();
  expect(body).toMatchObject({
    resourceId: 'res1',
    name: 'Public health',
    url: 'https://example.com/health',
    intervalSeconds: 30,
    required: true,
  });
});

test('manages notification channels and retries a failed delivery', async ({
  page,
}) => {
  await mockApp(page);
  let channelCreated = false;
  let smtpCreated = false;
  let channelTested = false;
  await page.route(
    '**/api/v1/notification-channels/channel-existing/test',
    (route) => {
      channelTested = true;
      return route.fulfill({ status: 202, json: { deliveryId: 'test' } });
    },
  );
  await page.route('**/api/v1/notification-channels', async (route) => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as Record<string, unknown>;
      if (body.kind === 'webhook')
        channelCreated = body.url === 'https://hooks.example.com/incidents';
      if (body.kind === 'smtp')
        smtpCreated =
          body.host === 'smtp.example.com:465' &&
          body.sender === 'binnacle@example.com' &&
          (body.recipients as string[])?.[0] === 'ops@example.com' &&
          body.tlsMode === 'implicit';
      return route.fulfill({
        status: 201,
        json: { id: 'channel-1', name: body.name, kind: body.kind, config: {} },
      });
    }
    return route.fulfill({
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
    });
  });
  let retried = false;
  await page.route('**/api/v1/notification-deliveries?*', (route) =>
    route.fulfill({
      json: [
        {
          id: 'delivery-2',
          channelId: 'channel-existing',
          eventType: 'opened',
          status: 'permanent_failure',
          attemptCount: 7,
          failureCode: 'http_400',
          updatedAt: '2026-07-11T12:00:00Z',
        },
      ],
    }),
  );
  await page.route(
    '**/api/v1/notification-deliveries/delivery-2/retry',
    (route) => {
      retried = true;
      return route.fulfill({ status: 202, json: { deliveryId: 'delivery-2' } });
    },
  );

  await page.goto('/settings/notifications');
  await page.getByRole('button', { name: 'Channel actions' }).click();
  await page.getByRole('menuitem', { name: 'Send test' }).click();
  await expect.poll(() => channelTested).toBe(true);

  await page.getByRole('button', { name: 'New channel' }).click();
  let dialog = page.getByRole('dialog', { name: 'New notification channel' });
  await dialog.getByLabel('Name').fill('Operations webhook');
  await dialog
    .getByLabel('Webhook URL')
    .fill('https://hooks.example.com/incidents');
  await dialog.getByRole('button', { name: 'Create channel' }).click();
  await expect.poll(() => channelCreated).toBe(true);

  await page.getByRole('button', { name: 'New channel' }).click();
  dialog = page.getByRole('dialog', { name: 'New notification channel' });
  await dialog.getByLabel('Name').fill('Operations email');
  await dialog.getByLabel('Type').selectOption('smtp');
  await dialog.getByLabel('SMTP host and port').fill('smtp.example.com:465');
  await dialog.getByLabel('TLS').selectOption('implicit');
  await dialog.getByLabel('Sender').fill('binnacle@example.com');
  await dialog.getByLabel('Recipients').fill('ops@example.com');
  await dialog.getByRole('button', { name: 'Create channel' }).click();
  await expect.poll(() => smtpCreated).toBe(true);

  await expect(page.getByText('Failed')).toBeVisible();
  // Toasts pause while hovered (WCAG 2.2.1), so a click that has to pass over
  // one would wait for ever on a phone; dismiss them as a user would.
  const dismiss = page.getByRole('button', { name: /^Dismiss: / });
  while ((await dismiss.count()) > 0) await dismiss.first().click();
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await expect.poll(() => retried).toBe(true);
});

test('changes the password with the current one', async ({ page }) => {
  await mockApp(page);
  let body: Record<string, unknown> | null = null;
  await page.route('**/api/v1/auth/password', (route) => {
    body = route.request().postDataJSON();
    return route.fulfill({ status: 204 });
  });
  await page.goto('/settings/access');
  await page.getByRole('button', { name: 'Change password…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Change password' });
  const submit = dialog.getByRole('button', { name: 'Change password' });
  await dialog.getByLabel('Current password').fill('old password value');
  await dialog
    .getByRole('textbox', { name: 'New password', exact: true })
    .fill('short');
  await dialog.getByLabel('Confirm new password').fill('short');
  await expect(submit).toBeDisabled();
  await dialog
    .getByRole('textbox', { name: 'New password', exact: true })
    .fill('a much longer passphrase');
  await dialog.getByLabel('Confirm new password').fill('a different one!!');
  await expect(submit).toBeDisabled();
  await dialog
    .getByLabel('Confirm new password')
    .fill('a much longer passphrase');
  await submit.click();
  await expect.poll(() => body).not.toBeNull();
  expect(body).toEqual({
    currentPassword: 'old password value',
    newPassword: 'a much longer passphrase',
  });
  await expect(dialog).toBeHidden();
});

test('deletes history with a typed confirmation and shows job progress', async ({
  page,
}) => {
  await mockApp(page);
  await page.route('**/api/v1/history/deletion-previews', (route) =>
    route.fulfill({
      json: {
        token: 'preview-token',
        confirmation: 'DELETE ALL HISTORY',
        totalRows: 9000,
        expiresAt: '2026-07-11T12:10:00Z',
      },
    }),
  );
  let started = false;
  await page.route('**/api/v1/history/deletion-jobs', (route) => {
    started =
      route.request().postDataJSON()?.confirmation === 'DELETE ALL HISTORY';
    return route.fulfill({
      status: 202,
      json: {
        id: 'job-1',
        kind: 'all',
        state: 'running',
        totalRows: 9000,
        deletedRows: 412,
      },
    });
  });
  await page.route('**/api/v1/history/deletion-jobs/job-1', (route) =>
    route.fulfill({
      json: {
        id: 'job-1',
        kind: 'all',
        state: 'completed',
        totalRows: 9000,
        deletedRows: 9000,
      },
    }),
  );
  await page.goto('/settings/data');
  await page.getByRole('radio', { name: /Everything/ }).check();
  await page.getByRole('button', { name: 'Preview deletion' }).click();
  const dialog = page.getByRole('dialog', {
    name: 'Delete history permanently?',
  });
  await expect(dialog).toContainText('9,000 rows');
  await expect(
    dialog.getByRole('button', { name: 'Delete history' }),
  ).toBeDisabled();
  await dialog.getByRole('textbox').fill('DELETE ALL HISTORY');
  await dialog.getByRole('button', { name: 'Delete history' }).click();
  await expect.poll(() => started).toBe(true);
  await expect(page.getByText('Deletion complete')).toBeVisible();
});

test('creates a scoped API token and pins a resource when portability is on', async ({
  page,
}) => {
  await mockApp(page, { portability: true });
  let tokenBody: Record<string, unknown> | null = null;
  await page.route('**/api/v1/api-tokens', async (route) => {
    if (route.request().method() === 'POST') {
      tokenBody = route.request().postDataJSON();
      return route.fulfill({
        status: 201,
        json: {
          token: {
            id: 'tok',
            name: 'Prometheus',
            prefix: 'bnk_abc',
            scopes: ['metrics:read'],
            createdAt: '2026-07-11T12:00:00Z',
          },
          plaintext: 'bnk_abc_secret',
        },
      });
    }
    return route.fulfill({
      json: { tokens: [], scopes: ['server:read', 'metrics:read'] },
    });
  });
  await page.goto('/settings/integrations');
  await page.getByRole('button', { name: 'New token' }).click();
  const dialog = page.getByRole('dialog', { name: 'New API token' });
  await dialog.getByLabel('Name').fill('Prometheus');
  await dialog.getByLabel('Host metrics').uncheck();
  await dialog.getByLabel('History and Prometheus').check();
  await dialog.getByRole('button', { name: 'Create token' }).click();
  await expect(page.getByText('bnk_abc_secret')).toBeVisible();
  expect(tokenBody).toMatchObject({
    name: 'Prometheus',
    scopes: ['metrics:read'],
  });

  let pinned: string[] | null = null;
  await page.route('**/api/v1/preferences', (route) => {
    if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON();
      pinned = body.pinnedResources;
      return route.fulfill({ json: body });
    }
    return route.fulfill({
      json: {
        exists: true,
        preferences: {
          schemaVersion: 1,
          theme: 'dark',
          density: 'comfortable',
          pinnedResources: [],
          landingPage: 'overview',
          chartRange: '24h',
        },
      },
    });
  });
  await page.goto('/settings/appearance');
  await page
    .getByRole('combobox', { name: 'Resource to pin' })
    .fill('postgres');
  await page.getByRole('option', { name: /postgres\.internal/ }).click();
  await page.getByRole('button', { name: 'Pin', exact: true }).click();
  await expect.poll(() => pinned).toEqual(['infra1']);
});

test('gated features explain themselves', async ({ page }) => {
  await mockApp(page);
  await page.goto('/settings/integrations');
  await expect(page.getByText('Disabled at deployment').first()).toBeVisible();
  await expect(page.getByRole('button', { name: 'New token' })).toHaveCount(0);
});

test('enrolls two-factor authentication when advanced auth is enabled', async ({
  page,
}) => {
  await mockApp(page, { advancedAuth: true });
  await page.route('**/api/v1/auth/mfa', (route) =>
    route.fulfill({ json: { enabled: false } }),
  );
  await page.route('**/api/v1/auth/mfa/enroll', (route) =>
    route.fulfill({
      json: {
        seed: 'JBSWY3DPEHPK3PXP',
        uri: 'otpauth://totp/Binnacle:admin?secret=JBSWY3DPEHPK3PXP',
        expiresAt: '2026-07-11T12:10:00Z',
      },
    }),
  );
  await page.route('**/api/v1/auth/mfa/confirm', (route) =>
    route.fulfill({ json: { recoveryCodes: ['aaaa-bbbb', 'cccc-dddd'] } }),
  );
  await page.goto('/settings/access');
  await page.getByRole('button', { name: 'Set up two-factor' }).click();
  const dialog = page.getByRole('dialog', {
    name: 'Set up two-factor authentication',
  });
  await dialog.getByLabel('Password').fill('correct horse battery staple');
  await dialog.getByRole('button', { name: 'Continue' }).click();
  await expect(
    dialog.getByText('JBSWY3DPEHPK3PXP', { exact: true }),
  ).toBeVisible();
  await dialog.getByLabel('Code from your app').fill('123456');
  await dialog.getByRole('button', { name: 'Enable' }).click();
  await expect(page.getByText('aaaa-bbbb')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Copy all' })).toBeVisible();
});
