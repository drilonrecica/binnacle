import type { Page } from '@playwright/test';

export const session = {
  user: { id: 'admin', username: 'admin' },
  expiresAt: '2026-07-11T13:00:00Z',
  absoluteExpiresAt: '2026-07-11T14:00:00Z',
};

export const snapshot = {
  seq: 1,
  ts: '2026-07-11T12:00:00Z',
  bootIdentity: 'boot-demo',
  host: {
    cpuPct: 24,
    cpuUserPct: 17,
    cpuSystemPct: 5,
    memoryUsedBytes: 3_435_973_837,
    memoryTotalBytes: 8_589_934_592,
    memoryAvailableBytes: 5_153_960_755,
    memoryPct: 40,
    swapUsedBytes: 188_743_680,
    swapTotalBytes: 2_147_483_648,
    diskUsedBytes: 45_097_156_608,
    diskTotalBytes: 107_374_182_400,
    load1: 0.42,
    load5: 0.38,
    load15: 0.35,
    uptimeSeconds: 1_641_600,
    networkRxBps: 12_582_912,
    networkTxBps: 4_194_304,
    diskReadBps: 1_048_576,
    diskWriteBps: 2_097_152,
  },
  resources: [
    {
      id: 'res1',
      name: 'api.production',
      status: 'healthy',
      cpuHostPct: 8.2,
      memoryBytes: 440_401_920,
      rxBps: 120_000,
      txBps: 80_000,
      lastSeenAt: '2026-07-11T12:00:00Z',
      category: 'application',
      project: 'binnacle',
      environment: 'production',
      components: [
        {
          id: 'api-1-000000000000',
          name: 'api-1',
          status: 'healthy',
          runtimeState: 'running',
          healthStatus: 'healthy',
        },
        {
          id: 'api-2-000000000000',
          name: 'api-2',
          status: 'healthy',
          runtimeState: 'running',
          healthStatus: 'healthy',
        },
      ],
    },
    {
      id: 'res_beta',
      name: 'worker.production',
      status: 'unknown',
      cpuHostPct: 20,
      memoryBytes: 289_406_976,
      lastSeenAt: '2026-07-11T12:00:00Z',
      category: 'worker',
      project: 'binnacle',
      environment: 'production',
      components: [
        {
          id: 'worker-1-00000000000',
          name: 'worker-1',
          status: 'unknown',
          runtimeState: 'running',
          healthStatus: 'starting',
        },
      ],
    },
    {
      id: 'infra1',
      name: 'postgres.internal',
      status: 'healthy',
      cpuHostPct: 3.1,
      memoryBytes: 713_031_680,
      lastSeenAt: '2026-07-11T12:00:00Z',
      category: 'database',
      infrastructure: true,
      components: [],
    },
  ],
  collectors: {
    host: { state: 'healthy', freshAt: '2026-07-11T12:00:00Z' },
    docker: { state: 'healthy', freshAt: '2026-07-11T12:00:00Z' },
  },
  filesystems: [
    {
      mountKey: 'root',
      mountPoint: '/',
      fsType: 'ext4',
      totalBytes: 107_374_182_400,
      usedBytes: 45_097_156_608,
      availableBytes: 62_277_025_792,
      usedPct: 42,
      inodesUsedPct: 12,
    },
  ],
};

export const authMethods = {
  mode: 'local',
  local: true,
  proxy: false,
  proxyAvailable: false,
  mfaAvailable: false,
};

export function settingsValues() {
  const live = (value: string) => ({
    value,
    source: 'Default',
    applyMode: 'live',
  });
  const restart = (value: string) => ({
    value,
    source: 'Default',
    applyMode: 'restart_required',
  });
  return {
    'collection.host_interval': live('10s'),
    'collection.container_interval': live('10s'),
    'persistence.raw_interval': live('10s'),
    'charts.max_points_per_series': live('1000'),
    'retention.preset': live('balanced'),
    'retention.raw': live('48h'),
    'retention.one_minute': live('720h'),
    'retention.fifteen_minute': live('8760h'),
    'retention.one_hour': live('0s'),
    'database.target_budget_bytes': live('1073741824'),
    'sessions.idle_timeout': live('12h'),
    'sessions.absolute_lifetime': live('720h'),
    'http.listen_address': restart(':8080'),
    'docker.socket_path': restart('/var/run/docker.sock'),
    'paths.host_proc': restart('/host/proc'),
    'paths.host_sys': restart('/host/sys'),
    'paths.data_dir': restart('/var/lib/binnacle'),
  };
}

export const preferences = {
  schemaVersion: 1,
  theme: 'dark',
  density: 'comfortable',
  pinnedResources: [] as string[],
  landingPage: 'overview',
  chartRange: '24h',
  updatedAt: '2026-07-11T12:00:00Z',
};

export const metricsResponse = {
  scope: 'host',
  from: '2026-07-11T11:00:00Z',
  to: '2026-07-11T12:00:00Z',
  resolution: '10s',
  series: [
    {
      metric: 'cpu',
      unit: 'percent',
      points: [
        { at: '2026-07-11T11:00:00Z', min: 1, avg: 2, max: 3, count: 1 },
        { at: '2026-07-11T11:30:00Z', min: 4, avg: 5, max: 6, count: 1 },
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
};

export interface MockOptions {
  authenticated?: boolean;
  onboarded?: boolean;
  portability?: boolean;
  advancedAuth?: boolean;
  setupAvailable?: boolean;
  liveSnapshot?: typeof snapshot | null;
}

/**
 * Installs the baseline route mocks every authenticated page needs. Tests
 * add specific routes afterwards; Playwright matches the most recent route
 * first, so overrides win.
 */
export async function mockApp(page: Page, options: MockOptions = {}) {
  const {
    authenticated = true,
    onboarded = true,
    portability = false,
    advancedAuth = false,
    setupAvailable = false,
    liveSnapshot = snapshot,
  } = options;

  await page.route('**/api/v1/auth/session', (route) =>
    authenticated
      ? route.fulfill({ json: session })
      : route.fulfill({
          status: 401,
          json: { error: { code: 'unauthorized' } },
        }),
  );
  await page.route('**/api/v1/session', (route) =>
    route.fulfill({ status: authenticated ? 204 : 401 }),
  );
  await page.route('**/api/v1/setup', (route) =>
    route.fulfill({ json: { available: setupAvailable } }),
  );
  await page.route('**/api/v1/auth/methods', (route) =>
    route.fulfill({
      json: { ...authMethods, mfaAvailable: advancedAuth },
    }),
  );
  await page.route('**/api/v1/onboarding', (route) =>
    route.fulfill({
      json: {
        checklistDismissed: true,
        retentionPreset: 'balanced',
        completedAt: onboarded ? '2026-07-11T11:00:00Z' : undefined,
        diagnostics: onboarded
          ? [
              {
                id: 'docker_api',
                name: 'Docker API access',
                status: 'passed',
                required: true,
                reason: 'Docker responded.',
              },
            ]
          : [],
      },
    }),
  );
  await page.route('**/api/v1/live', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body: liveSnapshot
        ? `event: snapshot\nid: 1\ndata: ${JSON.stringify(liveSnapshot)}\n\n`
        : '',
    }),
  );
  await page.route('**/api/v1/preferences', (route) =>
    route.request().method() === 'PUT'
      ? route.fulfill({
          json: { ...preferences, ...route.request().postDataJSON() },
        })
      : route.fulfill({ json: { exists: true, preferences } }),
  );
  await page.route('**/api/v1/settings', (route) =>
    route.fulfill({
      json: {
        revision: 1,
        values: settingsValues(),
        features: { advancedAuth, portability },
      },
    }),
  );
  await page.route('**/api/v1/metrics/sparklines?*', (route) =>
    route.fulfill({
      json: {
        from: '2026-07-11T11:00:00Z',
        to: '2026-07-11T12:00:00Z',
        stepSeconds: 60,
        resources: {
          res1: { cpu: [5, 6, 7, 8], memory: [1, 1, 2, 2] },
        },
      },
    }),
  );
  await page.route('**/api/v1/metrics?*', (route) =>
    route.fulfill({ json: metricsResponse }),
  );
  await page.route('**/api/v1/events?*', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/incidents?*', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/incidents', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/alerts?*', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/alert-rules', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/checks?*', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/checks', (route) => route.fulfill({ json: [] }));
  await page.route('**/api/v1/silences?*', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/silences', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/notification-channels', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/notification-deliveries?*', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/notification-deliveries', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/resources', (route) =>
    route.fulfill({ json: liveSnapshot?.resources ?? [] }),
  );
  await page.route('**/api/v1/resources?*', (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route('**/api/v1/monitor-health', (route) =>
    route.fulfill({
      json: {
        at: '2026-07-11T12:00:00Z',
        metrics: [
          {
            id: 'database',
            label: 'SQLite database',
            value: 42_000_000,
            unit: 'bytes',
            status: 'normal',
            help: 'Main SQLite file size.',
          },
        ],
      },
    }),
  );
  await page.route('**/api/v1/integrations/coolify', (route) =>
    route.fulfill({
      json: {
        enabled: false,
        tokenConfigured: false,
        environmentAuthoritative: false,
        collector: { state: 'unknown', resources: 0 },
      },
    }),
  );
  await page.route('**/api/v1/api-tokens', (route) =>
    route.fulfill({
      json: {
        tokens: [],
        scopes: [
          'server:read',
          'resources:read',
          'metrics:read',
          'events:read',
          'incidents:read',
        ],
      },
    }),
  );
  await page.route('**/api/v1/processes?*', (route) =>
    route.fulfill({ json: { processes: [], sampled: true } }),
  );
  await page.route('**/api/v1/logs?*', (route) =>
    route.fulfill({
      json: { entries: [], truncated: false, redaction: 'best-effort' },
    }),
  );
}
