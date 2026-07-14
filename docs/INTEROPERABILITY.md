# Interoperability, exports, and personalization

## Personal API tokens

Create and revoke tokens in **Settings → Authentication**. The plaintext is
shown once; Binnacle stores only its identifier, prefix, and SHA-256 hash. A
token may have only these read scopes: `server:read`, `resources:read`,
`metrics:read`, `events:read`, and `incidents:read`.

Tokens do not authorize mutations, settings, logs, processes, diagnostics,
live SSE, notification configuration, preferences, or token management. Treat
them as credentials, transmit them only over HTTPS, give each consumer its own
token, set an expiry where practical, and revoke unused tokens.

```bash
curl -H "Authorization: Bearer $BINNACLE_TOKEN" \
  https://binnacle.example/api/v1/resources
```

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

When disabled, `/metrics` returns 404. The exporter emits only current bounded
host/resource metrics, health-check state, collector state, and Binnacle
self-metrics. Unavailable values are omitted. Labels exclude domains and
secrets; cardinality remains bounded by current resources and checks.

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
