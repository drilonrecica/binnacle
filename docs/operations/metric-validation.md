# Metric validation

This guide describes how Binnacle collector metrics map to reference Linux and
Docker interfaces, and how to verify them.

## Host metrics

| Binnacle metric | Reference interface | Validation notes |
| --- | --- | --- |
| CPU percent | `/proc/stat` aggregate `cpu` line | Busy percentage over the sampling interval; busy is everything except `idle` and `iowait`, so `irq` and `softirq` count as busy. Should stay in `[0, 100]`. |
| Memory used | `/proc/meminfo` `MemTotal` - `MemAvailable` | Matches the kernel's own "used" convention on modern Linux. |
| Load | `/proc/loadavg` first field | 1-minute load average. |
| Uptime | `/proc/uptime` first field | Seconds since boot. |
| Network RX/TX | `/proc/1/net/dev` counters (host network namespace) | Rates are deltas over elapsed time; counters never decrease. |
| Disk read/write bytes and IOPS | `/proc/diskstats`, summed over the whole devices in `<host sys>/block` | Partitions and `dm-*`, `md*`, `zram*`, `loop*` and `ram*` devices are excluded, so each byte is counted once; compare with `iostat -d` on the physical disks. Without `<host sys>/block` disk I/O is unknown and the `host` collector shows degraded. |
| Filesystem used/total and inodes | `/proc/1/mountinfo` plus `statfs` on `<host root>/<target>` for each retained mount | Matches `df` on the host. Pseudo filesystems are filtered by type and duplicates by `maj:min`. |

## Docker metrics

| Binnacle metric | Reference | Semantics |
| --- | --- | --- |
| CPU host percent | `docker stats` `CPU %` | Ratio of container CPU time to system CPU time, expressed as a percent of one host core. |
| CPU cores ratio | `docker stats` | `container_delta / system_delta`; multiply by host cores for absolute core usage. |
| Memory working set | `docker stats` `MEM USAGE` | `usage - inactive_file` (`total_inactive_file` on cgroup v1), falling back to `usage` when cache data is unavailable. |
| Memory percent | `docker stats` `MEM %` | Working set divided by container limit or host memory, whichever is smaller. |
| IO rates | `docker stats` `NET I/O`, `BLOCK I/O` | Counter deltas divided by elapsed seconds. A container without networks (host or `none` network mode) has unknown network rates, not zero. |

All rates divide by elapsed time measured on the monotonic clock, so a
wall-clock step (NTP correction, manual change) does not skew them; samples
are stored with their UTC wall-clock time.

## Collection cadence and failures

Host and Docker are collected by two independent loops: the host loop every
`collection.host_interval`, the Docker loop every
`collection.container_interval` (both 2 s by default). A slow or unavailable
Docker daemon never delays host metrics. Each published snapshot carries the
latest result of both loops; `host.at` and each resource's `lastSeenAt` give
the time of the tick that measured them.

A Docker tick lists containers, inspects new or changed ones and reads stats
for the running ones, at most `Docker.MaxConcurrency` calls at a time, each
call limited to 5 s and the whole tick to 20 s.

- **One container fails** (removed after the list, call timeout, tick
  deadline): only that container is skipped. Its component is `unknown` with
  no metrics and its next rate starts from a fresh baseline; its resource
  sums the components that reported. The `docker` collector stays healthy and
  its reason reads `N of M containers skipped: …`.
- **The list fails**: the tick fails and counts towards the `docker` collector
  turning degraded (3 failures) and down (6). The last-known resources stay
  visible with `"stale": true`, status `unknown`, no metrics and their old
  `lastSeenAt`. Stale resources are never stored, and an observation is stored
  at most once, so history shows a gap for the outage rather than a flat line.

`freshAt` of a collector is the time of its last successful tick.

## Stored samples

The live view shows every tick. History stores one raw sample per
`persistence.raw_interval` (10 s by default), built from **every** tick
published in that window rather than the single snapshot at the end of it,
so a burst shorter than the window still reaches history:

| Field kind | Examples | Stored value |
| --- | --- | --- |
| Rates and gauges | CPU percent, memory and swap used, load, network and disk bytes/s, IOPS, packets/s, container CPU, memory and I/O | Mean of the window's ticks, each tick weighted equally. Ticks run at the collection interval, so the mean of the per-tick rates equals the rate over the window; a tick that ran late counts like any other. |
| Counter deltas | network errors and drops | Sum over the window's ticks. |
| Totals and identity | memory and swap total, uptime, status, PIDs | Value of the last tick. |

Each raw sample also stores the smallest and largest tick of the window
(`<column>_min` and `<column>_max`) for every metric that has rollup
statistics: host CPU (busy and per mode), load, memory and swap used, network
and disk bytes/s, disk IOPS (read plus write of the same tick), and resource
CPU, memory, network and block I/O. Raw history returns them as each point's
`min` and `max`, and the 1 m, 15 m and 1 h rollups take their `min` and `max`
from them, so a spike between two samples is the rollup maximum. Their `avg`
is the mean of the raw samples. Rows written before this change have no
ranges and fall back to the sample value.

A tick counts once: the host and Docker loops each republish the other's
latest result, so a host tick is identified by `host.at` and a Docker tick by
the resource's `lastSeenAt`. Stale resources are never counted. A resource
that no tick measured during the window is not stored for it, and a container
replaced during the window keeps the samples it reported before it went away,
so `active_instance_count` is the number of containers seen in the window.
A window without a host tick (a stuck host loop) stores the last host
observation unchanged.

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
