import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import { expect, test } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

test('field manual assets, links, and accessibility are valid', async ({
  page,
}) => {
  const responses: number[] = [];
  const requests: string[] = [];
  page.on('response', (response) => {
    if (response.url().startsWith('http://127.0.0.1:4174'))
      responses.push(response.status());
  });
  page.on('request', (request) => requests.push(request.url()));
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toContainText(
    'Know what your Docker server is doing before it runs out of room',
  );
  await expect(
    page.getByRole('heading', {
      name: 'One VPS. Too many containers. Not enough visibility.',
    }),
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { name: 'Coolify-aware, not Coolify-only.' }),
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { name: 'Not another observability stack.' }),
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { name: 'Pre-release status.' }),
  ).toBeVisible();
  await expect(
    page.getByRole('link', { name: 'View development install guide' }),
  ).toHaveAttribute(
    'href',
    'https://github.com/drilonrecica/binnacle/blob/master/docs/operations/install.md',
  );
  await expect(
    page.locator('img[src="assets/watch-console.png"]'),
  ).toBeVisible();
  await expect(
    page.getByText('No external telemetry', { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText('No v0.3 tag has been published yet.'),
  ).toBeVisible();
  await expect(page.locator('html')).toHaveCSS('color-scheme', 'dark');
  await expect(page.locator('script')).toHaveCount(0);
  expect(responses.every((status) => status < 400)).toBe(true);
  expect(
    requests.every((url) => new URL(url).origin === 'http://127.0.0.1:4174'),
  ).toBe(true);
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth <=
        document.documentElement.clientWidth,
    ),
  ).toBe(true);
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
    .analyze();
  expect(results.violations).toEqual([]);
});

test('landing Watch screenshot matches the tested app baseline', async () => {
  const digest = async (path: string) =>
    createHash('sha256')
      .update(await readFile(path))
      .digest('hex');
  expect(await digest('../landing/assets/watch-console.png')).toBe(
    await digest(
      'tests/e2e/watch-visual.spec.ts-snapshots/watch-degraded-chromium-linux.png',
    ),
  );
});

test('small mobile keeps navigation and install action usable', async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== 'mobile');
  await page.setViewportSize({ width: 320, height: 800 });
  await page.goto('/');
  await expect(
    page.getByRole('link', { name: 'Install', exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole('link', { name: 'View development install guide' }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth <=
        document.documentElement.clientWidth,
    ),
  ).toBe(true);
});

test('field manual visual baseline', async ({ page }, testInfo) => {
  await page.goto('/');
  await expect(page).toHaveScreenshot(`landing-${testInfo.project.name}.png`, {
    fullPage: true,
    animations: 'disabled',
    // Bound cross-host font anti-aliasing drift without masking layout changes.
    maxDiffPixelRatio: 0.002,
  });
});
