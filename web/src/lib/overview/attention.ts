import type { Incident } from '../api/incidents';
import type { ConnectionState, LiveSnapshot } from '../live.svelte';
import { staleAfterMs } from '../live.svelte';
import { incidentPath, resourcePath } from '../router';
import type { Tone } from '../ui/status';
import { staleResource } from '../watch';
import { fallbackThresholds, type GaugeThresholds } from './headroom';

export interface AttentionItem {
  id: string;
  tone: Tone;
  kind: 'incident' | 'resource' | 'collector' | 'filesystem' | 'stale';
  /** What it is about: a resource name, a mount, "Host". Never a raw id. */
  title: string;
  /** Why it needs attention. */
  detail: string;
  href: string;
  since?: string;
}

/** Display name of an incident's target, from the snapshot where it can. */
function targetName(incident: Incident, snapshot: LiveSnapshot | null): string {
  switch (incident.targetType) {
    case 'host':
      return 'Host';
    case 'filesystem': {
      const mount = snapshot?.filesystems?.find(
        (item) => item.mountKey === incident.targetId,
      );
      return mount?.mountPoint ?? 'A filesystem';
    }
    case 'subsystem':
      return incident.targetId === 'docker'
        ? 'Docker collector'
        : incident.targetId === 'persistence'
          ? 'Storage'
          : 'Binnacle';
    default:
      return (
        snapshot?.resources.find((item) => item.id === incident.targetId)
          ?.name ?? 'A removed resource'
      );
  }
}

/**
 * The "needs attention" chips for Headroom: open incidents first, then
 * resources that are down or degraded without an incident, then collector
 * trouble, disks over their critical line, and stale telemetry. Titles use
 * names, never ids (UI-18): stored incident titles carry the raw target id.
 */
export function attentionItems(
  snapshot: LiveSnapshot | null,
  incidents: Incident[],
  thresholds?: GaugeThresholds,
): AttentionItem[] {
  const items: AttentionItem[] = [];
  const covered = new Set<string>();
  for (const incident of incidents.filter((item) => item.status === 'open')) {
    covered.add(`${incident.targetType}:${incident.targetId}`);
    const severity = incident.severity === 'critical' ? 'Critical' : 'Warning';
    const firing = incident.firingAlertCount;
    items.push({
      id: `incident:${incident.id}`,
      tone: incident.severity === 'critical' ? 'critical' : 'warn',
      kind: 'incident',
      title: targetName(incident, snapshot),
      detail: `${severity} incident · ${firing} ${firing === 1 ? 'alert' : 'alerts'} firing`,
      href: incidentPath(incident.id),
      since: incident.openedAt,
    });
  }
  if (!snapshot) return items;

  for (const resource of snapshot.resources) {
    if (covered.has(`resource:${resource.id}`)) continue;
    if (resource.status === 'down' || resource.status === 'degraded') {
      items.push({
        id: `resource:${resource.id}`,
        tone: resource.status === 'down' ? 'critical' : 'warn',
        kind: 'resource',
        title: resource.name,
        detail: resource.status === 'down' ? 'Down' : 'Degraded',
        href: resourcePath(resource.id),
      });
    }
  }

  for (const [name, collector] of Object.entries(snapshot.collectors)) {
    if (collector.state === 'healthy') continue;
    items.push({
      id: `collector:${name}`,
      tone: collector.state === 'down' ? 'critical' : 'warn',
      kind: 'collector',
      title: `${name[0]?.toUpperCase() ?? ''}${name.slice(1)} collector`,
      detail:
        collector.reason ??
        (collector.state === 'down' ? 'Down' : `State ${collector.state}`),
      href: '/server?tab=collectors',
    });
  }

  for (const mount of snapshot.filesystems ?? []) {
    if (covered.has(`filesystem:${mount.mountKey}`)) continue;
    const pct = mount.usedPct ?? null;
    const limits =
      thresholds?.filesystem(mount.mountKey) ?? fallbackThresholds.filesystem;
    if (pct == null || pct < limits.critical) continue;
    items.push({
      id: `filesystem:${mount.mountKey}`,
      tone: 'critical',
      kind: 'filesystem',
      title: mount.mountPoint,
      detail: `${Math.round(pct)}% full`,
      href: `/server?tab=filesystems&mount=${encodeURIComponent(mount.mountKey)}`,
    });
  }

  const stale = snapshot.resources.filter(
    (resource) =>
      resource.status !== 'archived' && staleResource(resource, snapshot.ts),
  );
  if (stale.length) {
    items.push({
      id: 'stale',
      tone: 'neutral',
      kind: 'stale',
      title:
        stale.length === 1
          ? (stale[0]?.name ?? '1 resource')
          : `${stale.length} resources`,
      detail: 'No fresh samples',
      href: '/resources?filter=attention',
    });
  }
  return items;
}

export type AttentionState =
  | { kind: 'loading' }
  | { kind: 'clear' }
  | { kind: 'items'; items: AttentionItem[] }
  | {
      kind: 'unknown';
      /** When the status stopped being known. */
      since: string;
      /** The newest live snapshot, for "last update …". */
      lastUpdate: string | null;
      reason: 'stream' | 'incidents';
      /** What is still known for certain (live data while only incidents failed). */
      items: AttentionItem[];
    };

export interface AttentionInput {
  snapshot: LiveSnapshot | null;
  connection: ConnectionState;
  lastReceivedAt: string | null;
  incidents: Incident[] | null;
  /** When the incident list last loaded; null until it has. */
  incidentsLoadedAt: string | null;
  /** True while the latest incident fetch failed. */
  incidentsFailed: boolean;
  /** When the page started asking, for "since" before anything loaded. */
  openedAt: string;
  thresholds?: GaugeThresholds;
  now?: number;
}

/**
 * What the attention strip may say (UI-1). "Nothing needs attention" needs
 * live data and a loaded incident list; a stale stream or a failed incident
 * fetch turns the strip into "status unknown since …" instead.
 */
export function attentionState(input: AttentionInput): AttentionState {
  const now = input.now ?? Date.now();
  const last = input.lastReceivedAt ? Date.parse(input.lastReceivedAt) : NaN;
  const quiet = !Number.isFinite(last) || now - last > staleAfterMs;
  const streamLive = input.connection === 'live' && !quiet;
  if (!input.snapshot) return { kind: 'loading' };
  if (!streamLive)
    return {
      kind: 'unknown',
      reason: 'stream',
      since: input.lastReceivedAt ?? input.openedAt,
      lastUpdate: input.lastReceivedAt,
      items: [],
    };
  const known = attentionItems(
    input.snapshot,
    input.incidents ?? [],
    input.thresholds,
  );
  if (input.incidentsFailed)
    return {
      kind: 'unknown',
      reason: 'incidents',
      since: input.incidentsLoadedAt ?? input.openedAt,
      lastUpdate: input.lastReceivedAt,
      items: known,
    };
  if (input.incidents == null) return { kind: 'loading' };
  return known.length ? { kind: 'items', items: known } : { kind: 'clear' };
}
