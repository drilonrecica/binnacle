// Generated from internal/metrics/eventtypes.go; do not edit by hand.
// Regenerate with: BINNACLE_UPDATE_GENERATED=1 go test ./internal/metrics -run TestWebEventTypesAreCurrent

/** Every event type the backend writes to the events table. */
export const eventTypes = [
  'container_create',
  'container_start',
  'container_stop',
  'container_die',
  'container_destroy',
  'container_rename',
  'container_oom',
  'container_restart',
  'container_health_status_change',
  'deployment',
  'host_reboot',
  'collector_degraded',
  'persistence_gap',
  'persistence_emergency',
  'persistence_resumed',
  'retention_shortened',
  'alert_triggered',
  'alert_repeated',
  'alert_resolved',
] as const;

export type EventType = (typeof eventTypes)[number];

/** The event types drawn as markers on metric charts. */
export const annotatedEventTypes: readonly EventType[] = [
  'container_start',
  'container_stop',
  'container_die',
  'container_oom',
  'container_restart',
  'deployment',
  'host_reboot',
  'collector_degraded',
  'persistence_gap',
  'persistence_emergency',
  'persistence_resumed',
  'retention_shortened',
];
