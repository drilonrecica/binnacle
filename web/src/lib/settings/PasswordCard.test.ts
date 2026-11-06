// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { field, polyfillDom } from '../ui/testing';

vi.mock('../api/access', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/access')>()),
  changePassword: vi.fn(),
}));

const { default: PasswordCard } = await import('./PasswordCard.svelte');
const accessApi = await import('../api/access');

beforeAll(polyfillDom);
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

async function openDialog() {
  render(PasswordCard);
  await userEvent.click(
    screen.getByRole('button', { name: 'Change password…' }),
  );
  const element = screen.getByRole('dialog');
  const submit = within(element).getByRole('button', {
    name: 'Change password',
  }) as HTMLButtonElement;
  return { element, submit, field: (text: string) => field(element, text) };
}

describe('PasswordCard', () => {
  it('needs the current password and a long, confirmed new one', async () => {
    const f = await openDialog();
    expect(f.submit.disabled).toBe(true);
    await userEvent.type(f.field('Current password'), 'old');
    await userEvent.type(f.field('New password'), 'short');
    expect(f.element.textContent).toMatch(/12 characters/);
    expect(f.submit.disabled).toBe(true);
    await userEvent.clear(f.field('New password'));
    await userEvent.type(f.field('New password'), 'twelve chars!');
    await userEvent.type(f.field('Confirm new password'), 'twelve chars?');
    expect(f.submit.disabled).toBe(true);
    await userEvent.clear(f.field('Confirm new password'));
    await userEvent.type(f.field('Confirm new password'), 'twelve chars!');
    expect(f.submit.disabled).toBe(false);
  });

  it('counts characters, not UTF-16 units, toward the minimum', async () => {
    const f = await openDialog();
    await userEvent.type(f.field('Current password'), 'old');
    // Six emoji are twelve UTF-16 units but only six characters.
    const short = '🔑🔑🔑🔑🔑🔑';
    await userEvent.type(f.field('New password'), short);
    await userEvent.type(f.field('Confirm new password'), short);
    expect(f.submit.disabled).toBe(true);
  });

  it('changes the password, closes and clears the fields', async () => {
    vi.mocked(accessApi.changePassword).mockResolvedValue(undefined);
    const f = await openDialog();
    await userEvent.type(f.field('Current password'), 'old password');
    await userEvent.type(f.field('New password'), 'a long passphrase');
    await userEvent.type(f.field('Confirm new password'), 'a long passphrase');
    await userEvent.click(f.submit);
    expect(accessApi.changePassword).toHaveBeenCalledWith(
      'old password',
      'a long passphrase',
    );
    expect((f.element as HTMLDialogElement).open).toBe(false);
    await userEvent.click(
      screen.getByRole('button', { name: 'Change password…' }),
    );
    expect((f.field('Current password') as HTMLInputElement).value).toBe('');
  });

  it('keeps the dialog open with a wrong current password', async () => {
    vi.mocked(accessApi.changePassword).mockRejectedValue(
      new Error('The current password is wrong.'),
    );
    const f = await openDialog();
    await userEvent.type(f.field('Current password'), 'nope');
    await userEvent.type(f.field('New password'), 'a long passphrase');
    await userEvent.type(f.field('Confirm new password'), 'a long passphrase');
    await userEvent.click(f.submit);
    expect((await within(f.element).findByRole('alert')).textContent).toContain(
      'The current password is wrong.',
    );
    expect((f.element as HTMLDialogElement).open).toBe(true);
  });
});
