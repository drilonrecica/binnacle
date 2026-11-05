// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import Badge from './Badge.svelte';
import Button from './Button.svelte';
import IconButton from './IconButton.svelte';
import Status from './Status.svelte';
import StatusPill from './StatusPill.svelte';
import { textSnippet } from './testing';

describe('Status', () => {
  it.each([
    ['healthy', 'circle', 'Healthy'],
    ['degraded', 'ring', 'Degraded'],
    ['down', 'square', 'Down'],
    ['paused', 'diamond', 'Paused'],
    ['unknown', 'dashed', 'Unknown'],
  ])('pairs %s with a %s marker and a word', (status, shape, word) => {
    const { container } = render(Status, { status });
    expect(container.querySelector('svg')?.dataset.shape).toBe(shape);
    expect(container.querySelector('svg')?.getAttribute('aria-hidden')).toBe(
      'true',
    );
    expect(screen.getByText(word)).toBeTruthy();
  });

  it('keeps the word for screen readers when the label is hidden', () => {
    render(Status, { status: 'down', hideLabel: true });
    expect(screen.getByText('Down').classList.contains('sr-only')).toBe(true);
  });

  it('takes a tone and label directly', () => {
    const { container } = render(Status, { tone: 'info', label: 'Silenced' });
    expect(container.querySelector('svg')?.dataset.shape).toBe('diamond');
    expect(screen.getByText('Silenced')).toBeTruthy();
  });

  it('StatusPill renders the same shape and word', () => {
    const { container } = render(StatusPill, { status: 'firing' });
    expect(container.querySelector('svg')?.dataset.shape).toBe('square');
    expect(screen.getByText('Down')).toBeTruthy();
  });
});

describe('Badge', () => {
  it('uses the tone shape for its marker', () => {
    const { container } = render(Badge, {
      tone: 'warn',
      dot: true,
      children: textSnippet('Degraded'),
    });
    expect(container.querySelector('svg')?.dataset.shape).toBe('ring');
    expect(screen.getByText('Degraded')).toBeTruthy();
  });
});

describe('Button', () => {
  it('is busy and disabled while loading', async () => {
    const onclick = vi.fn();
    render(Button, {
      variant: 'primary',
      loading: true,
      onclick,
      children: textSnippet('Save'),
    });
    const button = screen.getByRole('button', { name: 'Save' });
    expect(button.getAttribute('aria-busy')).toBe('true');
    expect((button as HTMLButtonElement).disabled).toBe(true);
    await userEvent.click(button);
    expect(onclick).not.toHaveBeenCalled();
  });

  it('renders the quiet variant', () => {
    render(Button, { variant: 'quiet', children: textSnippet('Cancel') });
    expect(
      screen
        .getByRole('button', { name: 'Cancel' })
        .classList.contains('quiet'),
    ).toBe(true);
  });

  it('IconButton is always labelled', () => {
    render(IconButton, { label: 'Close', children: textSnippet('×') });
    expect(screen.getByRole('button', { name: 'Close' })).toBeTruthy();
  });
});
