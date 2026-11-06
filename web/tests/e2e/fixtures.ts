import AxeBuilder from '@axe-core/playwright';
import { expect, type Page, type Route } from '@playwright/test';

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
  /** The saved theme; every page and dialog is checked in both. */
  theme?: Theme;
}

export type Theme = 'dark' | 'light';
export const themes: Theme[] = ['dark', 'light'];

/**
 * Fails on any WCAG 2.1/2.2 A/AA violation axe can detect on the page. The
 * suites run with motion on, so it first waits for dialogs and menus to
 * finish fading in: axe would otherwise measure contrast on a frame halfway
 * through the transition. Endless animations (the live pulse) are ignored.
 */
export async function scan(page: Page) {
  await page.waitForFunction(() =>
    document
      .getAnimations()
      .every(
        (animation) =>
          animation.playState !== 'running' ||
          animation.effect?.getComputedTiming().endTime === Infinity,
      ),
  );
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
    .analyze();
  expect(results.violations).toEqual([]);
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
    theme,
  } = options;
  const saved = { ...preferences, theme: theme ?? preferences.theme };

  await page.addInitScript(holdLiveStreamOpen);
  // A chosen theme is also stored locally: the inline theme script reads it
  // before first paint and the server copy below agrees, so the page never
  // switches theme after loading. Only on the first load, so a test that
  // changes the theme keeps its choice across navigations.
  if (theme)
    await page.addInitScript((value) => {
      if (!localStorage.getItem('binnacle.preferences.v1'))
        localStorage.setItem('binnacle.preferences.v1', JSON.stringify(value));
    }, saved);

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
  // `*` also matches the `?lastEventId=` a reopened stream resumes with.
  await page.route('**/api/v1/live*', (route) =>
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
          json: { ...saved, ...route.request().postDataJSON() },
        })
      : route.fulfill({ json: { exists: true, preferences: saved } }),
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
        failures: [
          {
            id: 'persistence_write',
            label: 'Persistence writes',
            count: 0,
            status: 'normal',
            help: 'History batches whose SQLite write failed after its retries.',
          },
          {
            id: 'docker',
            label: 'Docker API calls',
            count: 2,
            status: 'warning',
            help: 'Docker Engine calls that failed or timed out.',
          },
        ],
        latency: [
          {
            id: 'persistence_write',
            label: 'Persistence write',
            count: 120,
            p50: 3.2,
            p95: 9.8,
            max: 41.5,
            status: 'normal',
            help: 'One history batch written to SQLite, retries included.',
          },
          {
            id: 'vacuum',
            label: 'Incremental vacuum',
            count: 0,
            p50: null,
            p95: null,
            max: null,
            status: 'unavailable',
            help: 'The incremental vacuum after a retention pass.',
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

/** The fixtures' "now": silences, incidents and checks are current at it. */
export const fixtureNow = new Date('2026-07-11T12:00:30Z');

export const openIncident = {
  id: 'inc-1',
  status: 'open',
  severity: 'critical',
  title: 'resource incident on res_beta',
  targetType: 'resource',
  targetId: 'res_beta',
  alertCount: 1,
  firingAlertCount: 1,
  openedAt: '2026-07-11T11:50:00Z',
};

const firingAlert = {
  id: 'alert-1',
  ruleId: 'builtin-restart-storm',
  family: 'restart_storm',
  severity: 'critical',
  status: 'firing',
  message: 'worker.production restarted 4 times in 10 minutes',
  startedAt: '2026-07-11T11:50:00Z',
  targetType: 'resource',
  targetId: 'res_beta',
  incidentId: 'inc-1',
};

const rule = {
  id: 'builtin-host-cpu-warning',
  family: 'host_cpu_warning',
  name: 'Host CPU warning',
  builtIn: true,
  enabled: true,
  severity: 'warning',
  scopeType: 'global',
  threshold: 90,
  recoveryThreshold: 80,
  triggerSeconds: 300,
  recoverySeconds: 120,
  suppressDuringDeployment: false,
};

const check = {
  id: 'check-1',
  resourceId: 'res1',
  name: 'Public health',
  url: 'https://api.example.com/health',
  method: 'GET',
  interval: 30e9,
  timeout: 5e9,
  expectedStatusMin: 200,
  expectedStatusMax: 299,
  required: true,
  enabled: true,
  createdAt: '2026-07-10T12:00:00Z',
  updatedAt: '2026-07-10T12:00:00Z',
  state: {
    checkId: 'check-1',
    status: 'success',
    httpStatus: 200,
    latencyMs: 84,
    checkedAt: '2026-07-11T11:59:45Z',
    consecutiveSuccesses: 12,
    consecutiveFailures: 0,
  },
};

export const historicalEvents = [
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

/**
 * Fills every list a page can show (incidents, alerts, rules, checks,
 * silences, events, channels, deliveries, tokens), so accessibility scans
 * and screenshots see populated tables and every row action has a target.
 * Install it after `mockApp`; its routes win.
 */
export async function mockPopulated(page: Page) {
  const json = (value: unknown) => (route: Route) =>
    route.fulfill({ json: value });
  await page.route('**/api/v1/incidents?*', (route) =>
    route.fulfill({
      json:
        new URL(route.request().url()).searchParams.get('status') === 'resolved'
          ? [
              {
                ...openIncident,
                id: 'inc-0',
                status: 'resolved',
                severity: 'warning',
                targetId: 'res1',
                title: 'resource incident on res1',
                openedAt: '2026-07-11T08:00:00Z',
                resolvedAt: '2026-07-11T08:20:00Z',
              },
            ]
          : [openIncident],
    }),
  );
  await page.route('**/api/v1/incidents', json([openIncident]));
  await page.route(
    '**/api/v1/incidents/inc-1',
    json({
      ...openIncident,
      alerts: [firingAlert],
      deliveries: [
        {
          id: 'delivery-1',
          channelId: 'channel-1',
          incidentId: 'inc-1',
          eventType: 'opened',
          status: 'succeeded',
          attemptCount: 1,
          updatedAt: '2026-07-11T11:50:05Z',
        },
      ],
    }),
  );
  await page.route('**/api/v1/alerts?*', json([firingAlert]));
  await page.route('**/api/v1/alert-rules', json([rule]));
  await page.route('**/api/v1/checks?*', json([check]));
  await page.route('**/api/v1/checks', json([check]));
  const silence = {
    id: 'sil-1',
    scopeType: 'resource',
    scopeId: 'res1',
    reason: 'Planned maintenance',
    startsAt: '2026-07-11T11:30:00Z',
    endsAt: '2026-07-11T13:30:00Z',
    createdBy: 'admin',
  };
  await page.route('**/api/v1/silences?*', json([silence]));
  await page.route('**/api/v1/silences', json([silence]));
  await page.route('**/api/v1/events?*', json(historicalEvents));
  await page.route(
    '**/api/v1/notification-channels',
    json([
      {
        id: 'channel-1',
        name: 'Operations webhook',
        kind: 'webhook',
        enabled: true,
        minimumSeverity: 'warning',
        notifyResolved: true,
        config: { url: 'https://hooks.example.com/ops' },
        secretConfigured: true,
      },
    ]),
  );
  await page.route(
    '**/api/v1/notification-deliveries?*',
    json([
      {
        id: 'delivery-2',
        channelId: 'channel-1',
        eventType: 'opened',
        status: 'permanent_failure',
        attemptCount: 7,
        failureCode: 'http_400',
        updatedAt: '2026-07-11T11:00:00Z',
      },
    ]),
  );
  await page.route(
    '**/api/v1/api-tokens',
    json({
      tokens: [
        {
          id: 'tok-1',
          name: 'Prometheus',
          prefix: 'bnk_abc',
          scopes: ['metrics:read'],
          createdAt: '2026-07-01T12:00:00Z',
          lastUsedAt: '2026-07-11T11:59:00Z',
        },
      ],
      scopes: ['server:read', 'resources:read', 'metrics:read'],
    }),
  );
  await page.route(
    '**/api/v1/processes?*',
    json({
      sampled: true,
      processes: [
        {
          pid: 4242,
          name: 'postgres',
          command: 'postgres -D [redacted]',
          cpuPct: 3.5,
          memoryBytes: 104_857_600,
        },
      ],
    }),
  );
  await page.route(
    '**/api/v1/logs?*',
    json({
      entries: [
        {
          timestamp: '2026-07-11T11:59:58Z',
          component: 'api-1',
          stream: 'stdout',
          severity: 'info',
          message: 'GET /health 200 3ms',
        },
        {
          timestamp: '2026-07-11T11:59:59Z',
          component: 'api-2',
          stream: 'stderr',
          severity: 'error',
          message: 'connection refused: postgres.internal:5432',
        },
      ],
      truncated: false,
      redaction: 'best-effort',
    }),
  );
}

/** What `holdLiveStreamOpen` exposes on `window` so specs can drive the stream. */
export interface LiveControl {
  /** Every URL a stream was opened with, oldest first. */
  opened: string[];
  /** Dispatches a frame on the open stream, as the server would. */
  emit(kind: string, data: unknown, id?: string): boolean;
  /** Ends the open stream the way a browser reports a dropped connection. */
  drop(): boolean;
}

declare global {
  interface Window {
    __binnacleLive?: LiveControl;
  }
}

/** The URLs the app has opened the live stream with so far. */
export function openedStreams(page: Page): Promise<string[]> {
  return page.evaluate(() => window.__binnacleLive?.opened ?? []);
}

/** Sends a frame on the open mocked stream; fails when none is open. */
export async function emitLive(
  page: Page,
  kind: 'snapshot' | 'event',
  data: unknown,
  id: string,
) {
  const sent = await page.evaluate(
    ({ kind, data, id }) => window.__binnacleLive?.emit(kind, data, id),
    { kind, data, id },
  );
  expect(sent, 'a live stream is open').toBe(true);
}

/** Drops the open mocked stream; fails when none is open. */
export async function dropLive(page: Page) {
  const dropped = await page.evaluate(() => window.__binnacleLive?.drop());
  expect(dropped, 'a live stream is open').toBe(true);
}

/**
 * Replaces EventSource with one that keeps the mocked live stream open (UI-2).
 * Playwright can only fulfil a route with a finished body, which a real
 * EventSource reads as a dropped stream and retries, so every test would run
 * with the connection flapping. This one fetches the routed body once,
 * dispatches its frames with their ids and then stays open until a spec
 * drops it through `window.__binnacleLive`; a non-stream answer (401, 503)
 * closes it as a browser would. A spec that installs its own EventSource
 * first keeps it.
 */
function holdLiveStreamOpen() {
  // A spec that installed its own stand-in first keeps it.
  if (!String(window.EventSource).includes('[native code]')) return;
  let current: HeldEventSource | null = null;
  const track = (source: HeldEventSource) => {
    current = source;
    control.opened.push(source.url);
  };
  const control = {
    opened: [] as string[],
    emit(kind: string, data: unknown, id = '') {
      if (current?.readyState !== 1) return false;
      current.dispatch(kind, JSON.stringify(data), id);
      return true;
    },
    drop() {
      if (current?.readyState !== 1) return false;
      current.fail();
      return true;
    },
  };
  class HeldEventSource extends EventTarget {
    static readonly CONNECTING = 0;
    static readonly OPEN = 1;
    static readonly CLOSED = 2;
    readyState = 0;
    onerror: ((event: Event) => void) | null = null;
    constructor(readonly url: string) {
      super();
      track(this);
      void fetch(url, { credentials: 'same-origin' }).then(
        async (response) => {
          if (this.readyState === 2) return;
          const type = response.headers.get('content-type') ?? '';
          if (!response.ok || !type.startsWith('text/event-stream')) {
            this.fail();
            return;
          }
          this.readyState = 1;
          this.dispatchEvent(new Event('open'));
          for (const block of (await response.text()).split('\n\n')) {
            let kind = 'message';
            let id = '';
            const data: string[] = [];
            for (const line of block.split('\n')) {
              if (line.startsWith('event: ')) kind = line.slice(7);
              else if (line.startsWith('id: ')) id = line.slice(4);
              else if (line.startsWith('data: ')) data.push(line.slice(6));
            }
            if (data.length) this.dispatch(kind, data.join('\n'), id);
          }
        },
        () => this.fail(),
      );
    }
    dispatch(kind: string, data: string, id: string) {
      if (this.readyState !== 1) return;
      this.dispatchEvent(new MessageEvent(kind, { data, lastEventId: id }));
    }
    close() {
      this.readyState = 2;
    }
    fail() {
      this.readyState = 2;
      const event = new Event('error');
      this.dispatchEvent(event);
      this.onerror?.(event);
    }
  }
  Object.defineProperty(window, 'EventSource', { value: HeldEventSource });
  Object.defineProperty(window, '__binnacleLive', { value: control });
}
