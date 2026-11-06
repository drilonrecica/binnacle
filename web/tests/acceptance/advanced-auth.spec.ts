import { expect, test, type Page } from '@playwright/test';
import { Authenticator } from './totp';

// Advanced-auth acceptance (PRC-1) against the real binary with
// BINNACLE_FEATURE_ADVANCED_AUTH=true: nothing here is mocked.

const username = 'admin';
const password = 'acceptance correct horse battery';

async function signIn(page: Page, code: string) {
  await page.getByRole('textbox', { name: 'Username' }).fill(username);
  await page
    .getByRole('textbox', { name: 'Password', exact: true })
    .fill(password);
  await page.getByRole('textbox', { name: 'Authentication code' }).fill(code);
  await page.getByRole('button', { name: 'Sign in' }).click();
}

/** Walks a fresh install's first-run wizard so the app is reachable. */
async function finishOnboarding(page: Page) {
  await expect(
    page.getByRole('heading', { name: 'Before you start' }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Continue' }).click();
  await page.getByRole('button', { name: 'Save and run checks' }).click();
  await page.getByRole('button', { name: 'Continue' }).click();
  await page.getByRole('button', { name: 'Open the dashboard' }).click();
  await expect(
    page.getByRole('heading', { name: 'Before you start' }),
  ).toHaveCount(0);
}

test('enrols TOTP in Settings, then signs in with a code and a recovery code', async ({
  page,
  context,
}) => {
  await page.goto('/login');
  await signIn(page, '');
  await expect(page).not.toHaveURL(/\/login/);
  await page.goto('/settings/access');
  await finishOnboarding(page);
  await page.goto('/settings/access');
  const card = page.locator('section.card').filter({
    has: page.getByRole('heading', { name: 'Two-factor authentication' }),
  });
  await expect(card.getByText('Off', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Set up two-factor' }).click();
  const dialog = page.getByRole('dialog', {
    name: 'Set up two-factor authentication',
  });
  await dialog
    .getByRole('textbox', { name: 'Password', exact: true })
    .fill(password);
  await dialog.getByRole('button', { name: 'Continue' }).click();
  const secret = dialog.locator('.secret-row code').first();
  await expect(secret).toHaveText(/^[A-Z2-7]{32}$/);
  const app = new Authenticator((await secret.textContent()) ?? '');
  await expect(dialog.locator('.secret-row code.uri')).toContainText(
    'otpauth://totp/Binnacle:admin?',
  );
  await dialog
    .getByRole('textbox', { name: 'Code from your app' })
    .fill(await app.next());
  await dialog.getByRole('button', { name: 'Enable' }).click();

  // The codes are shown once, so they must be reachable: the dialog closes
  // and nothing covers the list or its actions (DSC-31).
  await expect(dialog).toBeHidden();
  const codes = card.locator('.codes code');
  await expect(codes).toHaveCount(10);
  const recovery = await codes.allTextContents();
  await expect(card.getByText('Enabled', { exact: true })).toBeVisible();
  await card.getByRole('button', { name: 'Done' }).click();
  await expect(codes).toHaveCount(0);

  // Sign in again from a clean browser: a password alone is refused, a
  // current code is accepted.
  await context.clearCookies();
  await page.goto('/login?next=%2Fsettings%2Faccess');
  await signIn(page, '');
  await expect(page.getByRole('alert')).toHaveText(
    'Invalid username, password, or authentication code.',
  );
  await signIn(page, await app.next());
  await expect(page).toHaveURL(/\/settings\/access/);
  await expect(card.getByText('Enabled', { exact: true })).toBeVisible();

  await context.clearCookies();
  await page.goto('/login');
  await signIn(page, recovery[0]!);
  await expect(page).not.toHaveURL(/\/login/);
  await expect(
    page.getByRole('navigation', { name: 'Primary navigation' }),
  ).toBeVisible();
});
