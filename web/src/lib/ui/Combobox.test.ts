// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import Combobox from './Combobox.svelte';
import { polyfillDom } from './testing';

beforeAll(polyfillDom);
afterEach(cleanup);

const options = [
  { value: 'res1', label: 'api.production', hint: 'binnacle' },
  { value: 'res_beta', label: 'worker.production', hint: 'binnacle' },
  { value: 'infra1', label: 'postgres.internal', keywords: ['database'] },
];

function setup(value = '') {
  const onchange = vi.fn();
  render(Combobox, { options, label: 'Resource', value, onchange });
  const input = screen.getByRole('combobox', { name: 'Resource' });
  return { input, onchange };
}

describe('Combobox', () => {
  it('opens on focus with the listbox wired to the input', async () => {
    const { input } = setup();
    expect(input.getAttribute('aria-expanded')).toBe('false');
    await userEvent.click(input);
    expect(input.getAttribute('aria-expanded')).toBe('true');
    const listbox = screen.getByRole('listbox', { name: 'Resource' });
    expect(input.getAttribute('aria-controls')).toBe(listbox.id);
    expect(screen.getAllByRole('option')).toHaveLength(3);
  });

  it('narrows by label and keywords as you type', async () => {
    const { input } = setup();
    await userEvent.type(input, 'worker');
    expect(screen.getAllByRole('option').map((o) => o.textContent)).toEqual([
      expect.stringContaining('worker.production'),
    ]);
    await userEvent.clear(input);
    await userEvent.type(input, 'database');
    expect(screen.getByRole('option').textContent).toContain(
      'postgres.internal',
    );
  });

  it('says when nothing matches', async () => {
    const { input } = setup();
    await userEvent.type(input, 'zzz');
    expect(screen.queryAllByRole('option')).toHaveLength(0);
    expect(screen.getByText('No matches')).toBeTruthy();
  });

  it('moves the active option with the arrows via aria-activedescendant', async () => {
    const { input, onchange } = setup();
    await userEvent.click(input);
    const active = () =>
      document.getElementById(input.getAttribute('aria-activedescendant')!);
    expect(active()?.textContent).toContain('api.production');
    await userEvent.keyboard('{ArrowDown}{ArrowDown}{ArrowDown}');
    // It stops at the last option instead of wrapping.
    expect(active()?.textContent).toContain('postgres.internal');
    await userEvent.keyboard('{ArrowUp}{Enter}');
    expect(onchange).toHaveBeenCalledWith('res_beta');
    expect(input.getAttribute('aria-expanded')).toBe('false');
    expect((input as HTMLInputElement).value).toBe('worker.production');
  });

  it('starts on the selected option and marks it selected', async () => {
    const { input } = setup('infra1');
    expect((input as HTMLInputElement).value).toBe('postgres.internal');
    await userEvent.click(input);
    const selected = screen.getByRole('option', { selected: true });
    expect(selected.textContent).toContain('postgres.internal');
    expect(input.getAttribute('aria-activedescendant')).toBe(selected.id);
  });

  it('chooses with the pointer', async () => {
    const { input, onchange } = setup();
    await userEvent.click(input);
    await userEvent.click(screen.getByRole('option', { name: /postgres/ }));
    expect(onchange).toHaveBeenCalledWith('infra1');
  });

  it('closes on Escape and Tab and keeps the value', async () => {
    const { input, onchange } = setup('res1');
    await userEvent.click(input);
    await userEvent.type(input, 'wor');
    await userEvent.keyboard('{Escape}');
    expect(input.getAttribute('aria-expanded')).toBe('false');
    expect((input as HTMLInputElement).value).toBe('api.production');
    await userEvent.keyboard('{ArrowDown}');
    expect(input.getAttribute('aria-expanded')).toBe('true');
    await userEvent.tab();
    expect(input.getAttribute('aria-expanded')).toBe('false');
    expect(onchange).not.toHaveBeenCalled();
  });

  it('lets Escape through to a dialog once the listbox is closed', async () => {
    const { input } = setup();
    await userEvent.click(input);
    await userEvent.keyboard('{Escape}');
    const second = new KeyboardEvent('keydown', {
      key: 'Escape',
      bubbles: true,
      cancelable: true,
    });
    input.dispatchEvent(second);
    expect(second.defaultPrevented).toBe(false);
  });

  it('does not open while disabled', async () => {
    render(Combobox, { options, label: 'Resource', disabled: true });
    const input = screen.getByRole('combobox', { name: 'Resource' });
    await userEvent.click(input);
    expect(input.getAttribute('aria-expanded')).toBe('false');
  });
});
