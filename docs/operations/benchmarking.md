# Benchmarking Binnacle

This document describes how to run the reproducible benchmark harness and
interpret the results.

## Quick start

```bash
# Short validation run (60 seconds, 30 synthetic containers)
make benchmark

# Full 10/30/50/100 container matrix (about 4 minutes with default duration)
make benchmark-matrix

# Custom run
python3 scripts/benchmark.py --containers 100 --duration 300 --output report.json
```

## What is measured

The harness starts `binnacle` in deterministic demo mode (`--demo --demo-seed 1`)
with a configurable number of synthetic containers and collects:

| Metric | Source | Purpose |
| --- | --- | --- |
| RSS | `/proc/<pid>/stat` | Resident memory footprint |
| CPU | `/proc/<pid>/stat` | Process CPU time over the run |
| SQLite write latency | `/api/v1/monitor-health` | Persistence latency |
| Queue depth / dropped batches | `/api/v1/monitor-health` | Persistence pressure |
| Write transactions per second | `write_transactions` in `/api/v1/monitor-health` | SQLite write amplification |
| Go heap / goroutines | `/api/v1/monitor-health` | Go runtime pressure |
| Database size | Filesystem + monitor endpoint | On-disk growth |
| SSE bandwidth | `/api/v1/live` | Idle live-stream data rate |

Docker API rate and collection duration are meaningful only when running
against a real Docker engine; the demo harness provides a repeatable baseline
for regression detection and leaves real-host validation to release hardware.

## Performance goals

These targets are guidance, not guarantees, and must be validated on the
release machine with the exact binary version:

- RSS `< 50 MB`
- CPU `< 0.5%` of one core
- SQLite write p95 `< 50 ms`
- Idle SSE `< 10 KB/s`
- Write transactions `< 1` per second at steady state

`write_transactions` counts committed transactions that changed at least one
row; reads, read-only transactions and no-op statements are not counted. The
harness divides its change over the sampled run by the elapsed time. The
STO-4 changes (session activity written once a minute, unchanged alert phases
and container instances not rewritten) took the 30-container demo run from
4.1 to 0.36 write transactions per second. Demo mode does not run the Docker
collector, whose host and resource inventory is now written only on change or
once a minute in one transaction; that path is covered by its unit tests.

## Reproducibility

- The harness always uses seed `1`.
- Each run writes a fresh temporary `BINNACLE_DATA_DIR`.
- Reports are JSON so they can be diffed across commits.

## Real-host validation

To benchmark against real Docker and host interfaces, run `binnacle` in non-demo
mode with an isolated data directory and direct the harness at the same
`BINNACLE_LISTEN_ADDRESS`. Document the host architecture, Docker version, and
binary version alongside the report.
