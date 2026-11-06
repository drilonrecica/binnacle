import { expect, test } from '@playwright/test';
import { mockApp } from './fixtures';

// §Entry: setup, onboarding and sign-in on a centred card with the mark.

test('sign-in shows the brand, reports a failure and waits out a rate limit', async ({
  page,
}) => {
  await mockApp(page, { authenticated: false });
  let attempts = 0;
  await page.route('**/api/v1/auth/login', (route) => {
    attempts += 1;
    return route.fulfill({
      status: 429,
      json: {
        error: {
          code: 'rate_limited',
          message: 'Too many attempts. Try again shortly.',
          details: { retryAfterSeconds: 30 },
        },
      },
    });
  });
  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  const mark = page.locator('img[src*="binnacle-mark"]:visible').first();
  expect(
    await mark.evaluate((image) => (image as HTMLImageElement).naturalWidth),
  ).toBeGreaterThan(0);
  // No code field unless two-factor is available.
  await expect(page.getByLabel('Authentication code')).toHaveCount(0);

  await page.getByLabel('Username').fill('admin');
  await page.getByLabel('Password').fill('wrong password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  const error = page.getByRole('alert');
  await expect(error).toContainText('Too many attempts');
  // The error takes focus so a screen reader hears it.
  await expect(error).toBeFocused();
  await expect(
    page.getByRole('button', { name: /Sign in|Try again/ }),
  ).toBeDisabled();
  expect(attempts).toBe(1);
});

test('a deep link while signed out returns there after sign-in', async ({
  page,
}) => {
  let signedIn = false;
  await mockApp(page, { authenticated: false });
  await page.route('**/api/v1/auth/session', (route) =>
    signedIn
      ? route.fulfill({
          json: {
            user: { id: 'admin', username: 'admin' },
            expiresAt: '2026-07-11T13:00:00Z',
            absoluteExpiresAt: '2026-07-11T14:00:00Z',
          },
        })
      : route.fulfill({
          status: 401,
          json: { error: { code: 'unauthorized' } },
        }),
  );
  await page.route('**/api/v1/session', (route) =>
    route.fulfill({ status: signedIn ? 204 : 401 }),
  );
  await page.route('**/api/v1/auth/login', (route) => {
    signedIn = true;
    return route.fulfill({
      json: { user: { id: 'admin', username: 'admin' } },
    });
  });
  await page.goto('/incidents?tab=silences');
  await expect(page).toHaveURL(/\/login\?next=%2Fincidents%3Ftab%3Dsilences$/);
  await page.getByLabel('Username').fill('admin');
  await page.getByLabel('Password').fill('correct horse battery staple');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/incidents\?tab=silences$/);
  await expect(page.getByRole('tab', { name: /Silences/ })).toHaveAttribute(
    'aria-selected',
    'true',
  );
});

test('setup verifies the token, then checks the password before claiming', async ({
  page,
}) => {
  await mockApp(page, { authenticated: false, setupAvailable: true });
  let claim: Record<string, unknown> | null = null;
  await page.route('**/api/v1/setup/verify', (route) =>
    route.request().postDataJSON()?.token === 'good-token'
      ? route.fulfill({ status: 204 })
      : route.fulfill({ status: 401, json: {} }),
  );
  await page.route('**/api/v1/setup/claim', (route) => {
    claim = route.request().postDataJSON();
    return route.fulfill({ status: 204 });
  });
  await page.goto('/setup');
  await expect(
    page.getByRole('heading', { name: 'Set up Binnacle' }),
  ).toBeVisible();
  await page.getByLabel('Setup token').fill('bad-token');
  await page.getByRole('button', { name: 'Verify token' }).click();
  await expect(page.getByRole('alert')).toContainText('invalid or expired');

  await page.getByLabel('Setup token').fill('good-token');
  await page.getByRole('button', { name: 'Verify token' }).click();
  await expect(
    page.getByRole('heading', { name: 'Create the administrator' }),
  ).toBeVisible();
  await page.getByLabel('Username').fill('admin');
  await page
    .getByRole('textbox', { name: 'Password', exact: true })
    .fill('a long enough passphrase 1');
  await page
    .getByRole('textbox', { name: 'Confirm password' })
    .fill('a different passphrase 1');
  await page.getByRole('button', { name: 'Create administrator' }).click();
  // The form error takes focus; the field is marked invalid as well.
  await expect(page.locator('p[role="alert"][tabindex="-1"]')).toHaveText(
    'The passwords do not match.',
  );
  await expect(
    page.getByRole('textbox', { name: 'Confirm password' }),
  ).toHaveAttribute('aria-invalid', 'true');
  expect(claim).toBeNull();

  await page
    .getByRole('textbox', { name: 'Confirm password' })
    .fill('a long enough passphrase 1');
  await page.getByRole('button', { name: 'Create administrator' }).click();
  await expect.poll(() => claim).not.toBeNull();
  expect(claim).toEqual({
    token: 'good-token',
    username: 'admin',
    password: 'a long enough passphrase 1',
  });
});

test('onboarding runs without the shell and steps through to the checks', async ({
  page,
}) => {
  await mockApp(page, { onboarded: false });
  await page.goto('/onboarding');
  await expect(
    page.getByRole('navigation', { name: 'Primary navigation' }),
  ).toHaveCount(0);
  await expect(
    page.getByRole('heading', { name: 'Before you start' }),
  ).toBeVisible();
  await expect(
    page.getByRole('list', { name: 'Onboarding progress' }),
  ).toBeVisible();
});
