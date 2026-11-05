// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import Toaster from './Toaster.svelte';
import { DEFAULT_DURATION, ToastStore, toasts } from './toast.svelte';

describe('toast store (UI-17)', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('dismisses a plain toast after its duration', () => {
    const store = new ToastStore();
    store.info('Saved');
    vi.advanceTimersByTime(DEFAULT_DURATION - 1);
    expect(store.items).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(store.items).toHaveLength(0);
  });

  it('never times out a toast with an action', () => {
    const store = new ToastStore();
    store.success('Silenced', {
      duration: 1000,
      action: { label: 'Undo', onclick: () => {} },
    });
    expect(store.items[0].duration).toBe(0);
    vi.advanceTimersByTime(60_000);
    expect(store.items).toHaveLength(1);
  });

  it('pauses while hovered or focused and resumes with the time left', () => {
    const store = new ToastStore();
    const id = store.info('Saved', { duration: 1000 });
    vi.advanceTimersByTime(600);
    store.pause(id, 'hover');
    store.pause(id, 'focus');
    vi.advanceTimersByTime(10_000);
    expect(store.items).toHaveLength(1);
    store.resume(id, 'hover');
    vi.advanceTimersByTime(10_000);
    expect(store.items).toHaveLength(1); // still focused
    store.resume(id, 'focus');
    vi.advanceTimersByTime(399);
    expect(store.items).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(store.items).toHaveLength(0);
  });

  it('keeps at most five toasts', () => {
    const store = new ToastStore();
    for (let index = 0; index < 7; index += 1) store.info(`t${index}`);
    expect(store.items.map((item) => item.title)).toEqual([
      't2',
      't3',
      't4',
      't5',
      't6',
    ]);
  });
});

describe('Toaster', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    for (const item of [...toasts.items]) toasts.dismiss(item.id);
    vi.useRealTimers();
  });

  it('announces errors assertively and everything else politely', async () => {
    render(Toaster);
    toasts.error('Could not save');
    toasts.success('Saved');
    await vi.waitFor(() => screen.getByText('Saved'));
    expect(
      within(screen.getByRole('alert')).getByText('Could not save'),
    ).toBeTruthy();
    expect(within(screen.getByRole('status')).getByText('Saved')).toBeTruthy();
    expect(
      within(screen.getByRole('status')).queryByText('Could not save'),
    ).toBeNull();
  });

  it('pauses on hover', async () => {
    render(Toaster);
    toasts.info('Exported', { duration: 1000 });
    const toast = await vi.waitFor(() =>
      screen.getByRole('group', { name: 'Exported' }),
    );
    await fireEvent.mouseEnter(toast);
    vi.advanceTimersByTime(5000);
    expect(toasts.items).toHaveLength(1);
    await fireEvent.mouseLeave(toast);
    vi.advanceTimersByTime(1000);
    expect(toasts.items).toHaveLength(0);
  });
});
