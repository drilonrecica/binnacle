import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineConfig, devices } from '@playwright/test';

// Acceptance flows run against the real binnacle binary, not the mocked Vite
// dev server: `make build` first, or point BINNACLE_ACCEPTANCE_BINARY at a
// release binary. Each suite gets its own server on an empty data directory
// with its own gate switched on, so one suite's state (an enrolled second
// factor, tokens, pins) never reaches the other. Within a suite the tests
// share one server and one administrator, so they run serially.
const here = fileURLToPath(new URL('.', import.meta.url));
const binary = resolve(
  process.env.BINNACLE_ACCEPTANCE_BINARY ?? join(here, '..', 'bin', 'binnacle'),
);

interface Suite {
  name: string;
  port: number;
  env: Record<string, string>;
}

const suites: Suite[] = [
  {
    name: 'advanced-auth',
    port: 4175,
    env: { BINNACLE_FEATURE_ADVANCED_AUTH: 'true' },
  },
  {
    name: 'portability',
    port: 4176,
    env: {
      BINNACLE_FEATURE_PORTABILITY: 'true',
      BINNACLE_PROMETHEUS_ENABLED: 'true',
    },
  },
];

export default defineConfig({
  testDir: './tests/acceptance',
  fullyParallel: false,
  workers: 1,
  forbidOnly: Boolean(process.env.CI),
  reporter: process.env.CI ? 'github' : 'list',
  use: { trace: 'retain-on-failure' },
  webServer: suites.map(({ name, port, env }) => {
    const dataDir = join(tmpdir(), `binnacle-acceptance-${name}`);
    return {
      command: `rm -rf '${dataDir}' && mkdir -p '${dataDir}' && exec '${binary}' --demo --demo-containers 3 --demo-checks 1`,
      url: `http://127.0.0.1:${port}/healthz`,
      reuseExistingServer: false,
      env: {
        BINNACLE_DATA_DIR: dataDir,
        BINNACLE_LISTEN_ADDRESS: `127.0.0.1:${port}`,
        BINNACLE_ADMIN_USERNAME: 'admin',
        BINNACLE_ADMIN_PASSWORD: 'acceptance correct horse battery',
        BINNACLE_MASTER_KEY:
          '4f1c2b8e9d7a6035f2e1c4b7a8d9e0f1123456789abcdef0fedcba9876543210',
        ...env,
      },
    };
  }),
  projects: suites.map(({ name, port }) => ({
    name,
    testMatch: `${name}.spec.ts`,
    use: { ...devices['Desktop Chrome'], baseURL: `http://127.0.0.1:${port}` },
  })),
});
