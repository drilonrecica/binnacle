import { expect, test, type Page } from '@playwright/test';
import {
  fixtureNow,
  mockApp,
  mockPopulated,
  snapshot,
  themes,
  type MockOptions,
  type Theme,
} from './fixtures';

// Visual baselines for every screen in both themes. The two projects in the
// Playwright configs give each one a desktop (1280 px) and a phone (Pixel 7,
// 412 px) width, so a screen here is four baselines.
//
// Update after an intended change with
//   pnpm exec playwright test visual.spec.ts --update-snapshots
// and review every changed image before committing it.

// Motion and time are frozen here only; the functional suites run with the
// browser's defaults so they would catch motion that hides content (UI-12).
test.use({
  contextOptions: { reducedMotion: 'reduce' },
  locale: 'en-US',
  timezoneId: 'UTC',
});

const screenshotOptions = {
  animations: 'disabled' as const,
  caret: 'hide' as const,
  fullPage: true,
  // Bound Fedora/Ubuntu font rendering drift without masking layout changes.
  maxDiffPixelRatio: 0.005,
};

interface Screen {
  name: string;
  path: string;
  mock?: MockOptions;
  /** Waits for the content that makes the screen what it is. */
  ready: (page: Page) => Promise<void>;
  /** Extra routes installed after the populated data. */
  setup?: (page: Page) => Promise<void>;
}

const heading = (name: string) => async (page: Page) => {
  await expect(page.getByRole('heading', { name, level: 1 })).toBeVisible();
};

const settings = (section: string, card: string): Screen => ({
  name: `settings-${section}`,
  path: `/settings/${section}`,
  ready: async (page) => {
    await heading('Settings')(page);
    await expect(
      page.getByRole('heading', { name: card, exact: true }).first(),
    ).toBeVisible();
  },
});

const screens: Screen[] = [
  {
    name: 'login',
    path: '/login',
    mock: { authenticated: false },
    ready: async (page) => {
      await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible();
    },
  },
  {
    name: 'setup',
    path: '/setup',
    mock: { authenticated: false, setupAvailable: true },
    ready: async (page) => {
      await expect(
        page.getByRole('heading', { name: 'Set up Binnacle' }),
      ).toBeVisible();
    },
  },
  {
    name: 'onboarding',
    path: '/onboarding',
    mock: { onboarded: false },
    ready: async (page) => {
      await expect(
        page.getByRole('heading', { name: 'Before you start' }),
      ).toBeVisible();
    },
  },
  {
    name: 'headroom',
    path: '/headroom',
    ready: async (page) => {
      await heading('Headroom')(page);
      await expect(
        page.getByRole('region', { name: 'Needs attention' }),
      ).toContainText('worker.production');
      await expect(page.getByText('api.production deployed')).toBeVisible();
    },
  },
  {
    name: 'headroom-degraded',
    path: '/headroom',
    mock: { liveSnapshot: degraded() },
    ready: async (page) => {
      await heading('Headroom')(page);
      await expect(
        page.getByText('Docker API responses are delayed').first(),
      ).toBeVisible();
    },
  },
  {
    name: 'resources',
    path: '/resources',
    ready: async (page) => {
      await heading('Resources')(page);
      await expect(page.getByText('binnacle / production')).toBeVisible();
    },
  },
  {
    name: 'resource-detail',
    path: '/resources/res1',
    ready: async (page) => {
      await heading('api.production')(page);
      await expect(page.getByRole('tabpanel')).toBeVisible();
    },
  },
  {
    name: 'resource-logs',
    path: '/resources/res1?tab=logs',
    ready: async (page) => {
      await expect(
        page.getByRole('region', { name: 'Log lines' }),
      ).toContainText('connection refused');
    },
  },
  {
    name: 'server',
    path: '/server',
    ready: async (page) => {
      await heading('Server')(page);
      await expect(
        page.getByRole('heading', { name: 'Load average' }),
      ).toBeVisible();
      await expect(page.getByText('1 gap').first()).toBeVisible();
    },
  },
  {
    name: 'incidents',
    path: '/incidents',
    ready: async (page) => {
      await heading('Incidents')(page);
      await expect(
        page.getByRole('link', { name: /Resource incident on worker/ }),
      ).toBeVisible();
    },
  },
  {
    name: 'incident-detail',
    path: '/incidents/inc-1',
    ready: async (page) => {
      await expect(page.getByText(/restarted 4 times/).first()).toBeVisible();
      await expect(page.getByText('Operations webhook').first()).toBeVisible();
    },
  },
  {
    name: 'changes',
    path: '/changes',
    ready: async (page) => {
      await heading('Changes')(page);
      await expect(
        page.getByText('worker.production was OOM killed'),
      ).toBeVisible();
    },
  },
  settings('general', 'Collection'),
  settings('data', 'Retention'),
  settings('access', 'Sessions'),
  settings('notifications', 'Channels'),
  settings('alerting', 'Alert rules'),
  settings('appearance', 'Theme'),
  settings('integrations', 'Coolify'),
  settings('system', 'Background workers'),
];

/** A server under pressure: hot gauges, a degraded resource and collector. */
function degraded() {
  const value = structuredClone(snapshot);
  value.host.cpuPct = 81;
  value.host.memoryUsedBytes = 7_838_318_592;
  value.host.memoryPct = 91;
  value.host.diskUsedBytes = 94_489_280_512;
  value.filesystems[0]!.usedBytes = 94_489_280_512;
  value.filesystems[0]!.usedPct = 88;
  value.host.load1 = 3.86;
  value.resources[1]!.status = 'degraded';
  value.resources[1]!.cpuHostPct = 72.4;
  value.collectors.docker = {
    state: 'degraded',
    freshAt: '2026-07-11T12:00:00Z',
    reason: 'Docker API responses are delayed',
  } as (typeof value.collectors)['docker'];
  return value;
}

async function show(page: Page, screen: Screen, theme: Theme) {
  // Relative times ("4m ago") and the stale check read this clock.
  await page.clock.setFixedTime(fixtureNow);
  await mockApp(page, { ...screen.mock, theme });
  await mockPopulated(page);
  await screen.setup?.(page);
  await page.goto(screen.path);
  await screen.ready(page);
  await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
  // Webfonts finish loading before the first comparison.
  await page.evaluate(() => document.fonts.ready.then(() => undefined));
}

for (const theme of themes) {
  test.describe(`${theme} theme`, () => {
    for (const screen of screens) {
      test(screen.name, async ({ page }) => {
        await show(page, screen, theme);
        await expect(page).toHaveScreenshot(
          `${screen.name}-${theme}.png`,
          screenshotOptions,
        );
      });
    }
  });
}
