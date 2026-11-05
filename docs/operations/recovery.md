# Recovery

Notification deliveries left in progress by a crash return to pending on
startup with the same idempotency key. Use Delivery History for manual retry so
the audit record remains intact.

## Disk-full condition

When the database or WAL grows past configured thresholds, Binnacle enters a degraded persistence state:

- **Warning** — expired data is cleaned aggressively.
- **Critical** — additional expired cleanup runs.
- **Emergency** — raw 10-second persistence pauses; rollups, settings, and events are preserved.

The live Metrics Engine and SSE continue to work during storage pressure. Free
disk space or reduce retention. Binnacle reevaluates the database budget every
minute and automatically resumes raw persistence after usage falls below the
emergency threshold; no restart is required.

## History rollups and retention

Rollups run every minute and are incremental (ADR 021 §4): each tier keeps a
persisted watermark, `closed_until`, and a pass aggregates raw samples only
from two minutes before that watermark (so a late persistence batch still
repairs its bucket) up to the start of the current bucket. The 1 h tier
therefore lags real time by up to 62 minutes; charts read raw or 1 m data for
recent ranges, so the lag is not visible. A pass that fell behind, for example
after the process was stopped for a day, catches up in chunks of six hours.

Raw retention deletes 10-second samples only below a cutoff that is aligned
down to a whole hour and never ahead of the 1 h tier's watermark, and it
records that cutoff as `raw_deleted_below` before the first delete. No bucket
below it is ever recomputed, so a 1 h bucket is either computed from the
complete hour or left untouched; the retention edge can no longer overwrite
long-term history with a partial aggregate. The other retention cutoffs are
aligned down to the hour as well.

`/api/v1/monitor-health` reports `rollup_duration` and `retention_duration`,
the wall-clock time of the latest pass of each worker; both are `unavailable`
until the first pass completes. A failed pass is logged at `level=ERROR` and
retried on the next tick.

## Database integrity

Binnacle opens SQLite through two pools (ADR 021): one write connection and
four read-only connections, each configured in its connection string so the
settings survive reconnects. Readers never block the writer and the writer
never blocks readers.

At boot Binnacle runs `PRAGMA quick_check`, which is bounded by the size of
the schema tree rather than by the whole file. The full `PRAGMA integrity_check`
runs only on demand: generating a diagnostics bundle executes it on a read-only
connection with a two-minute limit and reports the result in the bundle's
`databaseIntegrity` field (`ok`, or the problems SQLite found). When the check
does not finish within the limit the bundle lists it under partial failures.

If the previous process stopped without closing storage (a crash, SIGKILL or a
power loss), the next start logs a warning and `/api/v1/monitor-health` reports
`unclean_shutdown` with status `warning` until the next clean stop. SQLite's
own WAL recovery restores the last consistent state automatically; generate a
diagnostics bundle to run the full check if you want to verify the file.

## Corruption

If startup fails with a quick-check error, or a diagnostics bundle reports
integrity problems:

1. Stop the container.
2. Copy the database files to a safe location:

   ```bash
   docker cp binnacle:/var/lib/binnacle /tmp/binnacle-recovery
   ```

3. Attempt an integrity check on a copy:

   ```bash
   sqlite3 /tmp/binnacle-recovery/binnacle.db "PRAGMA integrity_check;"
   ```

4. If the database is corrupt, restore from your most recent backup or start with a fresh database. Binnacle does not automatically repair or delete a corrupt database.

## Consistent database backup

The production image does not include the SQLite CLI. Stop Binnacle before
copying the database so the process closes SQLite and checkpoints its WAL:

```bash
docker compose -f packaging/docker/docker-compose.yml stop binnacle
docker cp binnacle:/var/lib/binnacle/binnacle.db ./binnacle-backup.db
docker compose -f packaging/docker/docker-compose.yml start binnacle
```

Do not copy an open `binnacle.db` without its WAL. For an online backup, use a
trusted host-side SQLite tool and its backup API against the persistent volume.

## Reset monitoring history

From the Settings page you can delete history for one resource, data before a date, or all monitoring history. These operations require typed confirmation and run in bounded batches. They do not delete users or configuration.

## Restart and logs

```bash
docker compose -f packaging/docker/docker-compose.yml restart binnacle
docker compose -f packaging/docker/docker-compose.yml logs -f binnacle
```

Check `level=ERROR` entries for migration, disk, or collector failures.
