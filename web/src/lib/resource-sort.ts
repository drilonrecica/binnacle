export type SortDirection = 'asc' | 'desc';
export type ActiveSortField =
  'name' | 'state' | 'cpu' | 'memory' | 'context' | 'components';
export type ArchivedSortField = 'name' | 'context' | 'archived';
export type ResourceSortField = ActiveSortField | ArchivedSortField;

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

export interface SortPreference<F extends ResourceSortField> {
  field: F;
  direction: SortDirection;
}

const activeFields: ActiveSortField[] = [
  'name',
  'state',
  'cpu',
  'memory',
  'context',
  'components',
];
const archivedFields: ArchivedSortField[] = ['name', 'context', 'archived'];
const numericFields = new Set<ResourceSortField>([
  'cpu',
  'memory',
  'components',
  'archived',
]);

export function resourceContext(resource: SortableResource): string {
  if (resource.context) return resource.context;
  return [resource.project ?? resource.category ?? '', resource.environment]
    .filter(Boolean)
    .join('/');
}

export function defaultDirection(field: ResourceSortField): SortDirection {
  return numericFields.has(field) ? 'desc' : 'asc';
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

export function sortResources<T extends SortableResource>(
  resources: readonly T[],
  preference: SortPreference<ResourceSortField>,
): T[] {
  return [...resources].sort((left, right) => {
    const leftValue = sortValue(left, preference.field);
    const rightValue = sortValue(right, preference.field);
    const missing = compareMissing(leftValue, rightValue);
    if (missing !== 0) return missing;

    const compared =
      typeof leftValue === 'number' && typeof rightValue === 'number'
        ? leftValue - rightValue
        : String(leftValue).localeCompare(String(rightValue), undefined, {
            sensitivity: 'base',
            numeric: true,
          });
    if (compared !== 0)
      return preference.direction === 'asc' ? compared : -compared;
    return (
      left.name.localeCompare(right.name, undefined, {
        sensitivity: 'base',
        numeric: true,
      }) || left.id.localeCompare(right.id)
    );
  });
}

export function loadSortPreference(
  view: 'active',
  storage?: Storage,
): SortPreference<ActiveSortField>;
export function loadSortPreference(
  view: 'archived',
  storage?: Storage,
): SortPreference<ArchivedSortField>;
export function loadSortPreference(
  view: 'active' | 'archived',
  storage: Storage = localStorage,
): SortPreference<ActiveSortField> | SortPreference<ArchivedSortField> {
  const fields = view === 'active' ? activeFields : archivedFields;
  const fallback = { field: 'name', direction: 'asc' } as const;
  try {
    const parsed = JSON.parse(
      storage.getItem(`binnacle.resources.sort.${view}`) ?? 'null',
    ) as Partial<SortPreference<ResourceSortField>> | null;
    if (
      parsed &&
      fields.includes(parsed.field as never) &&
      (parsed.direction === 'asc' || parsed.direction === 'desc')
    )
      return parsed as
        SortPreference<ActiveSortField> | SortPreference<ArchivedSortField>;
  } catch {
    // Ignore malformed user preferences.
  }
  return fallback;
}

export function saveSortPreference(
  view: 'active' | 'archived',
  preference: SortPreference<ResourceSortField>,
  storage: Storage = localStorage,
) {
  storage.setItem(
    `binnacle.resources.sort.${view}`,
    JSON.stringify(preference),
  );
}

function sortValue(
  resource: SortableResource,
  field: ResourceSortField,
): string | number | null {
  switch (field) {
    case 'name':
      return resource.name || null;
    case 'state':
      return resourceStatusLabel(resource) || null;
    case 'cpu':
      return finite(resource.cpuHostPct);
    case 'memory':
      return finite(resource.memoryBytes);
    case 'context':
      return resourceContext(resource) || null;
    case 'components':
      return resource.components?.length ?? null;
    case 'archived': {
      if (!resource.archivedAt) return null;
      const value = Date.parse(resource.archivedAt);
      return Number.isFinite(value) ? value : null;
    }
  }
}

function finite(value: number | null | undefined): number | null {
  return value != null && Number.isFinite(value) ? value : null;
}

function compareMissing(
  left: string | number | null,
  right: string | number | null,
): number {
  if (left == null && right == null) return 0;
  if (left == null) return 1;
  if (right == null) return -1;
  return 0;
}
