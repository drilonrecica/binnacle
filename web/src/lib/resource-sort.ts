/** Presentation helpers for a resource's state. */

export interface SortableResource {
  id: string;
  name: string;
  status?: string;
  context?: string;
  project?: string;
  environment?: string;
  category?: string;
  cpuHostPct?: number | null;
  memoryBytes?: number | null;
  components?: unknown[];
  archivedAt?: string;
}

export function resourceStatusLabel(resource: SortableResource): string {
  if (
    resource.status === 'unknown' &&
    resource.components?.some(
      (component) =>
        (component as { healthStatus?: string }).healthStatus === 'starting',
    )
  )
    return 'starting';
  return resource.status ?? 'unknown';
}
