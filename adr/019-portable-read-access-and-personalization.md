# ADR 019: Portable read access and bounded personalization

Status: Accepted for v0.6

## Decision

Binnacle exposes portable monitoring data through narrowly scoped, read-only
personal API tokens. Tokens declare one or more fixed scopes, are stored only
as SHA-256 hashes, and never inherit session-only capabilities. An invalid
Bearer credential never falls back to a browser session. Settings, mutations,
diagnostics, logs, processes, live streams, notification configuration, and
token management remain browser-session only.

Exports reuse the existing metrics, event, incident, and resource contracts.
They are bounded by time, row count, and response size and fail explicitly
rather than silently truncating. The optional Prometheus endpoint emits current
bounded state directly, is disabled by default, and requires `metrics:read`.
It does not expose domain names, secrets, or unbounded labels.

Administrator preferences are typed, versioned server state. Theme and density
retain a local mirror to avoid startup flashing; older local values are imported
once only when no server preference exists. Personalization is limited to the
documented landing page, chart range, density, theme, and twelve ordered pinned
resources.

## Consequences

External consumers receive stable, least-privilege read access without turning
Binnacle into an automation control plane. Token revocation is immediate, while
last-used timestamps are deliberately throttled to limit write amplification.
Export and Prometheus bounds protect the embedded database and monitoring loop
at the cost of requiring consumers to request smaller windows when limits are
exceeded.

No dashboard builder, arbitrary queries, widgets, or drag-and-drop layout are
introduced. SQLite portability remains an operational backup concern: a
consistent stopped-service copy or SQLite online backup is required in WAL
mode.
