# ADR 021: Storage architecture

## Status

Accepted (2026-11-05). Extends ADR 003 (typed SQLite storage) and ADR 011
(tiered retention and rollups); both remain in force. Implemented by the P3
storage tasks of the v1.0 plan.

## Context

The v1.0 audit found that the storage layer, although correct in its schema,
does not meet the bounded-overhead and explicit-degradation promises of
`docs/PRODUCT.md`:

- **One connection for everything** (STO-3). `internal/storage/sqlite.go`
  opens one `*sql.DB` with `MaxOpenConns(1)`. API reads, session updates,
  persistence batches, rollups, retention and `VACUUM` all queue on it. The
  pragmas are applied with `Exec` on whichever connection happens to be open,
  so a reconnect after `driver.ErrBadConn` silently loses `busy_timeout`,
  `foreign_keys` and `synchronous`.
- **Rollups rebuild everything every minute** (STO-1). `RollupOnce` runs
  `INSERT OR REPLACE … SELECT … WHERE ts < cutoff` for every tier with no lower
  bound, rewriting about 86,000 rows per minute at 30 containers and 48 h of
  raw history. This is the measured 339 ms p95 and it grows with retention.
- **Long-term rollups are corrupted at the raw-retention edge** (STO-2).
  Retention deletes raw rows below `now - Raw`, which is not a bucket boundary.
  The next rollup recomputes the straddling 15m and 1h buckets from the rows
  that remain and overwrites the correct aggregate. Because every bucket
  straddles the cutoff at some point, every 1h bucket older than the raw window
  carries an aggregate of a partial hour, and 1h buckets are kept forever by
  default.
- **Budget and vacuum** (STO-5). The budget is the size of the database file
  plus its WAL and shm files, so a large WAL can latch the emergency pause.
  `DELETE` never shrinks the file, `VACUUM` only runs when the budget is below
  critical, and nothing checkpoints or truncates the WAL.
- **Unbounded tables** (STO-9). `container_instances`, archived `resources`,
  `coolify_deployments` and `alert_evaluation_state` are never pruned, and the
  archived resource list is not paginated.
- **Event ids restart on boot** (EVT-1). `metrics.Event.ID` is the engine's
  in-memory sequence, which restarts at 1 on every process start. Events are
  persisted with `INSERT OR IGNORE` on that id, so after a restart new events
  are silently dropped whenever their id collides with a stored row.

The constraints that shape the decision are the ones in `docs/PRODUCT.md`:
one Go process, one local SQLite database, no external services, forward-only
migrations, stored data survives upgrade, every piece of background work
bounded, and degradation reported rather than hidden.

## Decision

### 1. Two pools: one writer, a small read-only pool

The storage manager opens the database twice through `database/sql`:

- **Writer pool**: exactly one connection (`MaxOpenConns(1)`,
  `MaxIdleConns(1)`). Every statement that modifies data, and every
  transaction that reads and then writes, runs here. SQLite allows one writer
  at a time, so a single Go connection turns lock contention into an ordered
  queue instead of `SQLITE_BUSY` retries.
- **Reader pool**: four connections opened read-only. Every statement that
  only reads runs here: HTTP handlers, SSE decorators, the alert evaluator's
  reads, exports, sparklines, history, budget measurement and `quick_check`.
  Four is a fixed number, not a setting; it covers concurrent browser tabs and
  background readers on a one-server deployment while keeping the memory of
  page caches bounded. The pool's wait count and wait duration from
  `sql.DBStats` are exposed in monitor-health so the bound stays visible.

In WAL mode readers never block the writer and the writer never blocks
readers, so a long export no longer delays a persistence batch, and a rollup
no longer delays a page load.

The manager exposes `Writer()` and `Reader()`. Repositories that today call
`SetDB(store.DB())` receive both handles and route each query by whether it
writes. No `SELECT` outside a write transaction may run on the writer; the
reader pool is opened with `mode=ro` and `_query_only`, so a misrouted write
fails loudly instead of competing for the lock.

### 2. Pragmas live in the DSN, not in `Exec`

Connection settings are part of the connection string so that every
connection the pool opens, including reconnects after `ErrBadConn`, carries
them. Using the `file:` URI form of `github.com/mattn/go-sqlite3`:

```text
writer: file:<path>?_txlock=immediate&_journal_mode=WAL&_synchronous=NORMAL
        &_busy_timeout=5000&_foreign_keys=on&_auto_vacuum=incremental
reader: file:<path>?mode=ro&_query_only=true&_journal_mode=WAL
        &_synchronous=NORMAL&_busy_timeout=5000&_foreign_keys=on
```

- `_txlock=immediate` makes every writer transaction `BEGIN IMMEDIATE`, so
  the write lock is taken at `BEGIN` and `busy_timeout` applies there, rather
  than failing with `SQLITE_BUSY` when a deferred transaction tries to upgrade
  mid-way (for example while an external backup tool holds the lock).
- `synchronous=NORMAL` with WAL is durable against process crashes and loses
  at most the last transactions on power loss, which is acceptable for a
  monitoring history and is the documented SQLite recommendation for WAL.
- `journal_size_limit` has no DSN parameter in the driver. The manager
  registers a named driver with a `ConnectHook` that runs
  `PRAGMA journal_size_limit=67108864` (64 MiB) on every new connection. The
  hook is the only place pragmas are applied outside the DSN, and it runs for
  every connection, so nothing is lost on reconnect.
- `auto_vacuum=incremental` in the writer DSN takes effect for databases
  created from now on, because the pragma runs before the first migration
  creates a table. Existing databases are converted once; see §8.

### 3. Boot integrity check is bounded

Boot runs `PRAGMA quick_check` on the writer connection, not
`integrity_check`. `quick_check` verifies page structure in time roughly
proportional to the number of pages touched by the tree walk and skips the
index-content verification that makes `integrity_check` O(database size).

The full `integrity_check` is available on demand from the diagnostics
surface (ADR 017): session-authenticated, run on a reader connection, bounded
by a context deadline, and reported as a result rather than blocking boot.
After an unclean shutdown Binnacle logs it and marks it in monitor-health;
it does not run the full check automatically, because a one-gigabyte database
would fail the container healthcheck for the whole scan. SQLite's own WAL
recovery is the correctness mechanism; the full check is an operator tool.

### 4. Rollups are incremental from a persisted watermark

Each rollup tier (`1m`, `15m`, `1h`, for host and resource tables) keeps a
watermark row in a new table:

```sql
CREATE TABLE rollup_watermarks (
  table_name TEXT PRIMARY KEY,   -- e.g. 'host_rollups_1h'
  closed_until INTEGER NOT NULL, -- ms; buckets with ts < closed_until are computed
  updated_at INTEGER NOT NULL
);
```

A rollup pass for tier size `T` and lateness window `L = 2 min`:

1. `upper = truncate(now, T)`, the start of the currently open bucket.
2. `lower = max(truncate(closed_until - L, T), raw_deleted_below)`, where
   `raw_deleted_below` is the retention cutoff from §5.
3. `INSERT OR REPLACE … SELECT … WHERE ts >= lower AND ts < upper GROUP BY …`.
4. Persist `closed_until = upper` in the same transaction.

The lateness window re-aggregates the buckets a late persistence batch could
still touch, which keeps the current "late samples repair the aggregate"
property without rewriting history. A bucket is never recomputed once it is
below `raw_deleted_below`, so partially deleted raw data can never overwrite a
correct aggregate. The pass is one short write transaction per tier and table
whose row count is bounded by `(L + T) / 10 s × resources`, independent of
retention length. Its wall-clock duration is reported in monitor-health.

The 1m tier lags real time by up to `T + L`, and the 1h tier by up to 62 min.
History queries use raw or 1m data for recent ranges, so the lag is not
visible in charts; the gap classifier (P3-06) selects the finest tier whose
retention covers the range start.

### 5. Raw retention is aligned to the hour and never ahead of final buckets

Retention deletes raw sample rows only below a cutoff `C` that is:

- aligned down to a whole hour: `C = truncate(now - Raw, 1h)`, and
- not above the 1h watermark: `C = min(C, closed_until(host_rollups_1h),
  closed_until(resource_rollups_1h))`.

`C` is persisted as `raw_deleted_below` in `rollup_watermarks` before the
first `DELETE` runs. Together with §4 this gives the invariant: every 1h
bucket below `C` was computed from complete raw data, and no bucket below `C`
is ever recomputed. Rollup tiers' own retention cutoffs are aligned down to
one hour as well; the alignment is harmless there and keeps one rule.

Retention runs one minute after boot, then hourly, and immediately when
`SetRetention` changes the policy. Deletes run in batches of 500 rows
(2,000 when the budget is critical or worse) with
`DELETE … WHERE rowid IN (SELECT rowid … LIMIT ?)`, loop on `RowsAffected`
instead of a `COUNT(*)` per iteration, and yield between batches on every
pass so persistence batches interleave. Each pass has a total duration budget;
when it is exhausted the pass stops, reports in monitor-health, and resumes on
the next tick. Retention duration is reported in monitor-health.

### 6. Retention for the tables that had none

| Table | Rows deleted | Cutoff |
| --- | --- | --- |
| `container_instances` | `stopped_at` or `destroyed_at` set, and the later of them is below the cutoff | the 1m tier retention, aligned to the hour. Running instances (`stopped_at IS NULL AND destroyed_at IS NULL`) are never deleted. |
| `resources` (archived) | `archived_at` set and below the cutoff, and no rollup, sample, event, incident or check still references the resource | the 1h tier retention. When the 1h tier is unlimited, a count cap of 1,000 archived resources applies instead: the oldest-archived beyond the cap are removed together with their history through the existing deletion job, so nothing is orphaned. |
| `coolify_deployments` | `updated_at` below the cutoff | the raw retention. The sync ignores deployments whose Coolify timestamp predates the cutoff so a deleted row is not re-inserted and re-announced. |
| `alert_evaluation_state` | `last_evaluated_at` below the cutoff, any phase | the raw retention. A target not evaluated for the whole raw window no longer exists; the `alerts` rows that describe what happened are separate and keep their own lifecycle. |

The archived-resources API is paginated with the same cursor convention as
events. The deletion-job tables and the existing deletion worker are unchanged.

### 7. Budget from page counts, incremental vacuum, WAL kept small

The database budget compares live data, not file size:

```text
used = (page_count - freelist_count) × page_size
```

read via pragmas on a reader connection every minute. The WAL and shm files
are excluded because they are bounded independently: `journal_size_limit`
truncates the WAL to at most 64 MiB on the next checkpoint, and retention
ends with `PRAGMA wal_checkpoint(TRUNCATE)`. The free-disk floor
(`MinimumFreeSpace`, 64 MiB) is checked each minute as well and, when
breached, triggers the emergency pause regardless of the budget, because
SQLite needs disk to recover.

The thresholds stay at 80 % warning, 95 % critical and 98 % emergency of the
configured target. At emergency the retention pass shortens retention
instead of only pausing:

1. delete the oldest raw rows down to half the configured raw retention
   (aligned, and respecting §5), checkpoint, vacuum incrementally, re-measure;
2. if still at emergency, delete the oldest 1m rows down to half the
   configured 1m retention and re-measure;
3. if still at emergency, pause raw persistence (today's behaviour) and report
   `persistence_emergency`.

The 15m and 1h tiers are never shortened automatically, because long-term
history is the product promise. The pause clears once usage drops below the
critical threshold, not only below emergency, so the state cannot flap. Both
the shortened retention and the pause are visible in monitor-health and as
events.

Space is reclaimed with `PRAGMA incremental_vacuum(N)` after each retention
pass, in batches of 1,000 pages (about 4 MiB) with a yield between batches,
until the freelist is under 10 % of `page_count` or the pass has reclaimed
25,000 pages (about 100 MiB). A full `VACUUM` never runs automatically except
for the one-time conversion in §8, because it needs twice the database size on
disk and holds the write lock for the whole copy.

### 8. Migration strategy for existing databases

Stored data must survive the upgrade; migrations are forward-only. The upgrade
to this architecture is one SQL migration plus two one-off, bounded, resumable
Go steps that record their completion in `application_metadata`.

**Migration `023_rollup_watermarks.sql`** creates `rollup_watermarks` and
indexes needed by the new retention predicates. No data is rewritten in SQL.

**Rollup repair.** The first rollup pass after the upgrade finds no watermark
rows and runs the repair instead of a normal pass:

1. `oldest = MIN(ts)` over `host_samples_10s` and `resource_samples_10s`;
   `lower = align_up(oldest, 1h)`. The bucket that contains `oldest` is left
   as it is: its raw rows are already partial, so it cannot be recomputed
   correctly.
2. Recompute every 1m, 15m and 1h bucket with `ts >= lower` from raw, in
   chunks of six hours of raw data per write transaction, yielding between
   chunks so persistence batches interleave. This repairs every 15m and 1h
   bucket whose raw rows still exist.
3. Write `closed_until = truncate(now, T)` for each tier and
   `raw_deleted_below = lower`. Normal incremental passes follow.

Buckets older than the raw window cannot be repaired because their inputs are
gone. They are kept rather than deleted: an aggregate of part of an hour is
still better history than none, and their `sample_count` column (about 360
for a full hour at today's persistence cadence) identifies them. This ADR
does not mandate surfacing that in the UI.

**`auto_vacuum` conversion.** `PRAGMA auto_vacuum` cannot change on an
existing database without a full `VACUUM`. At the first start after the
upgrade the manager reads `PRAGMA auto_vacuum`; when it is not
`incremental` and free disk space is at least the database size plus
`MinimumFreeSpace`, it runs `VACUUM` once before the workers start, logs the
duration, and records completion. When free space is insufficient it logs a
warning, reports `vacuum_pending` in monitor-health, keeps the pre-upgrade
space-reclamation behaviour for that database, and retries at the next
start. Boot is delayed by the `VACUUM` once; at the default one-gigabyte
budget this is tens of seconds on the supported targets, which is accepted
as a one-time upgrade cost in exchange for never running a full `VACUUM`
again.

**Event ids.** Stored event ids are already `TEXT`; existing rows keep their
numeric-looking ids and remain queryable. The TEXT cursor comparison in
`QueryEvents` is replaced by the rowid cursor from P3-06, so mixed old and new
ids sort by `ts` then insertion order rather than lexically.

### 9. Event identity

Each event gets a durable, globally unique id that is independent of the
process lifetime: a ULID (26 Crockford base32 characters encoding 48 bits of
Unix milliseconds and 80 bits of randomness), generated in-process from
`crypto/rand` with the monotonic-within-a-millisecond rule so a batch keeps
its order. The encoder is about forty lines in `internal/metrics`; no new
module is added, in line with `docs/DEPENDENCIES.md`.

`metrics.Event.ID` becomes a string carrying the ULID. The per-process
`Sequence` stays, as a separate `Seq` field, and remains what the SSE stream
uses in its `id:` frame field and what `Last-Event-ID` resumes from, because
resuming needs a per-connection monotonic counter, not a global identifier.
Events generated inside the storage package (`persistence_emergency`,
`persistence_resumed`, gap events) use the same generator, so they can no
longer collide with each other or with engine events. Persistence inserts
events with `INSERT OR IGNORE` as today; with unique ids the clause only
protects against a replayed batch.

ULIDs are preferred over a boot-id prefix because they need no coordination
with `boot_sessions`, survive several process starts within one host boot
without persisting a counter, and sort by time as text, which keeps the
events table index useful.

## Consequences

- **Concurrency.** Reads no longer wait behind writes and writes no longer
  wait behind reads. Write statements still serialize on one connection by
  design; the ordering guarantee this gives persistence, rollups and
  retention is kept deliberately. Code that previously relied on one
  connection for read-your-writes within a request must do both on the
  writer inside one transaction.
- **Bounded background work.** Rollup cost is proportional to the lateness
  window, not to retention. Retention, vacuum and the repair are chunked,
  yield between chunks, and report their durations. Every step has a bound
  that is visible in monitor-health.
- **Correct long-term history going forward.** New 15m and 1h buckets are
  computed from complete data and never overwritten by partial data. Buckets
  older than the raw window at upgrade time keep their partial aggregates,
  identifiable by `sample_count`.
- **API changes.** Event `id` becomes a string in the live stream and in the
  history API, and live event payloads gain `seq`. The archived resources
  endpoint becomes paginated. Both are documented breaking changes permitted
  before 1.0 by the compatibility policy.
- **Upgrade cost.** One `VACUUM` on first start after the upgrade when disk
  space allows, and a background rollup repair over the raw window. Both are
  logged and both are resumable.
- **Backups.** Nothing changes for operators: a stopped-service copy or the
  SQLite online backup API remains the supported method in WAL mode
  (ADR 019). The 64 MiB WAL cap makes a copy of the three files less likely
  to be large.
- **Configuration.** No new settings. Pool size, lateness window, batch sizes
  and the WAL cap are constants, documented here, so the bounds cannot be
  configured away.

## Alternatives

- **Keep one connection and only fix the pragmas.** Rejected: it leaves the
  339 ms rollup and every export queued ahead of persistence. The audit
  measured this as the dominant latency source.
- **A larger or configurable reader pool.** Rejected: on a one-server
  deployment four readers cover the realistic concurrency, and an
  unbounded or operator-tuned pool makes memory use invisible.
- **Rollups chained tier to tier (1h from 15m from 1m).** Rejected: the
  existing schema computes min, max and count from raw and chaining would
  change the semantics of `avg` for buckets with uneven sample counts. The
  watermark makes raw-based rollups cheap enough.
- **Deleting unrepairable long-term buckets at upgrade.** Rejected: it
  destroys history the operator chose to keep; the partial aggregates are
  marked by `sample_count` and remain useful.
- **Full `VACUUM` on a schedule.** Rejected: it needs twice the database size
  on disk and blocks writes for the whole copy, which contradicts bounded
  background work. The one-time conversion to incremental auto-vacuum is the
  only full `VACUUM` Binnacle runs.
- **Boot-id-prefixed event ids** (`<boot>-<seq>`). Rejected in favour of
  ULIDs: the prefix needs a persisted per-boot counter to survive restarts
  within one host boot and does not sort as text. A third-party ULID module
  is also rejected; the encoder is small enough to own.
- **Converting the event id to an `INTEGER PRIMARY KEY` rowid.** Rejected:
  ids must be assigned before persistence so the live stream, the in-memory
  ring and the stored row agree, and a migration rewriting `events` is not
  needed to fix the collision.
