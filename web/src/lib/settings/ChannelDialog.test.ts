// @vitest-environment jsdom
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import type { Channel } from '../api/channels';
import { field, polyfillDom } from '../ui/testing';

vi.mock('../api/channels', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/channels')>()),
  createChannel: vi.fn(),
  updateChannel: vi.fn(),
}));

const { default: ChannelDialog } = await import('./ChannelDialog.svelte');
const channelsApi = await import('../api/channels');
const { ApiError } = await import('../api/client');

beforeAll(polyfillDom);
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const saved: Channel = {
  id: 'channel-1',
  name: 'Ops',
  kind: 'webhook',
  enabled: true,
  minimumSeverity: 'warning',
  notifyResolved: true,
  config: { url: 'https://hooks.example.com/ops' },
  secretConfigured: true,
  createdAt: '2026-07-10T12:00:00Z',
  updatedAt: '2026-07-10T12:00:00Z',
};

function form() {
  const element = screen.getByRole('dialog');
  return {
    element,
    field: (text: string) => field(element, text),
    button: (name: string) => within(element).getByRole('button', { name }),
  };
}

describe('ChannelDialog', () => {
  it('needs a name and an https webhook', async () => {
    render(ChannelDialog, { open: true });
    const f = form();
    await userEvent.click(f.button('Create channel'));
    expect(within(f.element).getByRole('alert').textContent).toContain(
      'Give the channel a name.',
    );
    await userEvent.type(f.field('Name'), 'Ops');
    await userEvent.type(f.field('Webhook URL'), 'http://hooks.example.com');
    await userEvent.click(f.button('Create channel'));
    expect(within(f.element).getByRole('alert').textContent).toContain(
      'https:// URL',
    );
    expect(channelsApi.createChannel).not.toHaveBeenCalled();
  });

  it('creates a webhook and sends secrets only when typed', async () => {
    vi.mocked(channelsApi.createChannel).mockResolvedValue(saved);
    const onsaved = vi.fn();
    render(ChannelDialog, { open: true, onsaved });
    const f = form();
    await userEvent.type(f.field('Name'), ' Ops ');
    await userEvent.type(
      f.field('Webhook URL'),
      'https://hooks.example.com/ops',
    );
    await userEvent.type(f.field('Signing secret'), 's3cret');
    await userEvent.click(f.button('Create channel'));
    expect(channelsApi.createChannel).toHaveBeenCalledWith({
      name: 'Ops',
      kind: 'webhook',
      enabled: true,
      minimumSeverity: 'warning',
      notifyResolved: true,
      url: 'https://hooks.example.com/ops',
      signingSecret: 's3cret',
    });
    expect(onsaved).toHaveBeenCalledWith(saved);
  });

  it('switches to SMTP fields and splits the recipients', async () => {
    vi.mocked(channelsApi.createChannel).mockResolvedValue({
      ...saved,
      kind: 'smtp',
    });
    render(ChannelDialog, { open: true });
    const f = form();
    await userEvent.type(f.field('Name'), 'Mail');
    await userEvent.selectOptions(f.field('Type'), 'smtp');
    expect(within(f.element).queryByLabelText(/^Webhook URL/)).toBeNull();
    await userEvent.type(f.field('SMTP host and port'), 'smtp.example.com:465');
    await userEvent.type(f.field('Sender'), 'binnacle@example.com');
    await userEvent.click(f.button('Create channel'));
    expect(within(f.element).getByRole('alert').textContent).toContain(
      'at least one recipient',
    );
    await userEvent.type(
      f.field('Recipients'),
      'ops@example.com, , oncall@example.com',
    );
    await userEvent.selectOptions(f.field('TLS'), 'implicit');
    await userEvent.click(f.button('Create channel'));
    expect(channelsApi.createChannel).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: 'smtp',
        host: 'smtp.example.com:465',
        sender: 'binnacle@example.com',
        recipients: ['ops@example.com', 'oncall@example.com'],
        tlsMode: 'implicit',
      }),
    );
  });

  it('edits a channel without resending its stored secret', async () => {
    vi.mocked(channelsApi.updateChannel).mockResolvedValue(saved);
    render(ChannelDialog, { open: true, channel: saved });
    const f = form();
    expect((f.field('Webhook URL') as HTMLInputElement).value).toBe(
      'https://hooks.example.com/ops',
    );
    await userEvent.click(f.button('Save channel'));
    const [, input] = vi.mocked(channelsApi.updateChannel).mock.calls[0]!;
    expect(input).not.toHaveProperty('signingSecret');
    expect(input).not.toHaveProperty('bearerToken');
  });

  it('explains a missing master key instead of a raw error', async () => {
    vi.mocked(channelsApi.createChannel).mockRejectedValue(
      new ApiError('no key', 409, 'master_key_missing'),
    );
    render(ChannelDialog, { open: true });
    const f = form();
    await userEvent.type(f.field('Name'), 'Ops');
    await userEvent.type(f.field('Webhook URL'), 'https://h.example.com');
    await userEvent.click(f.button('Create channel'));
    expect(f.element.textContent).toMatch(/master key/i);
    expect((f.element as HTMLDialogElement).open).toBe(true);
  });
});
