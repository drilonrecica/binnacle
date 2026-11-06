import { expect, type Page } from '@playwright/test';

// Shared steps for the acceptance flows, which drive the real binary.

export const username = 'admin';
export const password = 'acceptance correct horse battery';

export async function signIn(page: Page, code = '') {
  await page.getByRole('textbox', { name: 'Username' }).fill(username);
  await page
    .getByRole('textbox', { name: 'Password', exact: true })
    .fill(password);
  // The code field exists only with advanced auth on.
  if (code) {
    await page.getByRole('textbox', { name: 'Authentication code' }).fill(code);
  }
  await page.getByRole('button', { name: 'Sign in' }).click();
}

/** Walks a fresh install's first-run wizard so the app is reachable. */
export async function finishOnboarding(page: Page) {
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
