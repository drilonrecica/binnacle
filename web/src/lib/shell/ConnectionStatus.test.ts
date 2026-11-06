// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import type { LiveStore } from '../live.svelte';
import ConnectionStatus from './ConnectionStatus.svelte';

function liveWith(state: LiveStore['state'], lastReceivedAt: string | null) {
  return { state, lastReceivedAt } as unknown as LiveStore;
}

describe('ConnectionStatus (UI-3)', () => {
  it('announces only the state word, never a clock', () => {
    const quietSince = new Date(Date.now() - 60_000).toISOString();
    render(ConnectionStatus, { live: liveWith('stale', quietSince) });
    const regions = screen.getAllByRole('status');
    expect(regions).toHaveLength(1);
    expect(regions[0]?.textContent).toBe('Stale');
    // The age is shown next to the word, outside the region.
    const status = screen.getByTestId('connection-status');
    expect(status.textContent).toMatch(/Stale\s*since /);
    expect(status.getAttribute('aria-live')).toBeNull();
  });

  it('shows no time while live', () => {
    render(ConnectionStatus, {
      live: liveWith('live', new Date().toISOString()),
    });
    expect(screen.getByTestId('connection-status').textContent?.trim()).toBe(
      'Live',
    );
  });
});
