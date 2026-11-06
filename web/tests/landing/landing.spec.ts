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
    'How much room your Docker server has left, and when it runs out',
  );
  for (const name of [
    'Four questions answered in one screen.',
    'Each service, incident and change gets a page.',
    'One binary and one SQLite file with a 1 GiB budget.',
    'Read-only toward Docker, permanently.',
    'Deploy v0.6.0.',
  ])
    await expect(page.getByRole('heading', { name })).toBeVisible();
  await expect(
    page.getByRole('link', { name: 'View install guide' }),
  ).toHaveAttribute(
    'href',
    'https://github.com/drilonrecica/binnacle/blob/master/docs/operations/install.md',
  );
  await expect(
    page.locator('img[src="assets/screens/headroom-dark.png"]'),
  ).toBeVisible();
  await expect(page.getByText('74 MiB', { exact: true })).toBeVisible();
  await expect(
    page.getByText('v0.6.0 is available for amd64 and arm64', { exact: false }),
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

// PRC-3: headlines state what Binnacle does; none is an "X, not Y" contrast.
test('headings make claims instead of contrasts', async ({ page }) => {
  await page.goto('/');
  const headings = await page
    .getByRole('heading')
    .evaluateAll((nodes) => nodes.map((node) => node.textContent ?? ''));
  expect(headings.length).toBeGreaterThan(5);
  for (const heading of headings)
    expect(heading).not.toMatch(/,\s*not\b|\bnot (just|another|only)\b/i);
});

// The screenshots come from `pnpm screenshots` (scripts/demo-screenshots.mjs)
// against the real demo. The landing page shows the same captures as the
// README and docs, and each one at the size its markup declares.
test('screenshots are the demo captures at their declared size', async ({
  page,
}) => {
  const digest = async (path: string) =>
    createHash('sha256')
      .update(await readFile(path))
      .digest('hex');
  await page.goto('/');
  const images = await page
    .locator('img[src^="assets/screens/"]')
    .evaluateAll((nodes) =>
      (nodes as HTMLImageElement[]).map((img) => ({
        src: img.getAttribute('src') ?? '',
        width: img.getAttribute('width'),
        height: img.getAttribute('height'),
        natural: [img.naturalWidth, img.naturalHeight],
        alt: img.alt,
      })),
    );
  expect(images.length).toBe(4);
  for (const image of images) {
    expect(image.natural).toEqual([Number(image.width), Number(image.height)]);
    expect(image.alt.length).toBeGreaterThan(40);
    const file = image.src.replace('assets/screens/', '');
    expect(await digest(`../landing/${image.src}`)).toBe(
      await digest(`../docs/assets/screenshots/${file}`),
    );
  }
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
    page.getByRole('link', { name: 'View install guide' }),
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
