import { api } from './client';
import type { HistoricalEvent } from '../events';
import { eventTypes, type EventType } from '../event-types.generated';

export type { HistoricalEvent };

export interface EventQuery {
  from: Date;
  to: Date;
  resourceId?: string;
  severity?: string;
  /** Expanded into the family's types; takes precedence over `type`. */
  family?: EventFamily;
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
      type: query.family ? familyTypes(query.family).join(',') : query.type,
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
    retention_shortened: 'Retention shortened',
    alert_triggered: 'Alert triggered',
    alert_repeated: 'Alert repeated',
    alert_resolved: 'Alert resolved',
  };
  return labels[type] ?? type.replaceAll('_', ' ');
}

/**
 * The families the Changes page filters by (DESIGN §Changes). This is the one
 * family table: the history query expands a family into exactly these types
 * and the page matches live events against the same table, so a filter never
 * disagrees between live and history (UI-14).
 */
export const eventFamilies = [
  { value: 'deploys', label: 'Deploys' },
  { value: 'lifecycle', label: 'Lifecycle' },
  { value: 'health', label: 'Health' },
  { value: 'collectors', label: 'Collectors' },
  { value: 'alerts', label: 'Alerts' },
  { value: 'system', label: 'System' },
] as const;

export type EventFamily = (typeof eventFamilies)[number]['value'];

/**
 * Family of every type the backend writes. Keyed by the generated catalogue,
 * so a new backend type fails the type check until it is given a family.
 */
const familyOfType: Record<EventType, EventFamily> = {
  deployment: 'deploys',
  container_create: 'lifecycle',
  container_start: 'lifecycle',
  container_stop: 'lifecycle',
  container_die: 'lifecycle',
  container_destroy: 'lifecycle',
  container_rename: 'lifecycle',
  container_restart: 'lifecycle',
  container_oom: 'health',
  container_health_status_change: 'health',
  collector_degraded: 'collectors',
  alert_triggered: 'alerts',
  alert_repeated: 'alerts',
  alert_resolved: 'alerts',
  host_reboot: 'system',
  persistence_gap: 'system',
  persistence_emergency: 'system',
  persistence_resumed: 'system',
  retention_shortened: 'system',
};

export function isEventFamily(value: string): value is EventFamily {
  return eventFamilies.some((family) => family.value === value);
}

/** The family of an event type; undefined for a type outside the catalogue. */
export function eventFamily(type: string): EventFamily | undefined {
  return Object.hasOwn(familyOfType, type)
    ? familyOfType[type as EventType]
    : undefined;
}

/** The event types in one family, in catalogue order. */
export function familyTypes(family: EventFamily): EventType[] {
  return eventTypes.filter((type) => familyOfType[type] === family);
}
