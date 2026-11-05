# Benchmarking Binnacle

`scripts/benchmark.py` runs the binnacle binary in deterministic demo mode,
measures it, and compares the result with the targets below. A missed target,
or one the run could not measure, makes it exit with status 1. The release
gate (`scripts/release-gate.sh`) runs `make benchmark` and fails on that exit
status, so these targets are release requirements, not guidance.

## Quick start

```bash
# Release gate: long run for 30 and 100 containers (about 15 minutes)
make benchmark

# Short run on a fresh demo database (about 1 minute)
make benchmark-short

# Short-scenario matrix: 10/30/50/100 containers x 10/50/100 checks
make benchmark-matrix

# Custom run
python3 scripts/benchmark.py --scenario long-run --containers 100 --duration 600 --output report.json
```

`make benchmark` writes `benchmark-report.json` (every measurement) and
`benchmark-report.md` (the target table the release record includes). A run
that fails keeps its data directory, with `binnacle.log`, and prints its path.

## Scenarios

### Long run (the gate)

Each container count runs in two starts on one data directory:

1. **Seed.** `--demo-raw-history 49h` writes 49 h of raw 10 s samples (host,
   resources and container instances, filesystems once a minute), starting at
   a whole hour, and rebuilds the 1 m, 15 m and 1 h rollups. This start uses
   the `long-term` retention preset so no retention pass deletes anything
   while it seeds. 30 containers take about a minute; 100 take about three.
2. **Measure.** A second start on the seeded database with the stock
   (`balanced`, 48 h raw) retention, the check scheduler on
   (`--demo-run-checks`, 10 synthetic checks at 30 s), and eight SSE clients
   (the default `live.max_clients`) held open for the whole run. The
   retention pass at boot deletes the oldest hour, the share an hourly pass
   deletes at steady state, then vacuums and checkpoints the WAL. Live demo
   writes, the rollup worker, the alert evaluator and the notification worker
   run as in production. The run is measured for 300 s after a 5 s warmup.

The run also checks that it did what it claims; each item is a target row:

- at least one retention pass completed, and it was not cut short by its
  30 s budget;
- every enabled check stored a result during the run;
- after retention, 48 h (47 h if an hour boundary passed between the two
  starts) to 49 h of raw history remain, so exactly the expired hour went;
- all eight SSE clients streamed for the whole run.

### Short

A fresh demo database with the default demo history (24 h at one minute, the
latest 2 h at 10 s), one SSE client, no check scheduler, 60 s. It is for quick
comparisons between commits. Retention and vacuum are not applicable; every
other target applies.

## Targets

A run uses the targets of the smallest tier that is at least its container
count; above 100 containers there are none. `TARGETS` in
`scripts/benchmark.py` holds the same values and is what the harness checks;
change both together.

| Metric | Measured as | 30 containers | 100 containers |
| --- | --- | --- | --- |
| `rss_max_mib` | Highest RSS from `/proc/<pid>/stat`, sampled every 2 s | ≤ 100 MiB | ≤ 128 MiB |
| `cpu_avg_percent` | Process CPU time over the run, percent of one core | ≤ 2 % | ≤ 3 % |
| `write_p95_ms` | p95 of every persistence batch write in the measured window, from the `persistence_write` histogram | ≤ 50 ms | ≤ 100 ms |
| `rollup_p95_ms` | p95 of the rollup passes (one a minute) in the measured window, from the `rollup` histogram | ≤ 250 ms | ≤ 500 ms |
| `retention_ms` | Slowest retention pass since start (vacuum and checkpoint included); the boot pass that deletes the expired hour | ≤ 2 s | ≤ 5 s |
| `vacuum_ms` | Slowest incremental vacuum since start | ≤ 2 s | ≤ 2 s |
| `wal_max_mib` | Largest `binnacle.db-wal` sampled every 2 s; `journal_size_limit` is 64 MiB | ≤ 64 MiB | ≤ 64 MiB |
| `sse_bytes_per_second_per_client` | Bytes per second of the busiest of the SSE clients | ≤ 20 KiB/s | ≤ 64 KiB/s |
| `dropped_batches` | Increase of the `dropped` counter over the run | 0 | 0 |
| `worker_failures` | Increase of every `monitor-health` failure counter except deliveries | 0 | 0 |
| `write_transactions_per_second` | Increase of `write_transactions` over the run per second | ≤ 1 | ≤ 1 |

`write_transactions` counts committed SQLite transactions that changed at
least one row; reads, read-only transactions and no-op statements are not
counted (STO-4).

Deliveries are reported (`deliveries_failed`) but not counted as failures:
the demo channel points at a `.invalid` host and the benchmark sets no master
key, so the alert raised by the failing demo check always ends in a failed
delivery.

### Where the numbers come from

The targets were set in P3-13 from the long run on a 4-CPU x86_64 Linux
development machine, with headroom for slower CI runners, and line up with
the `monitor-health` warning thresholds (50 ms write, 500 ms rollup, 2 s
retention and vacuum). That run measured:

| Metric | 30 containers | 100 containers |
| --- | --- | --- |
| RSS max | 74 MiB | 90 MiB |
| CPU average | 0.5 % | 1.1 % |
| Write p95 (p50) | 17.5 ms (5.0 ms) | 42.5 ms (15.8 ms) |
| Rollup p95 | 69 ms | 238 ms |
| Retention pass deleting one hour | 0.42 s | 2.33 s |
| Incremental vacuum | 1.8 ms | 0.03 ms |
| WAL max | 4.3 MiB | 5.2 MiB |
| SSE per client | 13.7 KiB/s | 42.7 KiB/s |
| Write transactions | 0.78/s | 0.78/s |
| Seeding 49 h | 59 s | 207 s |
| Database after seeding | 218 MiB | 713 MiB |

Deleting one hour of 49 leaves the freelist under the vacuum threshold, so
the incremental vacuum has nothing to do; it is measured to catch a pass that
starts doing work it should not. The earlier goals (RSS under 50 MB, CPU under
0.5 %, idle SSE under 10 KB/s) were never met by any measured run and are
replaced by this table. Lowering a target is a deliberate change to this
document and `TARGETS`, reviewed like any other.

## How the write p95 is computed

`monitor-health` reports, for each `latency` entry, a `histogram` of
cumulative bucket counts since the process started (bounds from 1 ms to 30 s,
then an unbounded bucket) besides the p50/p95/max of the latest 256 runs.
The harness reads `monitor-health` when the measured window starts and when it
ends, subtracts the two histograms, and computes the quantile with linear
interpolation inside the bucket, as Prometheus `histogram_quantile` does. A
quantile that falls in the unbounded bucket is infinite and fails its target.
The earlier harness took the p95 of "latest write latency" values sampled
once a second, which saw at most one write in ten.

## Reproducibility

- The demo seed is always `1`; each run uses a fresh temporary data directory
  and a clean `BINNACLE_*` environment.
- The JSON report records the binary version, host OS, architecture and CPU
  count; compare reports from the same machine only.

## Real-host validation

The demo does not run the Docker collector, so Docker API rate and collection
duration are not measured. To benchmark against real Docker and host
interfaces, run `binnacle` in non-demo mode with an isolated data directory
and record the host architecture, Docker version and binary version with the
numbers.
