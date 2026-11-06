# Interoperability, exports, and personalization

Personal API tokens, attachment exports, and Prometheus eligibility are enabled
in the packaged Compose and Coolify defaults (`BINNACLE_FEATURE_PORTABILITY=true`)
after passing the portability acceptance suite. A standalone binary still
defaults to off; set the variable explicitly there. Prometheus additionally
needs `BINNACLE_PROMETHEUS_ENABLED=true`. Existing token data is
preserved while the gate is off, token-management and export routes return 404,
and Bearer credentials do not authorize normal read endpoints. Personalization
remains available independently.

## Personal API tokens

Create and revoke tokens in **Settings → Integrations**. The plaintext is
shown once; Binnacle stores only its identifier, prefix, and SHA-256 hash. A
token may have only these read scopes: `server:read`, `resources:read`,
`metrics:read`, `events:read`, and `incidents:read`.

Tokens do not authorize mutations, settings, logs, processes, diagnostics,
live SSE, notification configuration, preferences, or token management. A
request that carries an `Authorization` header is judged by that header alone:
a missing, revoked or malformed token is `401 invalid_token` and a token
without the route's scope is `403 insufficient_scope`, even when the same
request also carries a valid session cookie. Treat
them as credentials, transmit them only over HTTPS, give each consumer its own
token, set an expiry where practical, and revoke unused tokens.

```bash
curl -H "Authorization: Bearer $BINNACLE_TOKEN" \
  https://binnacle.example/api/v1/resources
```

Resources returned by `/api/v1/resources` and the live stream carry a
boolean `stale` (additive in v1.0). It is `true` when the Docker collector
failed its last tick: the resource is the last-known one, its metrics are
`null`, its status is `unknown` and `lastSeenAt` is the time it was last
measured. The Prometheus exporter omits those metrics.

Event `id` is a string: a ULID that stays unique across restarts and is the
same in the live stream, `/api/v1/events` and the exports (ADR 021 §9; events
stored by earlier versions keep their numeric-looking ids). Live event
payloads also carry `seq`, the per-process stream sequence. The SSE `id:`
field of an event frame is that sequence, and of a snapshot frame the highest
sequence sent so far, so ids never go backwards and `Last-Event-ID` resumes
events missed since it; it restarts with the process. A client that opens a
new stream instead of letting the browser retry, and so cannot set the header,
passes the same id as `?lastEventId=`; the header wins when both are sent. When every live slot is
taken (`live.max_clients`, default 8) the stream answers `503` with
`Retry-After` and error code `live_capacity`. `/api/v1/events` pages with
`before=<id>`, ordered newest first by time and then by storage order.

History responses list `gaps` with a `reason`: `host_reboot` (a
`host_reboot` event falls inside the gap), `persistence_failure` (history
batches were not stored: queue overflow, the storage emergency pause or a
failed write, each recorded as a `persistence_gap` event),
`collector_unavailable`, `inactive` (resources only) or `missing`. The
resolution is the finest tier whose retention still covers the range start,
never finer than the range length selects. Monitor health reports
`dropped` batches and `events_dropped`, events shown live but never stored.

`/api/v1/resources?state=archived` lists archived resources newest first and
is paginated like the event history: `limit` is 1 to 500 (default 100) and
`before=<id>` returns the resources archived before that one. A page shorter
than `limit` is the last. `/api/v1/exports/resources.json` walks every page
for you.

## Attachment exports

| Endpoint | Required scope |
| --- | --- |
| `/api/v1/exports/metrics.csv` | `metrics:read` |
| `/api/v1/exports/events.json` | `events:read` |
| `/api/v1/exports/incidents.json` | `incidents:read` |
| `/api/v1/exports/resources.json` | `resources:read` |

Metrics, events, and incidents require RFC 3339 `from` and `to` values. Metrics
also requires the existing `scope`, `metrics`, and optional resource `id`
parameters. Exports use UTC, fixed safe filenames, explicit schema metadata,
and `Cache-Control: no-store`. Requests are limited to 30 days, 10,000 rows,
and 16 MiB. Binnacle returns an error when a bound would be exceeded; it does
not silently produce an incomplete export. Incident exports include alert
membership but exclude notification configuration and delivery secrets.

## Prometheus

The root `/metrics` endpoint is disabled by default. Enable it at deployment:

```yaml
environment:
  BINNACLE_FEATURE_PORTABILITY: "true"
  BINNACLE_PROMETHEUS_ENABLED: "true"
```

Scrapes require a personal token with `metrics:read`:

```yaml
scrape_configs:
  - job_name: binnacle
    authorization:
      credentials: YOUR_BINNACLE_TOKEN
    static_configs:
      - targets: [binnacle:8080]
```

Both settings are required. With portability on and Prometheus off,
`/metrics` returns 404; Prometheus on with portability off is a configuration
error, and Binnacle refuses to start. A scrape authenticates only with the
`Authorization: Bearer` header: a browser session cookie is not accepted, and
a token without `metrics:read` gets `403 insufficient_scope`.
The exporter emits only current bounded
host/resource metrics, health-check state, collector state, and Binnacle
self-metrics. Unavailable values are omitted. Labels exclude domains and
secrets; cardinality remains bounded by current resources and checks.

Worker health is exported as `binnacle_self_failures_total{operation="…"}`,
failures since start, and `binnacle_self_latency_milliseconds{operation="…",
quantile="0.5|0.95|1"}`, the p50, p95 and max of the latest 256 runs
(`quantile="1"` is the max). The operations are those listed under
"Monitoring Binnacle itself" in [recovery](operations/recovery.md).

## Safe SQLite export

For a portable database backup, use one of these supported approaches:

1. Stop Binnacle, then copy `binnacle.db` after the process has exited.
2. Keep Binnacle running and use SQLite's online backup mechanism, for example
   `sqlite3 /var/lib/binnacle/binnacle.db ".backup '/backup/binnacle.db'"`
   from a context with access to the volume.

Do **not** copy only the live main database file while WAL mode is active.
Committed data may still be in `binnacle.db-wal`, producing an incomplete or
inconsistent copy. If filesystem snapshotting is used, capture the database,
WAL, and SHM files atomically.

## Personalization

Theme, density, default landing page, default chart range, and up to twelve
ordered pinned resources are stored for the administrator. Existing local
theme and density values are imported once if server preferences do not yet
exist. A local mirror prevents theme flashing during startup. Missing or
archived pins are ignored; all remaining resources retain their health-first
ordering.
