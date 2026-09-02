import type { Incident } from '../api/incidents';
import { targetLabel } from '../api/incidents';
import type { LiveSnapshot } from '../live.svelte';
import { incidentPath, resourcePath } from '../router';
import type { Tone } from '../ui/status';
import { staleResource } from '../watch';

export interface AttentionItem {
  id: string;
  tone: Tone;
  kind: 'incident' | 'resource' | 'collector' | 'filesystem' | 'stale';
  title: string;
  detail: string;
  href: string;
  since?: string;
}

/**
 * Builds the prioritized "needs attention" list for the Overview. Incidents
 * come first, then unhealthy resources without an incident, then collector
 * and filesystem pressure, then stale telemetry.
 */
export function attentionItems(
  snapshot: LiveSnapshot | null,
  incidents: Incident[],
): AttentionItem[] {
  const items: AttentionItem[] = [];
  const covered = new Set<string>();
  const names = new Map(
    (snapshot?.resources ?? []).map((resource) => [resource.id, resource.name]),
  );
  for (const incident of incidents.filter((item) => item.status === 'open')) {
    covered.add(incident.targetId);
    items.push({
      id: `incident:${incident.id}`,
      tone: incident.severity === 'critical' ? 'critical' : 'warn',
      kind: 'incident',
      title: incident.title,
      detail: `${names.get(incident.targetId) ?? targetLabel(incident.targetType, incident.targetId)} · ${incident.firingAlertCount} firing`,
      href: incidentPath(incident.id),
      since: incident.openedAt,
    });
  }
  if (!snapshot) return items;

  for (const resource of snapshot.resources) {
    if (covered.has(resource.id)) continue;
    if (resource.status === 'down' || resource.status === 'degraded') {
      items.push({
        id: `resource:${resource.id}`,
        tone: resource.status === 'down' ? 'critical' : 'warn',
        kind: 'resource',
        title: resource.name,
        detail:
          resource.status === 'down'
            ? 'Resource is down'
            : 'Resource is degraded',
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
      title: `${name} collector ${collector.state}`,
      detail:
        collector.reason ?? 'Telemetry from this collector may be incomplete.',
      href: '/host?tab=collectors',
    });
  }

  for (const mount of snapshot.filesystems ?? []) {
    const pct = mount.usedPct ?? null;
    if (pct == null || pct < 80) continue;
    items.push({
      id: `filesystem:${mount.mountKey}`,
      tone: pct >= 95 ? 'critical' : 'warn',
      kind: 'filesystem',
      title: `${mount.mountPoint} is ${Math.round(pct)}% full`,
      detail: 'Disk pressure on this mount',
      href: '/host?tab=filesystems',
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
          ? `${stale[0].name} has stale telemetry`
          : `${stale.length} resources have stale telemetry`,
      detail: 'No fresh samples in the last collection cycles',
      href: '/resources?filter=attention',
    });
  }
  return items;
}
