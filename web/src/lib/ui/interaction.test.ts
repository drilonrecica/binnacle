// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import Combobox from './Combobox.svelte';
import Tabs from './Tabs.svelte';
import { polyfillDom } from './testing';
import DialogHarness from './DialogHarness.test.svelte';
import MenuHarness from './MenuHarness.test.svelte';

beforeAll(polyfillDom);

describe('Tabs', () => {
  const tabs = [
    { id: 'overview', label: 'Overview' },
    { id: 'logs', label: 'Logs' },
    { id: 'containers', label: 'Containers', count: 3 },
  ];

  it('uses a roving tabindex and arrow keys', async () => {
    const onchange = vi.fn();
    render(Tabs, {
      tabs,
      active: 'overview',
      prefix: 't',
      label: 'Sections',
      onchange,
    });
    const [overview, logs, containers] = screen.getAllByRole('tab') as [
      HTMLElement,
      HTMLElement,
      HTMLElement,
    ];
    expect(overview.tabIndex).toBe(0);
    expect(logs.tabIndex).toBe(-1);
    overview.focus();
    await fireEvent.keyDown(overview, { key: 'ArrowRight' });
    expect(onchange).toHaveBeenLastCalledWith('logs');
    expect(document.activeElement).toBe(logs);
    expect(logs.getAttribute('aria-selected')).toBe('true');
    await fireEvent.keyDown(logs, { key: 'End' });
    expect(document.activeElement).toBe(containers);
    await fireEvent.keyDown(containers, { key: 'ArrowRight' });
    expect(document.activeElement).toBe(overview);
  });
});

describe('Combobox', () => {
  const options = Array.from({ length: 60 }, (_, index) => ({
    value: `res_${index}`,
    label: `service-${String(index).padStart(2, '0')}`,
  }));

  it('says how many matches are left out instead of hiding them (UI-20)', async () => {
    render(Combobox, { options, label: 'Resource' });
    const input = screen.getByRole('combobox', { name: 'Resource' });
    await fireEvent.focus(input);
    expect(screen.getAllByRole('option')).toHaveLength(50);
    expect(screen.getByText('10 more, type to narrow')).toBeTruthy();
    expect(input.getAttribute('aria-describedby')).toBeTruthy();

    await userEvent.type(input, 'service-5');
    expect(screen.getAllByRole('option').length).toBeLessThanOrEqual(50);
    expect(screen.queryByText(/more, type to narrow/)).toBeNull();
  });

  it('chooses with the keyboard', async () => {
    const onchange = vi.fn();
    render(Combobox, {
      options: options.slice(0, 3),
      label: 'Resource',
      onchange,
    });
    const input = screen.getByRole('combobox', { name: 'Resource' });
    input.focus();
    await userEvent.keyboard('{ArrowDown}{Enter}');
    expect(onchange).toHaveBeenCalledWith('res_1');
  });
});

describe('Dialog', () => {
  it('opens as a modal and returns focus to the opener', async () => {
    render(DialogHarness);
    const opener = screen.getByRole('button', { name: 'Open' });
    await userEvent.click(opener);
    const dialog = document.querySelector('dialog');
    expect(dialog?.hasAttribute('open')).toBe(true);
    expect(dialog?.getAttribute('aria-labelledby')).toBeTruthy();
    expect(
      screen.getByRole('heading', { name: 'Delete history' }),
    ).toBeTruthy();
    await userEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(dialog?.hasAttribute('open')).toBe(false);
    expect(document.activeElement).toBe(opener);
  });
});

describe('Menu', () => {
  afterEach(() => vi.useRealTimers());

  it('opens from the keyboard, moves with arrows and restores focus', async () => {
    const onselect = vi.fn();
    render(MenuHarness, { onselect });
    const trigger = screen.getByRole('button', { name: 'Actions' });
    trigger.focus();
    await userEvent.keyboard('{ArrowDown}');
    const items = await screen.findAllByRole('menuitem');
    expect(document.activeElement).toBe(items[0]);
    await userEvent.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(items[1]);
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('menu')).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });
});
