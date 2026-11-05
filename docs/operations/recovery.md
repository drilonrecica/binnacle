# Recovery

Notification deliveries left in progress by a crash return to pending on
startup with the same idempotency key. Use Delivery History for manual retry so
the audit record remains intact.

## Database budget and disk-full condition

The budget (`database.target_budget_bytes`, 1 GiB by default) is compared
against live data every minute: `(page_count - freelist_count) x page_size`,
read with pragmas on a read-only connection (ADR 021 §7). The WAL and shm
files are not counted; they are bounded separately by a 64 MiB
`journal_size_limit` and by the `wal_checkpoint(TRUNCATE)` that ends every
retention pass. When the free space on the database's filesystem drops under
64 MiB the state is emergency regardless of the budget, because SQLite needs
disk to recover.

The thresholds are fixed at 80 % (warning), 95 % (critical) and 98 % (emergency)
of the target; they are not configurable:

- **Warning** — reported only.
- **Critical** — retention passes delete in larger batches (2,000 rows).
- **Emergency** — the retention pass shortens retention instead of only
  pausing: it deletes the oldest raw samples down to half the configured raw
  retention, reclaims the space and re-measures; if still at emergency it
  deletes the oldest 1 m rollups down to half their retention and re-measures;
  only if that is not enough does raw 10-second persistence pause. The 15 m
  and 1 h tiers are never shortened. The shortening is logged, recorded as a
  `retention_shortened` event and shown in Settings › System as
  `retention_shortened`.

The pause is recorded as a `persistence_emergency` event and shown as
`persistence_paused`. It clears once usage falls below the **critical**
threshold, not merely below emergency, so the state cannot flap; the
`persistence_resumed` event marks the transition. No restart is required.
Rollups, settings and events are always preserved, and the live Metrics
Engine and SSE keep working during storage pressure.

## Space reclamation

Databases use SQLite's incremental auto-vacuum. After the deletes of each
retention pass Binnacle runs `PRAGMA incremental_vacuum` in batches of 1,000
pages (about 4 MiB) with a pause between batches, until the freelist is under
10 % of the file or 25,000 pages (about 100 MiB) were returned in that pass,
then truncates the WAL. A full `VACUUM` never runs on a schedule.

A database created before v1.0 uses `auto_vacuum=none`. At the first start
after the upgrade Binnacle converts it once: when free disk space is at least
the database size plus 64 MiB it runs `VACUUM` before the workers start and
logs the duration; otherwise it logs a warning, reports `vacuum_pending` in
`/api/v1/monitor-health`, keeps running without reclaiming space, and retries
at the next start. Free the disk and restart to complete the conversion.

## Query planner statistics

Two minutes after boot and then every six hours Binnacle runs
`PRAGMA optimize` on its write connection, with `analysis_limit=400` so a pass
reads a bounded sample of each index whatever the database size. SQLite only
analyzes tables that were never analyzed or whose row count changed
substantially since; the statistics in `sqlite_stat1` let the planner keep
choosing index searches for history, sparkline and event queries as the data
grows. A failed pass is logged as a
warning and retried at the next interval.

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

Retention runs one minute after boot, then hourly, and immediately when the
retention settings change, so a process that is restarted often still prunes
(ADR 021 §5). Deletes run in batches of 500 rows (2,000 when the budget is
critical or worse) with a short pause between batches so persistence keeps
flowing. A pass has a 30-second budget; when it runs out the pass stops,
`retention_incomplete` is reported, and the next pass resumes the work.

Besides the sample and rollup tables, retention prunes (ADR 021 §6):

| Table | Rule |
| --- | --- |
| Container instances | Instances whose stop or destruction time is older than the 1 m retention. Running instances are never deleted. |
| Archived resources | Archived before the 1 h retention cutoff and referenced by no check, incident, sample, rollup or event; their container instance rows go with them. When the 1 h tier is unlimited, at most 1,000 archived resources are kept: the oldest beyond that is handed to the history deletion job (visible in Settings), one per pass, so its history is removed with it. Resources with a check or an incident are never removed automatically. |
| Coolify deployments | Rows not updated within the raw retention. |
| Alert evaluation state | Targets not evaluated within the raw retention. |

`/api/v1/monitor-health` reports `rollup_duration`, `retention_duration` and
`vacuum_duration`, the wall-clock time of the latest pass of each kind; each
is `unavailable` until the first pass completes. `database_budget` is the live
data measured against the target, and `persistence_paused`,
`retention_shortened`, `retention_incomplete` and `vacuum_pending` carry the
states described above. A failed pass is logged at `level=ERROR`, counted (see
below) and retried on the next tick.

## Monitoring Binnacle itself

Every background worker (persistence, rollups, retention and vacuum, the
history deletion job, the host and Docker collectors, checks, alert
evaluation, notification delivery and Coolify sync) logs its errors as JSON
through the application log on stderr. A worker that keeps failing would
repeat the same error every tick, so each message is written at most once a
minute; the next one carries `suppressed=N`, the number of identical messages
left out in between. The counters below still count every failure.

`/api/v1/monitor-health` and Settings → System add two sections to the
metrics above:

| Section | Entries | Meaning |
| --- | --- | --- |
| `failures` | `persistence_write`, `rollup`, `retention`, `vacuum`, `docker`, `checks`, `checks_skipped`, `deliveries` | Failures since the process started. Any failure marks the entry `warning`; the log has the cause. A check whose target is down is a result, not a `checks` failure. `docker` leaves out calls cancelled by shutdown and is absent in demo mode, like `checks`. |
| `latency` | `persistence_write`, `rollup`, `retention`, `vacuum` | p50, p95 and max in milliseconds over the latest 256 runs, plus `count`, the runs since start. `warning` when p95 exceeds 50 ms (write), 500 ms (rollup) or 2 s (retention, vacuum); `null` and `unavailable` before the first run. |

The windows cover recent runs only, so a slow hour long ago does not hide a
healthy present, and memory stays fixed however long Binnacle runs. Counters
reset at restart; nothing is persisted or sent anywhere.

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
