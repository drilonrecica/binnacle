import { api } from './client';
import type { LiveSnapshot } from '../live.svelte';

export type LiveResource = LiveSnapshot['resources'][number];
export type LiveComponent = NonNullable<LiveResource['components']>[number];

export interface ArchivedResource {
  id: string;
  sourceKind?: string;
  name: string;
  context?: string;
  category: string;
  status: string;
  project?: string;
  environment?: string;
  archivedAt?: string;
}

export function listResources(signal?: AbortSignal) {
  return api.get<LiveResource[]>('/api/v1/resources', {
    signal,
    fallback: 'Resources are unavailable.',
  });
}

/** Page size of the archived list; the server accepts 1 to 500. */
export const ARCHIVED_PAGE_SIZE = 100;

/**
 * One page of archived resources, newest first. Pass the id of the last row
 * of the previous page as `before` to fetch the next one; a page shorter than
 * ARCHIVED_PAGE_SIZE is the last.
 */
export function listArchivedResources(
  options: { before?: string; signal?: AbortSignal } = {},
) {
  const query: Record<string, string> = {
    state: 'archived',
    limit: String(ARCHIVED_PAGE_SIZE),
  };
  if (options.before) query.before = options.before;
  return api.get<ArchivedResource[]>('/api/v1/resources', {
    query,
    signal: options.signal,
    fallback: 'Archived resources are unavailable.',
  });
}

export function getResource(id: string, signal?: AbortSignal) {
  return api.get<LiveResource | ArchivedResource>(
    `/api/v1/resources/${encodeURIComponent(id)}`,
    { signal, fallback: 'The resource is unavailable.' },
  );
}

/** Human-readable grouping key: "shop / production", "Infrastructure", or "Ungrouped". */
export function resourceGroup(resource: {
  project?: string;
  environment?: string;
  infrastructure?: boolean;
}): string {
  if (resource.infrastructure) return 'Infrastructure';
  const parts = [resource.project, resource.environment].filter(Boolean);
  return parts.length ? parts.join(' / ') : 'Ungrouped';
}

export function categoryLabel(category: string | undefined): string {
  switch (category) {
    case 'application':
      return 'App';
    case 'database':
      return 'Database';
    case 'cache':
      return 'Cache';
    case 'worker':
      return 'Worker';
    case 'proxy':
      return 'Proxy';
    case 'infrastructure':
      return 'Infra';
    case 'unmanaged':
      return 'Unmanaged';
    case 'service':
      return 'Service';
    default:
      return category ?? 'Service';
  }
}
