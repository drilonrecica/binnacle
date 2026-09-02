import { expect, test, type Page } from '@playwright/test';
import { mockApp, snapshot } from './fixtures';

const screenshotOptions = {
  animations: 'disabled' as const,
  caret: 'hide' as const,
  // Bound Fedora/Ubuntu font rendering drift without masking layout changes.
  maxDiffPixelRatio: 0.005,
};

async function prepare(
  page: Page,
  options: { theme?: 'dark' | 'light'; degraded?: boolean } = {},
) {
  const value = structuredClone(snapshot);
  const events: Array<{
    id: number;
    at: string;
    type: string;
    severity: string;
    message: string;
    resourceId?: string;
  }> = [];
  const incidents: unknown[] = [];
  if (options.degraded) {
    value.host.cpuPct = 81;
    value.host.memoryUsedBytes = 7_838_318_592;
    value.host.memoryPct = 91;
    value.host.diskUsedBytes = 94_489_280_512;
    value.filesystems[0].usedBytes = 94_489_280_512;
    value.filesystems[0].usedPct = 88;
    value.host.load1 = 3.86;
    value.resources[1].status = 'degraded';
    value.resources[1].cpuHostPct = 72.4;
    value.collectors.docker = {
      state: 'degraded',
      freshAt: '2026-07-11T12:00:00Z',
      reason: 'Docker API responses are delayed',
    } as (typeof value.collectors)['docker'];
    events.push(
      {
        id: 41,
        at: '2026-07-11T11:58:00Z',
        type: 'container_oom',
        severity: 'critical',
        message: 'worker.production was OOM killed',
        resourceId: 'res_beta',
      },
      {
        id: 42,
        at: '2026-07-11T11:59:00Z',
        type: 'container_restart',
        severity: 'warning',
        message: 'worker.production restarted',
        resourceId: 'res_beta',
      },
    );
    incidents.push({
      id: 'inc-1',
      status: 'open',
      severity: 'critical',
      title: 'resource incident on res_beta',
      targetType: 'resource',
      targetId: 'res_beta',
      alertCount: 1,
      firingAlertCount: 1,
      openedAt: '2026-07-11T11:50:00Z',
    });
  }
  await page.addInitScript(
    ({ value, theme, liveEvents }) => {
      localStorage.setItem(
        'binnacle.preferences.v1',
        JSON.stringify({
          schemaVersion: 1,
          theme,
          density: 'comfortable',
          pinnedResources: [],
          landingPage: 'overview',
          chartRange: '24h',
        }),
      );
      class DemoEventSource extends EventTarget {
        onerror: ((event: Event) => void) | null = null;
        constructor() {
          super();
          window.setTimeout(() => {
            this.dispatchEvent(
              new MessageEvent('snapshot', { data: JSON.stringify(value) }),
            );
            for (const liveEvent of liveEvents) {
              this.dispatchEvent(
                new MessageEvent('event', { data: JSON.stringify(liveEvent) }),
              );
            }
          });
        }
        close() {}
      }
      Object.defineProperty(window, 'EventSource', { value: DemoEventSource });
    },
    { value, theme: options.theme ?? 'dark', liveEvents: events },
  );
  await mockApp(page, { liveSnapshot: value });
  const preferences = {
    schemaVersion: 1,
    theme: options.theme ?? 'dark',
    density: 'comfortable',
    pinnedResources: [],
    landingPage: 'overview',
    chartRange: '24h',
  };
  await page.route('**/api/v1/preferences', (route) =>
    route.fulfill({ json: { exists: true, preferences } }),
  );
  await page.route('**/api/v1/incidents?*', (route) =>
    route.fulfill({ json: incidents }),
  );
  // Freeze relative times so "just now" does not drift between runs.
  await page.clock.setFixedTime(new Date('2026-07-11T12:00:30Z'));
}

test('dark overview visual baseline', async ({ page }) => {
  await prepare(page);
  await page.goto('/overview');
  await expect(page.getByText('api.production')).toBeVisible();
  await expect(page).toHaveScreenshot('overview-dark.png', screenshotOptions);
});

test('light overview visual baseline', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'chromium');
  await prepare(page, { theme: 'light' });
  await page.goto('/overview');
  await expect(page.getByText('api.production')).toBeVisible();
  await expect(page).toHaveScreenshot('overview-light.png', screenshotOptions);
});

test('degraded overview visual baseline', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'chromium');
  await prepare(page, { degraded: true });
  await page.goto('/overview');
  await expect(
    page.getByText('Docker API responses are delayed'),
  ).toBeVisible();
  await expect(
    page.getByText('worker.production was OOM killed'),
  ).toBeVisible();
  await expect(page).toHaveScreenshot(
    'overview-degraded.png',
    screenshotOptions,
  );
});
