# Metric validation

This guide describes how Binnacle collector metrics map to reference Linux and
Docker interfaces, and how to verify them.

## Host metrics

| Binnacle metric | Reference interface | Validation notes |
| --- | --- | --- |
| CPU percent | `/proc/stat` aggregate `cpu` line | Busy percentage over the sampling interval; should stay in `[0, 100]`. |
| Memory used | `/proc/meminfo` `MemTotal` - `MemAvailable` | Matches the kernel's own "used" convention on modern Linux. |
| Load | `/proc/loadavg` first field | 1-minute load average. |
| Uptime | `/proc/uptime` first field | Seconds since boot. |
| Network RX/TX | `/proc/1/net/dev` counters (host network namespace) | Rates are deltas over elapsed time; counters never decrease. |
| Filesystem used/total and inodes | `/proc/1/mountinfo` plus `statfs` on `<host root>/<target>` for each retained mount | Matches `df` on the host. Pseudo filesystems are filtered by type and duplicates by `maj:min`. |

## Docker metrics

| Binnacle metric | Reference | Semantics |
| --- | --- | --- |
| CPU host percent | `docker stats` `CPU %` | Ratio of container CPU time to system CPU time, expressed as a percent of one host core. |
| CPU cores ratio | `docker stats` | `container_delta / system_delta`; multiply by host cores for absolute core usage. |
| Memory working set | `docker stats` `MEM USAGE` | `usage - inactive_file`, falling back to `usage` when cache data is unavailable. |
| Memory percent | `docker stats` `MEM %` | Working set divided by container limit or host memory, whichever is smaller. |
| IO rates | `docker stats` `NET I/O`, `BLOCK I/O` | Counter deltas divided by elapsed seconds. |

## Known semantic differences

- **Memory working set:** Binnacle uses `usage - inactive_file`, the same convention
  as the Docker CLI. This intentionally excludes reclaimable page cache and can
  be significantly lower than raw `usage`.
- **CPU host percent:** Binnacle reports percent of one host core. A container
  using two full cores on a four-core host reports `200%` in `docker stats` but
  `50%` host percent here.
- **Filesystems:** Binnacle reads the host's mount table from
  `<host proc>/1/mountinfo` (PID 1 is in the host mount namespace), keeps one
  mount per `maj:min` device with the shortest target, drops pseudo
  filesystems (`proc`, `sysfs`, `cgroup*`, `overlay`, `nsfs`, `squashfs`,
  `tmpfs` under `/run` and `/dev`, and similar), and calls `statfs` on
  `<host root>/<target>`. The filesystem that holds the data directory is
  always reported. A mount whose `statfs` fails is skipped and named in the
  `host` collector reason; when the host root is not mounted no filesystems
  are reported and the `host` collector shows degraded.

## Running the reference suite

The fixture-backed suite lives in `internal/collector/reference` and covers
normalization formulas plus live `/proc` sanity checks:

```bash
go test ./internal/collector/reference/...
```

For release qualification, capture the same values from the reference tools on
the target host and attach sanitized evidence to the release record.
