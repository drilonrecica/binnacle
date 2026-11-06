// @vitest-environment jsdom
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';
import type { LiveStore } from '../live.svelte';
import { router } from '../router.svelte';
import { polyfillDom } from '../ui/testing';

vi.mock('../api/alerts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/alerts')>()),
  listRules: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/checks', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/checks')>()),
  listChecks: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/incidents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/incidents')>()),
  listIncidents: vi.fn().mockResolvedValue([]),
  listAlerts: vi.fn().mockResolvedValue([]),
}));
vi.mock('../api/silences', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/silences')>()),
  listSilences: vi.fn().mockResolvedValue([]),
}));

const { default: Alerts } = await import('./Alerts.svelte');

beforeAll(() => {
  polyfillDom();
  window.scrollTo = () => {};
});

describe('Incidents page', () => {
  it('opens the silence dialog when the palette links here while it is open (UI-20)', async () => {
    history.replaceState({}, '', '/incidents');
    router.sync();
    render(Alerts, { live: { snapshot: null } as unknown as LiveStore });
    expect(document.querySelector('dialog[open]')).toBeNull();

    router.navigate('/incidents?tab=silences&new=1');

    await waitFor(() =>
      expect(document.querySelector('dialog[open]')).not.toBeNull(),
    );
    expect(router.param('new')).toBe('');
    expect(router.param('tab')).toBe('silences');
  });
});
