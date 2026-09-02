import { api } from './client';
import type { HistoricalEvent } from '../events';

export type { HistoricalEvent };

export interface EventQuery {
  from: Date;
  to: Date;
  resourceId?: string;
  severity?: string;
  type?: string;
  limit?: number;
  before?: string;
}

export function listEvents(query: EventQuery, signal?: AbortSignal) {
  return api.get<HistoricalEvent[]>('/api/v1/events', {
    query: {
      from: query.from.toISOString(),
      to: query.to.toISOString(),
      resource_id: query.resourceId,
      severity: query.severity,
      type: query.type,
      limit: query.limit,
      before: query.before,
    },
    signal,
    fallback: 'Event history is unavailable.',
  });
}

/** Short human label for an event type such as `container_oom`. */
export function eventTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    container_start: 'Started',
    container_stop: 'Stopped',
    container_die: 'Exited',
    container_destroy: 'Removed',
    container_create: 'Created',
    container_rename: 'Renamed',
    container_oom: 'Out of memory',
    container_restart: 'Restarted',
    container_health_status_change: 'Health changed',
    deployment: 'Deployment',
    deployment_likely: 'Likely deployment',
    container_replacement: 'Replacement',
    resource_archived: 'Archived',
    collector_degraded: 'Collector degraded',
    collector_down: 'Collector down',
    persistence_degraded: 'Storage degraded',
    persistence_gap: 'Storage gap',
    persistence_emergency: 'Storage emergency',
    persistence_resumed: 'Storage resumed',
    host_reboot: 'Host reboot',
    alert_triggered: 'Alert triggered',
    alert_repeated: 'Alert repeated',
    alert_resolved: 'Alert resolved',
  };
  return labels[type] ?? type.replaceAll('_', ' ');
}

export type EventFamily =
  'containers' | 'deployments' | 'collectors' | 'storage' | 'alerts' | 'other';

export function eventFamily(type: string): EventFamily {
  if (type.startsWith('container_') || type === 'resource_archived')
    return 'containers';
  if (type.startsWith('deployment') || type === 'container_replacement')
    return 'deployments';
  if (type.startsWith('collector_')) return 'collectors';
  if (type.startsWith('persistence_')) return 'storage';
  if (type.startsWith('alert_')) return 'alerts';
  return 'other';
}
