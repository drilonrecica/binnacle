import { api } from './client';
import type { Delivery } from './incidents';

export type ChannelKind = 'webhook' | 'smtp';

export interface Channel {
  id: string;
  name: string;
  kind: ChannelKind;
  enabled: boolean;
  minimumSeverity: 'warning' | 'critical';
  notifyResolved: boolean;
  config: {
    url?: string;
    host?: string;
    sender?: string;
    recipients?: string[];
    tlsMode?: 'starttls' | 'implicit';
    username?: string;
  };
  secretConfigured: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface ChannelInput {
  name: string;
  kind: ChannelKind;
  enabled: boolean;
  minimumSeverity: 'warning' | 'critical';
  notifyResolved: boolean;
  url?: string;
  bearerToken?: string;
  signingSecret?: string;
  host?: string;
  username?: string;
  password?: string;
  sender?: string;
  recipients?: string[];
  tlsMode?: 'starttls' | 'implicit';
}

export function listChannels(signal?: AbortSignal) {
  return api.get<Channel[]>('/api/v1/notification-channels', {
    signal,
    fallback: 'Notification channels are unavailable.',
  });
}

export function createChannel(input: ChannelInput) {
  return api.post<Channel>('/api/v1/notification-channels', input, {
    fallback: 'The channel could not be created.',
  });
}

export function updateChannel(id: string, input: Partial<ChannelInput>) {
  return api.patch<Channel>(
    `/api/v1/notification-channels/${encodeURIComponent(id)}`,
    input,
    {
      fallback: 'The channel could not be updated.',
    },
  );
}

export function deleteChannel(id: string) {
  return api.delete(`/api/v1/notification-channels/${encodeURIComponent(id)}`, {
    fallback: 'The channel could not be deleted.',
  });
}

export function testChannel(id: string) {
  return api.post<{ deliveryId: string }>(
    `/api/v1/notification-channels/${encodeURIComponent(id)}/test`,
    undefined,
    {
      fallback: 'The test could not be sent.',
    },
  );
}

export function listDeliveries(
  query: { incidentId?: string; limit?: number; offset?: number } = {},
  signal?: AbortSignal,
) {
  return api.get<Delivery[]>('/api/v1/notification-deliveries', {
    query,
    signal,
    fallback: 'Delivery history is unavailable.',
  });
}

export function retryDelivery(id: string) {
  return api.post<{ deliveryId: string }>(
    `/api/v1/notification-deliveries/${encodeURIComponent(id)}/retry`,
    undefined,
    {
      fallback: 'The delivery could not be retried.',
    },
  );
}

export function channelTarget(channel: Channel): string {
  if (channel.kind === 'smtp') {
    const recipients = channel.config.recipients ?? [];
    return recipients.length
      ? `${channel.config.host ?? ''} → ${recipients.join(', ')}`
      : (channel.config.host ?? '');
  }
  return channel.config.url ?? '';
}

export function deliveryStatusLabel(status: Delivery['status']): string {
  switch (status) {
    case 'succeeded':
      return 'Delivered';
    case 'pending':
      return 'Queued';
    case 'in_progress':
      return 'Sending';
    case 'permanent_failure':
      return 'Failed';
    case 'cancelled':
      return 'Cancelled';
    default:
      return status;
  }
}
