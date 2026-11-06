// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import ConfirmDialog from './ConfirmDialog.svelte';
import Dialog from './Dialog.svelte';
import DialogHarness from './DialogHarness.test.svelte';
import { polyfillDom, textSnippet } from './testing';

beforeAll(polyfillDom);
afterEach(cleanup);

function dialogElement() {
  const element = document.querySelector('dialog');
  if (!element) throw new Error('No dialog rendered');
  return element;
}

/** What the browser does on Escape: fire a cancelable cancel, then close. */
function pressEscape(dialog: HTMLDialogElement) {
  const cancel = new Event('cancel', { cancelable: true });
  dialog.dispatchEvent(cancel);
  if (!cancel.defaultPrevented) dialog.close();
  return cancel;
}

describe('Dialog', () => {
  it('is labelled by its title and described by its description', () => {
    render(Dialog, {
      open: true,
      title: 'Edit rule',
      description: 'Thresholds apply to new evaluations.',
      children: textSnippet('Body'),
    });
    const dialog = dialogElement();
    const title = document.getElementById(
      dialog.getAttribute('aria-labelledby') ?? '',
    );
    const description = document.getElementById(
      dialog.getAttribute('aria-describedby') ?? '',
    );
    expect(title?.textContent).toBe('Edit rule');
    expect(description?.textContent).toBe(
      'Thresholds apply to new evaluations.',
    );
  });

  it('closes on Escape and returns focus to the opener', async () => {
    render(DialogHarness);
    const opener = screen.getByRole('button', { name: 'Open' });
    await userEvent.click(opener);
    const dialog = dialogElement();
    expect(dialog.open).toBe(true);
    expect(pressEscape(dialog).defaultPrevented).toBe(false);
    expect(dialog.open).toBe(false);
    expect(document.activeElement).toBe(opener);
  });

  it('ignores Escape while it must not be dismissed', () => {
    const onclose = vi.fn();
    render(Dialog, {
      open: true,
      title: 'Deleting',
      dismissible: false,
      onclose,
      children: textSnippet('Working…'),
    });
    const dialog = dialogElement();
    expect(pressEscape(dialog).defaultPrevented).toBe(true);
    expect(dialog.open).toBe(true);
    expect(onclose).not.toHaveBeenCalled();
  });

  it('closes on a backdrop click only when dismissible', async () => {
    const { rerender } = render(Dialog, {
      open: true,
      title: 'Backdrop',
      dismissible: false,
      children: textSnippet('Body'),
    });
    const dialog = dialogElement();
    await fireEvent.click(dialog);
    expect(dialog.open).toBe(true);
    await rerender({ dismissible: true });
    await fireEvent.click(dialog);
    expect(dialog.open).toBe(false);
  });

  it('keeps Tab inside the dialog', async () => {
    render(DialogHarness);
    await userEvent.click(screen.getByRole('button', { name: 'Open' }));
    const close = screen.getByRole('button', { name: 'Close' });
    close.focus();
    await userEvent.tab();
    expect(dialogElement().contains(document.activeElement)).toBe(true);
    await userEvent.tab({ shift: true });
    expect(dialogElement().contains(document.activeElement)).toBe(true);
  });
});

describe('ConfirmDialog', () => {
  it('enables the action only once the phrase is typed exactly', async () => {
    const onconfirm = vi.fn();
    render(ConfirmDialog, {
      open: true,
      title: 'Delete history permanently?',
      confirmLabel: 'Delete history',
      phrase: 'DELETE ALL HISTORY',
      onconfirm,
    });
    const action = screen.getByRole('button', { name: 'Delete history' });
    expect((action as HTMLButtonElement).disabled).toBe(true);
    const input = screen.getByRole('textbox');
    await userEvent.type(input, 'delete all history');
    expect((action as HTMLButtonElement).disabled).toBe(true);
    await userEvent.clear(input);
    await userEvent.type(input, 'DELETE ALL HISTORY');
    expect((action as HTMLButtonElement).disabled).toBe(false);
    await userEvent.click(action);
    expect(onconfirm).toHaveBeenCalledOnce();
    expect(dialogElement().open).toBe(false);
  });

  it('stays open and shows the error when the action fails', async () => {
    const onconfirm = vi
      .fn()
      .mockRejectedValue(new Error('The channel is in use.'));
    render(ConfirmDialog, {
      open: true,
      title: 'Delete this channel?',
      confirmLabel: 'Delete',
      onconfirm,
    });
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }));
    expect(await screen.findByRole('alert')).toHaveProperty(
      'textContent',
      expect.stringContaining('The channel is in use.'),
    );
    expect(dialogElement().open).toBe(true);
  });

  it('cancels without running the action', async () => {
    const onconfirm = vi.fn();
    render(ConfirmDialog, {
      open: true,
      title: 'End this silence now?',
      onconfirm,
    });
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onconfirm).not.toHaveBeenCalled();
    expect(dialogElement().open).toBe(false);
  });
});
