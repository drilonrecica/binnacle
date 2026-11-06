// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render, screen, waitFor } from '@testing-library/svelte';
import type { LiveStore } from '../live.svelte';
import { router } from '../router.svelte';
import RouteView from './RouteView.svelte';
import { mounts } from './testing/mounts';

const { stub } = vi.hoisted(() => ({
  stub: async () => ({
    default: (await import('./testing/PageStub.test.svelte')).default,
  }),
}));
vi.mock('../pages/Overview.svelte', stub);
vi.mock('../pages/Host.svelte', stub);
vi.mock('../pages/Resources.svelte', stub);
vi.mock('../pages/Activity.svelte', stub);
vi.mock('../pages/Logs.svelte', stub);
vi.mock('../pages/Alerts.svelte', stub);
vi.mock('../pages/Incident.svelte', stub);
vi.mock('../pages/ResourceDetail.svelte', stub);
vi.mock('../pages/Settings.svelte', stub);

const live = {} as unknown as LiveStore;

function go(path: string) {
  history.pushState({}, '', path);
  router.sync();
  flushSync();
}

afterEach(() => {
  mounts.length = 0;
});

describe('RouteView', () => {
  it('rebuilds a resource page when only the id changes (UI-4)', () => {
    go('/resources/res_a');
    render(RouteView, { live, onsignedout: () => {} });
    go('/resources/res_b');
    expect(mounts).toEqual(['res_a', 'res_b']);
    expect(screen.getByTestId('stub').textContent).toBe(
      'mounted for res_b, showing res_b',
    );
  });

  it('rebuilds an incident page when only the id changes (UI-4)', async () => {
    go('/incidents/inc_1');
    render(RouteView, { live, onsignedout: () => {} });
    // Incident pages load on demand (UI-16).
    await screen.findByTestId('stub');
    go('/incidents/inc_2');
    await waitFor(() => expect(mounts).toEqual(['inc_1', 'inc_2']));
    expect(mounts).toEqual(['inc_1', 'inc_2']);
  });

  it('shows a loading state, then the lazily loaded page (UI-16)', async () => {
    go('/settings/general');
    render(RouteView, { live, onsignedout: () => {} });
    expect(screen.getByText('Loading page…')).toBeTruthy();
    expect(await screen.findByTestId('stub')).toBeTruthy();
    expect(screen.queryByText('Loading page…')).toBeNull();
  });

  it('says so when an address matches no page', () => {
    go('/nowhere');
    render(RouteView, { live, onsignedout: () => {} });
    expect(
      screen.getByRole('heading', { level: 1, name: 'Page not found' }),
    ).toBeTruthy();
    expect(
      screen.getByRole('link', { name: 'Go to Headroom' }).getAttribute('href'),
    ).toBe('/headroom');
  });
});
