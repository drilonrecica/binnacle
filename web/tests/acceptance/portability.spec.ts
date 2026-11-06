import { expect, request, test, type Page } from '@playwright/test';
import { finishOnboarding, signIn } from './app';

// Portability acceptance (PRC-1) against the real binary with
// BINNACLE_FEATURE_PORTABILITY and BINNACLE_PROMETHEUS_ENABLED on: nothing
// here is mocked. Tokens are exercised by a cookie-less client, the way a
// Prometheus server or a script would use them.

interface Resource {
  id: string;
  name: string;
  status: string;
  infrastructure?: boolean;
}

/** Scrapes /metrics without cookies, as Prometheus does. */
async function scrape(baseURL: string, token: string) {
  const client = await request.newContext({
    baseURL,
    extraHTTPHeaders: { Authorization: `Bearer ${token}` },
  });
  try {
    const metrics = await client.get('/metrics');
    const server = await client.get('/api/v1/server');
    return {
      metrics: metrics.status(),
      body: await metrics.text(),
      server: server.status(),
    };
  } finally {
    await client.dispose();
  }
}

/** Signs in, walking the first-run wizard if no test has finished it yet. */
async function signedIn(page: Page) {
  await page.goto('/login');
  await signIn(page);
  await expect(page).not.toHaveURL(/\/login/);
  const onboarding = await page.request.get('/api/v1/onboarding');
  expect(onboarding.status()).toBe(200);
  const state = (await onboarding.json()) as { completedAt?: string };
  if (!state.completedAt) {
    await page.goto('/settings/integrations');
    await finishOnboarding(page);
  }
}

function escape(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

test('creates a scoped token, scrapes Prometheus with it, then revokes it', async ({
  page,
  baseURL,
}) => {
  await signedIn(page);
  await page.goto('/settings/integrations');
  await page.getByRole('button', { name: 'New token' }).click();
  const dialog = page.getByRole('dialog', { name: 'New API token' });
  await dialog.getByLabel('Name').fill('Prometheus');
  await dialog.getByLabel('Host metrics').uncheck();
  await dialog.getByLabel('History and Prometheus').check();
  await dialog.getByRole('button', { name: 'Create token' }).click();

  const plaintext = page
    .getByRole('dialog', { name: 'Token created' })
    .locator('.plaintext code');
  await expect(plaintext).toHaveText(/^bnk_\S+$/);
  const token = (await plaintext.textContent()) ?? '';

  // The token scrapes, and its one scope is all it grants.
  const granted = await scrape(baseURL!, token);
  expect(granted.metrics).toBe(200);
  expect(granted.body).toContain('binnacle_host_cpu_percent ');
  expect(granted.server).toBe(403);

  await page.getByRole('button', { name: 'Done' }).click();
  const row = page.getByRole('row').filter({ hasText: 'Prometheus' });
  // The scope column names the one scope chosen.
  await expect(row.getByRole('cell').nth(1)).toHaveText(
    'History and Prometheus',
  );
  await row.getByRole('button', { name: 'Revoke' }).click();
  await page
    .getByRole('alertdialog', { name: 'Revoke this token?' })
    .or(page.getByRole('dialog', { name: 'Revoke this token?' }))
    .getByRole('button', { name: 'Revoke token' })
    .click();
  await expect(row.getByText('Revoked', { exact: true })).toBeVisible();
  await expect(row.getByRole('button', { name: 'Revoke' })).toHaveCount(0);

  const revoked = await scrape(baseURL!, token);
  expect(revoked.metrics).toBe(401);
  expect(revoked.server).toBe(401);

  // The listing survives a reload: it comes from the server, not the page.
  await page.reload();
  await expect(
    page
      .getByRole('row')
      .filter({ hasText: 'Prometheus' })
      .getByText('Revoked', { exact: true }),
  ).toBeVisible();
});

test('pins resources in Settings and the Overview lists them first', async ({
  page,
}) => {
  await signedIn(page);
  const listed = await page.request.get('/api/v1/resources');
  expect(listed.status()).toBe(200);
  const pickable = ((await listed.json()) as Resource[])
    .filter((r) => r.status !== 'archived' && !r.infrastructure)
    .sort((a, b) => a.name.localeCompare(b.name));
  expect(pickable.length).toBeGreaterThanOrEqual(2);
  // Pin the last two by name, then put the later-pinned one first.
  const [first, second] = pickable.slice(-2).reverse() as [Resource, Resource];

  await page.goto('/settings/appearance');
  const card = page.locator('section.card').filter({
    has: page.getByRole('heading', { name: 'Pinned resources' }),
  });
  for (const resource of [second, first]) {
    await page
      .getByRole('combobox', { name: 'Resource to pin' })
      .fill(resource.name);
    await page
      .getByRole('option', { name: new RegExp(`^${escape(resource.name)}`) })
      .click();
    await page.getByRole('button', { name: 'Pin', exact: true }).click();
    await expect(card.locator('.pins li').last()).toContainText(resource.name);
  }
  await card
    .locator('.pins li')
    .nth(1)
    .getByRole('button', { name: 'Move up' })
    .click();
  await expect(card.locator('.pins .pin-name')).toHaveText([
    first.name,
    second.name,
  ]);

  // Stored on the server: a reload and the API both see the order.
  await expect
    .poll(async () => {
      const response = await page.request.get('/api/v1/preferences');
      const body = (await response.json()) as {
        preferences: { pinnedResources: string[] };
      };
      return body.preferences.pinnedResources;
    })
    .toEqual([first.id, second.id]);
  await page.reload();
  await expect(card.locator('.pins .pin-name')).toHaveText([
    first.name,
    second.name,
  ]);

  // The Overview puts both in a leading Pinned group, each marked.
  await page.goto('/');
  for (const resource of [first, second]) {
    const row = page.getByRole('row').filter({ hasText: resource.name });
    await expect(row.getByLabel('Pinned')).toBeVisible();
  }
  await expect(page.locator('.group-name').first()).toHaveText('Pinned');

  // Unpinning one leaves the other.
  await page.goto('/settings/appearance');
  await card
    .locator('.pins li')
    .filter({ hasText: first.name })
    .getByRole('button', { name: 'Unpin' })
    .click();
  await expect(card.locator('.pins .pin-name')).toHaveText([second.name]);
  await page.goto('/');
  await expect(
    page.getByRole('row').filter({ hasText: first.name }).getByLabel('Pinned'),
  ).toHaveCount(0);
});
