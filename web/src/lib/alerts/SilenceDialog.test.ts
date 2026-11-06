// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import type { LiveSnapshot } from '../live.svelte';
import { field, polyfillDom } from '../ui/testing';

vi.mock('../api/silences', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/silences')>()),
  createSilence: vi.fn(),
}));

const { default: SilenceDialog } = await import('./SilenceDialog.svelte');
const silencesApi = await import('../api/silences');

beforeAll(polyfillDom);
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const snapshot = {
  resources: [
    { id: 'res1', name: 'api.production', status: 'healthy', project: 'shop' },
  ],
} as unknown as LiveSnapshot;

function form() {
  const element = screen.getByRole('dialog');
  return {
    element,
    field: (text: string) => field(element, text),
    create: () =>
      userEvent.click(
        within(element).getByRole('button', { name: 'Create silence' }),
      ),
  };
}

describe('SilenceDialog', () => {
  it('starts in the reason field with the scope it was opened for', async () => {
    render(SilenceDialog, {
      open: true,
      snapshot,
      scope: 'resource',
      scopeId: 'res1',
    });
    const f = form();
    await vi.waitFor(() =>
      expect(document.activeElement).toBe(f.field('Reason')),
    );
    expect((f.field('Resource') as HTMLSelectElement).value).toBe('res1');
    expect(f.element.textContent).toContain('api.production');
  });

  it('asks for the scope target and a custom end before sending', async () => {
    render(SilenceDialog, { open: true, snapshot });
    const f = form();
    await userEvent.selectOptions(f.field('Scope'), 'project');
    await userEvent.type(f.field('Reason'), 'Deploy');
    await f.create();
    expect(within(f.element).getByRole('alert').textContent).toBe(
      'Choose a project.',
    );
    await userEvent.selectOptions(f.field('Project'), 'shop');
    await userEvent.selectOptions(f.field('Duration'), 'custom');
    await f.create();
    expect(within(f.element).getByRole('alert').textContent).toBe(
      'Choose when the silence ends.',
    );
    expect(silencesApi.createSilence).not.toHaveBeenCalled();
  });

  it('silences the whole server without a scope id', async () => {
    vi.mocked(silencesApi.createSilence).mockResolvedValue({
      id: 'sil-1',
      scopeType: 'server',
      reason: 'Maintenance',
      startsAt: '',
      endsAt: '',
    });
    const oncreated = vi.fn();
    render(SilenceDialog, { open: true, snapshot, oncreated });
    const f = form();
    await userEvent.type(f.field('Reason'), 'Maintenance');
    await f.create();
    expect(silencesApi.createSilence).toHaveBeenCalledWith({
      scopeType: 'server',
      scopeId: undefined,
      reason: 'Maintenance',
      preset: '1h',
      customEnd: undefined,
    });
    expect(oncreated).toHaveBeenCalledOnce();
    expect((f.element as HTMLDialogElement).open).toBe(false);
  });
});
