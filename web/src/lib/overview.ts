import type { LiveSnapshot } from './live.svelte';

const rank: Record<string, number> = {
  down: 5,
  degraded: 4,
  unknown: 3,
  paused: 2,
  healthy: 1,
};

export function prioritizedResources(resources: LiveSnapshot['resources']) {
  return [...resources].sort(
    (left, right) =>
      (rank[right.status] ?? 0) - (rank[left.status] ?? 0) ||
      left.name.localeCompare(right.name),
  );
}

export function staleResource(
  resource: LiveSnapshot['resources'][number],
  snapshotAt: string,
  thresholdSeconds = 10,
) {
  if (!resource.lastSeenAt) return true;
  return (
    new Date(snapshotAt).getTime() - new Date(resource.lastSeenAt).getTime() >
    thresholdSeconds * 1000
  );
}
