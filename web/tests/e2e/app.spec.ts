import { expect, test } from '@playwright/test';

test('renders the TALOS application shell', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle('TALOS');
  await expect(page.getByRole('heading', { name: 'TALOS' })).toBeVisible();
});
