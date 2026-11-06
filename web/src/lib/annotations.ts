import { annotatedEventTypes } from './event-types.generated';

export interface OperationalEvent {
  id?: string | number;
  ts: string;
  type: string;
  summary: string;
  resourceId?: string;
}
export interface ChartAnnotation {
  at: number;
  label: string;
  count: number;
  href: string;
}

// Generated from the backend's catalogue (internal/metrics/eventtypes.go), so
// markers listen for the types the server really writes (UI-11).
const applicable: ReadonlySet<string> = new Set(annotatedEventTypes);

/** The `type` filter for an events query that returns only marker events. */
export const annotationTypeFilter = annotatedEventTypes.join(',');

export function isAnnotatedEvent(type: string): boolean {
  return applicable.has(type);
}

export function boundAnnotations(
  events: OperationalEvent[],
  from: Date,
  to: Date,
  maximum = 12,
): ChartAnnotation[] {
  const filtered = events
    .filter((event) => applicable.has(event.type))
    .map((event) => ({ ...event, milliseconds: new Date(event.ts).getTime() }))
    .filter(
      (event) =>
        event.milliseconds >= from.getTime() &&
        event.milliseconds <= to.getTime(),
    )
    .sort((left, right) => left.milliseconds - right.milliseconds);
  if (!filtered.length) return [];
  const bucketWidth = Math.max(1, (to.getTime() - from.getTime()) / maximum);
  const buckets = new Map<number, typeof filtered>();
  for (const event of filtered) {
    const bucket = Math.min(
      maximum - 1,
      Math.floor((event.milliseconds - from.getTime()) / bucketWidth),
    );
    buckets.set(bucket, [...(buckets.get(bucket) ?? []), event]);
  }
  return [...buckets.values()].map((eventsInBucket) => {
    const first = eventsInBucket[0];
    return {
      at: first.milliseconds / 1000,
      label:
        eventsInBucket.length === 1
          ? first.summary
          : `${first.summary} and ${eventsInBucket.length - 1} more events`,
      count: eventsInBucket.length,
      href: first.resourceId ? `/resources/${first.resourceId}` : '/changes',
    };
  });
}
