// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import type { LiveSnapshot } from '../live.svelte';
import type { Check } from '../api/checks';
import { field, polyfillDom } from '../ui/testing';

vi.mock('../api/checks', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/checks')>()),
  createCheck: vi.fn(),
  updateCheck: vi.fn(),
}));

const { default: CheckDialog } = await import('./CheckDialog.svelte');
const checksApi = await import('../api/checks');

beforeAll(polyfillDom);
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const snapshot = {
  resources: [
    { id: 'res1', name: 'api.production', status: 'healthy' },
    { id: 'res2', name: 'worker.production', status: 'healthy' },
  ],
} as unknown as LiveSnapshot;

const existing: Check = {
  id: 'check-1',
  resourceId: 'res2',
  name: 'Worker health',
  url: 'https://worker.example.com/health',
  method: 'HEAD',
  interval: 60e9,
  timeout: 10e9,
  expectedStatusMin: 200,
  expectedStatusMax: 299,
  required: false,
  enabled: true,
  createdAt: '2026-07-10T12:00:00Z',
  updatedAt: '2026-07-10T12:00:00Z',
};

function dialog() {
  const element = screen.getByRole('dialog');
  return {
    ...within(element),
    getByLabelText: (text: string) => field(element, text),
  };
}

describe('CheckDialog', () => {
  it('lists every problem at once and sends nothing', async () => {
    render(CheckDialog, { open: true, snapshot });
    const form = dialog();
    await userEvent.type(form.getByLabelText('URL'), 'http://localhost:8080');
    await userEvent.clear(form.getByLabelText('Every'));
    await userEvent.type(form.getByLabelText('Every'), '5');
    await userEvent.click(form.getByRole('button', { name: 'Create check' }));
    expect(
      form.getByText('Choose the resource this check belongs to.'),
    ).toBeTruthy();
    expect(form.getByText('Give the check a name.')).toBeTruthy();
    expect(form.getByText(/cannot target localhost/)).toBeTruthy();
    expect(form.getByText('Between 10 seconds and 1 hour.')).toBeTruthy();
    expect(form.getByLabelText('Name').getAttribute('aria-invalid')).toBe(
      'true',
    );
    expect(checksApi.createCheck).not.toHaveBeenCalled();
  });

  it('rejects an inverted status range', async () => {
    render(CheckDialog, { open: true, snapshot, resourceId: 'res1' });
    const form = dialog();
    await userEvent.type(form.getByLabelText('Name'), 'API');
    await userEvent.type(form.getByLabelText('URL'), 'https://a.example');
    await userEvent.clear(form.getByLabelText('Status from'));
    await userEvent.type(form.getByLabelText('Status from'), '500');
    await userEvent.clear(form.getByLabelText('Status to'));
    await userEvent.type(form.getByLabelText('Status to'), '200');
    await userEvent.click(form.getByRole('button', { name: 'Create check' }));
    expect(form.getByText(/valid status range/)).toBeTruthy();
    expect(checksApi.createCheck).not.toHaveBeenCalled();
  });

  it('creates a check with the resource chosen in the combobox', async () => {
    vi.mocked(checksApi.createCheck).mockResolvedValue({
      ...existing,
      id: 'new',
      name: 'Public health',
    });
    const onsaved = vi.fn();
    render(CheckDialog, { open: true, snapshot, onsaved });
    const form = dialog();
    await userEvent.click(form.getByRole('combobox', { name: 'Resource' }));
    await userEvent.click(
      screen.getByRole('option', { name: /api\.production/ }),
    );
    await userEvent.type(form.getByLabelText('Name'), '  Public health ');
    await userEvent.type(
      form.getByLabelText('URL'),
      'https://example.com/health',
    );
    await userEvent.click(form.getByRole('button', { name: 'Create check' }));
    expect(checksApi.createCheck).toHaveBeenCalledWith({
      resourceId: 'res1',
      name: 'Public health',
      url: 'https://example.com/health',
      method: 'GET',
      intervalSeconds: 30,
      timeoutSeconds: 5,
      expectedStatusMin: 200,
      expectedStatusMax: 399,
      bodySubstring: '',
      required: true,
      enabled: true,
    });
    expect(onsaved).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'new' }),
    );
    expect(document.querySelector('dialog')?.open).toBe(false);
  });

  it('edits an existing check from its stored values', async () => {
    vi.mocked(checksApi.updateCheck).mockResolvedValue(existing);
    render(CheckDialog, { open: true, snapshot, check: existing });
    const form = dialog();
    expect(
      screen.getByRole('heading', { name: 'Edit health check' }),
    ).toBeTruthy();
    expect((form.getByLabelText('Name') as HTMLInputElement).value).toBe(
      'Worker health',
    );
    expect((form.getByLabelText('Every') as HTMLInputElement).value).toBe('60');
    await userEvent.clear(form.getByLabelText('Every'));
    await userEvent.type(form.getByLabelText('Every'), '120');
    await userEvent.click(form.getByRole('button', { name: 'Save check' }));
    expect(checksApi.updateCheck).toHaveBeenCalledWith(
      'check-1',
      expect.objectContaining({
        resourceId: 'res2',
        method: 'HEAD',
        intervalSeconds: 120,
        timeoutSeconds: 10,
        required: false,
      }),
    );
  });

  it('keeps the dialog open with the server error', async () => {
    vi.mocked(checksApi.createCheck).mockRejectedValue(
      new Error('The target is blocked by the deployment.'),
    );
    render(CheckDialog, { open: true, snapshot, resourceId: 'res1' });
    const form = dialog();
    await userEvent.type(form.getByLabelText('Name'), 'API');
    await userEvent.type(form.getByLabelText('URL'), 'https://10.0.0.1/');
    await userEvent.click(form.getByRole('button', { name: 'Create check' }));
    expect((await form.findByRole('alert')).textContent).toContain(
      'blocked by the deployment',
    );
    expect(document.querySelector('dialog')?.open).toBe(true);
  });
});
