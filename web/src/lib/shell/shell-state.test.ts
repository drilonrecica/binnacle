// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest';

const listIncidents = vi.fn().mockResolvedValue([{ id: 'a' }, { id: 'b' }]);
vi.mock('../api/incidents', () => ({ listIncidents }));

const { shell } = await import('./shell-state.svelte');

describe('shell incident badge', () => {
  it('skips its own poll while a page reported a fresh count (UI-9)', async () => {
    await shell.pollOpenIncidents(1_000_000);
    expect(listIncidents).toHaveBeenCalledTimes(1);
    expect(shell.openIncidents).toBe(2);

    shell.reportOpenIncidents(5, 2_000_000);
    await shell.pollOpenIncidents(2_000_000 + 10_000);
    expect(listIncidents).toHaveBeenCalledTimes(1);
    expect(shell.openIncidents).toBe(5);

    // Once the report is older than the interval the shell polls again.
    await shell.pollOpenIncidents(2_000_000 + 31_000);
    expect(listIncidents).toHaveBeenCalledTimes(2);
    expect(shell.openIncidents).toBe(2);
  });
});
