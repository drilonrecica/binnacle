import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'admin', username: 'admin' },
  expiresAt: '2026-07-11T13:00:00Z',
  absoluteExpiresAt: '2026-07-11T14:00:00Z',
};

const healthySnapshot = {
  seq: 1,
  ts: '2026-07-11T12:00:00Z',
  bootIdentity: 'boot-demo',
  host: {
    cpuPct: 24,
    memoryUsedBytes: 3_435_973_837,
    memoryTotalBytes: 8_589_934_592,
    diskUsedBytes: 45_097_156_608,
    diskTotalBytes: 107_374_182_400,
    load1: 0.42,
    uptimeSeconds: 1_641_600,
    networkRxBps: 12_582_912,
    networkTxBps: 4_194_304,
  },
  resources: [
    {
      id: 'api-production',
      name: 'api.production',
      status: 'healthy',
      cpuHostPct: 8.2,
      memoryBytes: 440_401_920,
      lastSeenAt: '2026-07-11T12:00:00Z',
      project: 'binnacle',
      environment: 'production',
      components: [
        { id: 'api-1', name: 'api-1', status: 'healthy' },
        { id: 'api-2', name: 'api-2', status: 'healthy' },
      ],
    },
    {
      id: 'worker-production',
      name: 'worker.production',
      status: 'healthy',
      cpuHostPct: 11.7,
      memoryBytes: 289_406_976,
      lastSeenAt: '2026-07-11T12:00:00Z',
      project: 'binnacle',
      environment: 'production',
      components: [{ id: 'worker-1', name: 'worker-1', status: 'healthy' }],
    },
    {
      id: 'postgres',
      name: 'postgres.internal',
      status: 'healthy',
      cpuHostPct: 3.1,
      memoryBytes: 713_031_680,
      lastSeenAt: '2026-07-11T12:00:00Z',
      category: 'infrastructure',
      infrastructure: true,
    },
  ],
  collectors: {
    host: { state: 'healthy', freshAt: '2026-07-11T12:00:00Z' },
    docker: { state: 'healthy', freshAt: '2026-07-11T12:00:00Z' },
  },
};

async function prepare(
  page: Page,
  options: { theme?: 'dark' | 'light'; degraded?: boolean } = {},
) {
  const snapshot = structuredClone(healthySnapshot);
  if (options.degraded) {
    snapshot.resources[1].status = 'degraded';
    snapshot.resources[1].cpuHostPct = 72.4;
    snapshot.collectors.docker = {
      state: 'degraded',
      freshAt: '2026-07-11T12:00:00Z',
      reason: 'Docker API responses are delayed',
    } as (typeof snapshot.collectors)['docker'];
  }
  await page.addInitScript(
    ({ value, theme }) => {
      localStorage.setItem('binnacle.theme', theme);
      class DemoEventSource extends EventTarget {
        onerror: ((event: Event) => void) | null = null;
        constructor() {
          super();
          window.setTimeout(() => {
            this.dispatchEvent(
              new MessageEvent('snapshot', { data: JSON.stringify(value) }),
            );
          });
        }
        close() {}
      }
      Object.defineProperty(window, 'EventSource', { value: DemoEventSource });
    },
    { value: snapshot, theme: options.theme ?? 'dark' },
  );
  await page.route('**/api/v1/auth/session', (route) =>
    route.fulfill({ json: session }),
  );
  await page.route('**/api/v1/onboarding', (route) =>
    route.fulfill({
      json: { checklistDismissed: true, completedAt: '2026-07-11T11:00:00Z' },
    }),
  );
}

test('dark watch console visual baseline', async ({ page }) => {
  await prepare(page);
  await page.goto('/watch');
  await expect(page.getByRole('heading', { name: 'Watch' })).toBeVisible();
  await expect(page).toHaveScreenshot('watch-dark.png', {
    animations: 'disabled',
    caret: 'hide',
  });
});

test('light watch console visual baseline', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'chromium');
  await prepare(page, { theme: 'light' });
  await page.goto('/watch');
  await expect(page.getByRole('heading', { name: 'Watch' })).toBeVisible();
  await expect(page).toHaveScreenshot('watch-light.png', {
    animations: 'disabled',
    caret: 'hide',
  });
});

test('degraded watch console visual baseline', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'chromium');
  await prepare(page, { degraded: true });
  await page.goto('/watch');
  await expect(
    page.getByText('Docker API responses are delayed'),
  ).toBeVisible();
  await expect(page).toHaveScreenshot('watch-degraded.png', {
    animations: 'disabled',
    caret: 'hide',
  });
});
