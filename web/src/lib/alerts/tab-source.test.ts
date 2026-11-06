import { describe, expect, it } from 'vitest';
import { refreshSources, TabSource } from './tab-source.svelte';

describe('TabSource', () => {
  it('settles each source on its own when one request fails (UI-9)', async () => {
    const incidents = new TabSource(async () => ['inc-1']);
    const silences = new TabSource<string[]>(async () => {
      throw new Error('Silences are unavailable.');
    });

    await refreshSources([incidents, silences]);

    expect(incidents.data).toEqual(['inc-1']);
    expect(incidents.error).toBe('');
    expect(incidents.loadedAt).not.toBeNull();
    expect(silences.data).toBeNull();
    expect(silences.failed).toBe(true);
    expect(silences.error).toBe('Silences are unavailable.');
  });

  it('starts loading, then keeps rows as stale when a refresh fails', async () => {
    let fail = false;
    const source = new TabSource(async () => {
      if (fail) throw new Error('Network down');
      return [1, 2];
    });
    expect(source.loading).toBe(true);

    await source.load();
    expect(source.loading).toBe(false);
    expect(source.stale).toBe(false);
    const loadedAt = source.loadedAt;

    fail = true;
    await source.load();
    expect(source.data).toEqual([1, 2]);
    expect(source.stale).toBe(true);
    expect(source.failed).toBe(false);
    expect(source.loadedAt).toBe(loadedAt);

    fail = false;
    await source.load();
    expect(source.stale).toBe(false);
  });

  it('drops results that arrive after the page was left', async () => {
    const controller = new AbortController();
    const source = new TabSource(async () => {
      controller.abort();
      return ['late'];
    });
    await refreshSources([source], controller.signal);
    expect(source.data).toBeNull();
    expect(source.loading).toBe(true);
  });
});
