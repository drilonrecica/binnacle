import { expect, test } from '@playwright/test';
import { mockApp, snapshot } from './fixtures';

test('renders the application shell with the primary navigation', async ({
  page,
}) => {
  await mockApp(page);
  await page.goto('/overview');

  await expect(page).toHaveTitle('Binnacle — Overview');
  await expect(
    page.getByRole('heading', { name: 'Overview', level: 1 }),
  ).toBeVisible();
  const nav = page.getByRole('navigation', { name: 'Primary navigation' });
  for (const label of ['Overview', 'Resources', 'Alerts', 'Activity']) {
    await expect(nav.getByRole('link', { name: label })).toBeVisible();
  }
  await expect(
    page.getByRole('heading', { name: 'Host headroom' }),
  ).toBeAttached();
  await expect(page.getByText('api.production')).toBeVisible();
});

test('legacy and unknown routes redirect to their replacements', async ({
  page,
}) => {
  await mockApp(page);
  await page.goto('/watch');
  await expect(page).toHaveURL(/\/overview$/);
  await page.goto('/server');
  await expect(page).toHaveURL(/\/host$/);
  await page.goto('/events');
  await expect(page).toHaveURL(/\/activity$/);
  await page.goto('/watch?inspect=res1');
  await expect(page).toHaveURL(/\/resources\/res1$/);
  await page.goto('/not-a-real-view');
  await expect(page).toHaveURL(/\/overview$/);
});

test('resources list filters, sorts, and opens detail with containers', async ({
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
  await page.getByRole('tab', { name: /Containers/ }).click();
  await expect(page.getByRole('table', { name: 'Containers' })).toContainText(
    'worker-1',
  );
  await expect(page.getByRole('table', { name: 'Containers' })).toContainText(
    /starting/i,
  );
});

test('creates a health check from the Alerts checks tab', async ({ page }) => {
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
  await page.goto('/alerts?tab=checks');
  await expect(
    page.getByRole('heading', { name: 'Alerts', level: 1 }),
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

test('shows incidents and manages notification channels and deliveries', async ({
  page,
}) => {
  await mockApp(page);
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

  await page.goto('/alerts');
  await expect(page.getByRole('tab', { name: /Incidents/ })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await page.getByRole('link', { name: 'Incident on api.production' }).click();
  await expect(page).toHaveURL(/\/alerts\/incidents\/inc-1$/);
  await expect(page.getByText('API is unhealthy')).toBeVisible();
  await expect(page.getByText('Existing webhook')).toBeVisible();

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
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await expect.poll(() => retried).toBe(true);
});

test('host charts switch ranges and surface data gaps', async ({ page }) => {
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
  await page.goto('/host');
  await expect(
    page.getByRole('heading', { name: 'Host', level: 1 }),
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

test('gated features explain themselves and the mobile shell uses a tab bar', async ({
  page,
}, testInfo) => {
  await mockApp(page);
  await page.goto('/settings/integrations');
  await expect(page.getByText('Disabled at deployment').first()).toBeVisible();
  await expect(page.getByRole('button', { name: 'New token' })).toHaveCount(0);

  if (!testInfo.project.name.includes('mobile')) {
    // Touch-device projects already run at phone size; resizing an emulated
    // device after load makes Chromium's hit-testing disagree with layout.
    await page.setViewportSize({ width: 390, height: 844 });
  }
  await page.goto('/overview');
  const nav = page.getByRole('navigation', { name: 'Primary navigation' });
  await expect(nav.getByRole('button', { name: 'More' })).toBeVisible();
  await nav.getByRole('button', { name: 'More' }).click();
  await expect(page.getByRole('dialog', { name: 'More' })).toBeVisible();
  await expect(
    page
      .getByRole('dialog', { name: 'More' })
      .getByRole('link', { name: 'Settings' }),
  ).toBeVisible();
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

test('the command palette jumps to a resource', async ({ page }) => {
  await mockApp(page);
  await page.goto('/overview');
  await page.keyboard.press('Control+k');
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  await expect(palette).toBeVisible();
  await palette.getByRole('combobox').fill('postgres');
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/\/resources\/infra1$/);
  await expect(
    page.getByRole('heading', { name: snapshot.resources[2].name, level: 1 }),
  ).toBeVisible();
});
