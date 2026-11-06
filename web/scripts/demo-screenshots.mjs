// SPDX-License-Identifier: AGPL-3.0-only
// Captures the README, docs and landing-page screenshots from the real demo
// (`binnacle --demo`): a fresh temporary database, deterministic synthetic
// containers and seeded history, so the gauges show trends and time-to-full
// the way an installed server does after a day.
//
//   make build && pnpm --dir web screenshots
//
// Writes docs/assets/screenshots/*.png and copies the landing page's subset
// to landing/assets/screens/. The demo is live, so values and relative times
// differ between runs: review the images before committing them.
import { spawn } from 'node:child_process';
import { copyFile, mkdir, mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { randomBytes } from 'node:crypto';
import { chromium } from '@playwright/test';

const root = fileURLToPath(new URL('../../', import.meta.url));
const binary = process.env.BINNACLE_BIN ?? join(root, 'bin/binnacle');
const docsDir = join(root, 'docs/assets/screenshots');
const landingDir = join(root, 'landing/assets/screens');
const address = '127.0.0.1:8098';
const base = `http://${address}`;
const token = randomBytes(32).toString('hex');
const password = 'demo screenshots passphrase';
const viewport = { width: 1440, height: 900 };
const settle = 3000;

/** @typedef {{ name: string, theme: 'dark' | 'light', path: string, open?: (page: import('@playwright/test').Page) => Promise<void>, landing?: boolean }} Shot */

/** @type {Shot[]} */
const shots = [
  {
    name: 'resource-detail-dark',
    theme: 'dark',
    path: '/resources',
    open: async (page) => {
      await page
        .getByRole('link', { name: 'api-shop', exact: true })
        .first()
        .click();
      await page.getByRole('tabpanel').waitFor();
    },
    landing: true,
  },
  {
    name: 'incident-dark',
    theme: 'dark',
    path: '/incidents',
    open: async (page) => {
      // The demo opens its critical incident a minute or so after start.
      await page
        .getByRole('link', { name: /Resource incident on/ })
        .first()
        .click({ timeout: 120_000 });
      await page.waitForURL(/\/incidents\/.+/);
    },
    landing: true,
  },
  { name: 'changes-dark', theme: 'dark', path: '/changes', landing: true },
  // Last: the estimator needs a few minutes of live samples on top of the
  // seeded history before it states a time-to-full.
  {
    name: 'headroom-dark',
    theme: 'dark',
    path: '/headroom',
    open: async (page) => {
      await page
        .getByText(/full in ~/)
        .first()
        .waitFor({ timeout: 360_000 });
    },
    landing: true,
  },
  { name: 'headroom-light', theme: 'light', path: '/headroom' },
];

async function waitForServer() {
  for (let attempt = 0; attempt < 120; attempt++) {
    try {
      if ((await fetch(base + '/')).ok) return;
    } catch {
      // Not listening yet.
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`the demo did not answer on ${base} within 60 s`);
}

/** Claims the instance, walks onboarding and leaves an authenticated context. */
async function signIn(browser) {
  const context = await browser.newContext({ viewport });
  const claim = await context.request.post(base + '/api/v1/setup/claim', {
    headers: { Origin: base },
    data: { token, username: 'admin', password },
  });
  if (claim.status() !== 204)
    throw new Error(`setup claim answered ${claim.status()}`);
  const page = await context.newPage();
  await page.goto(base + '/onboarding');
  const next = page.getByRole('button', {
    name: /^(Continue|Save and run checks|Open the dashboard)$/,
  });
  for (let step = 0; step < 8 && page.url().includes('/onboarding'); step++) {
    await next.last().click();
    await page.waitForTimeout(1500);
  }
  if (page.url().includes('/onboarding'))
    throw new Error('onboarding did not finish');
  await page.close();
  return context;
}

async function setTheme(page, theme) {
  const status = await page.evaluate(async (theme) => {
    const csrf = document.cookie
      .split('; ')
      .find((part) => part.startsWith('binnacle_csrf='))
      ?.split('=')[1];
    const current = await (await fetch('/api/v1/preferences')).json();
    const response = await fetch('/api/v1/preferences', {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': csrf ?? '',
      },
      body: JSON.stringify({ ...current.preferences, theme }),
    });
    return response.status;
  }, theme);
  if (status !== 200) throw new Error(`preferences answered ${status}`);
}

const data = await mkdtemp(join(tmpdir(), 'binnacle-screenshots-'));
const server = spawn(binary, ['--demo'], {
  env: {
    ...process.env,
    BINNACLE_SETUP_TOKEN: token,
    BINNACLE_LISTEN_ADDRESS: address,
    BINNACLE_DATA_DIR: data,
  },
  stdio: 'ignore',
});
const browser = await chromium.launch();
try {
  await waitForServer();
  const context = await signIn(browser);
  await mkdir(docsDir, { recursive: true });
  await mkdir(landingDir, { recursive: true });
  const page = await context.newPage();
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto(base + '/headroom');
  for (const shot of shots) {
    await setTheme(page, shot.theme);
    await page.goto(base + shot.path);
    await shot.open?.(page);
    await page.waitForTimeout(settle);
    await page.evaluate(() => document.fonts.ready.then(() => undefined));
    const file = join(docsDir, `${shot.name}.png`);
    await page.screenshot({
      path: file,
      animations: 'disabled',
      caret: 'hide',
    });
    if (shot.landing)
      await copyFile(file, join(landingDir, `${shot.name}.png`));
    console.log(`captured ${shot.name}`);
  }
} finally {
  await browser.close();
  server.kill('SIGTERM');
  await new Promise((resolve) => server.once('exit', resolve));
  await rm(data, { recursive: true, force: true });
}
