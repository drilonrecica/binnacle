import { api } from './client';

export type IncidentStatus = 'open' | 'resolved';
export type Severity = 'warning' | 'critical';

export interface Alert {
  id: string;
  dedupKey?: string;
  ruleId?: string;
  family: string;
  severity: Severity;
  targetType: string;
  targetId: string;
  status: 'firing' | 'resolved';
  startedAt: string;
  resolvedAt?: string;
  lastObservedAt?: string;
  observedValue?: number | null;
  message: string;
  incidentId?: string;
}

export interface Delivery {
  id: string;
  channelId: string;
  incidentId?: string;
  eventType: string;
  status:
    'pending' | 'in_progress' | 'succeeded' | 'permanent_failure' | 'cancelled';
  attemptCount: number;
  failureCode?: string;
  nextAttemptAt?: string;
  lastAttemptAt?: string;
  createdAt?: string;
  updatedAt?: string;
  idempotencyKey?: string;
}

export interface Incident {
  id: string;
  title: string;
  status: IncidentStatus;
  severity: Severity;
  targetType: string;
  targetId: string;
  alertCount: number;
  firingAlertCount: number;
  openedAt: string;
  resolvedAt?: string;
  updatedAt?: string;
  alerts?: Alert[];
  deliveries?: Delivery[];
}

export function listIncidents(
  query: {
    status?: IncidentStatus;
    severity?: Severity;
    limit?: number;
    offset?: number;
  } = {},
  signal?: AbortSignal,
) {
  return api.get<Incident[]>('/api/v1/incidents', {
    query,
    signal,
    fallback: 'Incidents are unavailable.',
  });
}

export function getIncident(id: string, signal?: AbortSignal) {
  return api.get<Incident>(`/api/v1/incidents/${encodeURIComponent(id)}`, {
    signal,
    fallback: 'The incident is unavailable.',
  });
}

export function listAlerts(
  query: {
    status?: 'firing' | 'resolved';
    severity?: Severity;
    resource?: string;
    family?: string;
    limit?: number;
  } = {},
  signal?: AbortSignal,
) {
  return api.get<Alert[]>('/api/v1/alerts', {
    query,
    signal,
    fallback: 'Alerts are unavailable.',
  });
}

export function targetLabel(targetType: string, targetId: string): string {
  switch (targetType) {
    case 'host':
      return 'Host';
    case 'filesystem':
      return `Mount ${targetId}`;
    case 'subsystem':
      return targetId === 'docker'
        ? 'Docker collector'
        : targetId === 'persistence'
          ? 'Storage'
          : targetId;
    default:
      return targetId;
  }
}
