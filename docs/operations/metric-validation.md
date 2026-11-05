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
| CPU host percent | `docker stats` `CPU %` divided by the online CPUs | Ratio of container CPU time to system CPU time over the same interval, expressed as a percent of the whole host (all cores). |
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
- **CPU host percent:** Binnacle reports the share of the whole host, `docker
  stats` the share of one core. A container using two full cores on a
  four-core host reports `200%` in `docker stats` but `50%` host percent here.
- **Filesystems:** Binnacle reads the host's mount table from
  `<host proc>/1/mountinfo` (PID 1 is in the host mount namespace), keeps one
  mount per `maj:min` device with the shortest target, drops pseudo
  filesystems (`proc`, `sysfs`, `cgroup*`, `overlay`, `nsfs`, `squashfs`,
  `tmpfs` under `/run` and `/dev`, and similar), and calls `statfs` on
  `<host root>/<target>`. The filesystem that holds the data directory is
  always reported. A mount whose `statfs` fails is skipped and named in the
  `host` collector reason; when the host root is not mounted no filesystems
  are reported and the `host` collector shows degraded.

## Validating the collectors

Two layers check the collectors against the reference tools: fixture tests
that run in every `make check`, and a script the owner runs on a real host
before a release.

### Fixture tests

`internal/collector/production/validation_test.go` feeds the production
sampler a recording of a real host and of Docker Engine API answers, and
compares its output with the values the reference tools report for the same
inputs:

| Fixture file | What it holds |
| --- | --- |
| `proc/t0/`, `proc/t1/` | Two snapshots of the files the host collector reads (`stat`, `meminfo`, `loadavg`, `uptime`, `diskstats`, `1/net/dev`, `1/mountinfo`) |
| `elapsed` | Seconds between the two snapshots |
| `sys-block.txt` | The entries of `/sys/block` |
| `statfs.tsv`, `df.txt` | `statfs` of every mount and `df -B1` read right after it |
| `docker/containers.json`, `docker/inspect/`, `docker/stats-t0/`, `docker/stats-t1/`, `docker/elapsed` | Engine API list, inspect and two one-shot stats reads |
| `reference.json`, `docker/reference.json` | Reference values computed by `scripts/validate-host.sh` from the files above, independently of the Go code: top's busy share, `MemTotal - MemAvailable`, the `/proc/net/dev` and `/proc/diskstats` deltas, and the Docker CLI's `docker stats` formulas |

The host part was recorded on a workstation with btrfs `/` and `/home` on one
device, CIFS shares behind autofs, zram, partitions and Docker bridges, so it
exercises deduplication, pseudo-filesystem filtering and whole-device disk
sums. The Docker part was written by hand to cover a cgroup v2 container on
two networks, a cgroup v1 container (`total_inactive_file`, `Read`/`Write`
next to `Sync` and `Total` in blkio), a host-network container and an exited
one. The tests serve it from a Unix socket to the real Docker client, so the
Engine API decoding is covered too.

```bash
go test ./internal/collector/production/ -run Validation
go test ./internal/collector/reference/...
```

To add a host to the fixtures, record it and review the files for host
names, user names and paths before committing; replace them with neutral
values, as the committed fixture does with `nas.example` for its CIFS server:

```bash
scripts/validate-host.sh --record /tmp/fixture --window 4
# edit, then recompute the reference values for the edited files:
scripts/validate-host.sh --reference /tmp/fixture
```

The recording needs read access to `/proc/1` and, for the Docker part, to the
Docker socket; the test expects every filesystem it keeps to have a `statfs.tsv`
row and lists the expected mount points, so a new recording also updates that
list.

### Real-host procedure

Run this on the release hardware, or on every server class a release targets,
with Docker Engine 29.5.1 or newer:

1. Build the binary: `make build`.
2. Put the host under steady load for the run: a busy container
   (`docker run -d --rm --name bnv-load alpine sh -c 'while :; do :; done'`),
   a network transfer and some disk writes (for example
   `dd if=/dev/zero of=/var/tmp/bnv bs=1M count=2000 oflag=direct`). Rates
   are compared over a window, so bursty load makes the rate rows noisy.
3. As a user that can read the Docker socket (a member of the `docker`
   group, or root), run:

   ```bash
   scripts/validate-host.sh --window 30
   ```

   The script starts `bin/binnacle` on a free loopback port with a throwaway
   data directory, `BINNACLE_HOST_PROC=/proc`, `BINNACLE_HOST_SYS=/sys`,
   `BINNACLE_HOST_ROOT=/` and the Docker socket (`BINNACLE_DOCKER_SOCKET` to
   override), claims setup with a random password, and reads `/api/v1/server`,
   `/api/v1/resources`, `/api/v1/filesystems` and `/api/v1/collector-health`
   once a second over the window. Around the window it reads `/proc/stat`,
   `/proc/meminfo`, `/proc/1/net/dev`, `/proc/diskstats` and the Engine API
   stats of up to 25 running containers (`VALIDATE_MAX_CONTAINERS`), and
   runs `top`, `free`, `df` and `docker stats`. It only reads from Docker.
4. Read the report. Each row shows Binnacle, the reference, the difference,
   a verdict and the reference source:

   | Rows | Reference | Tolerance |
   | --- | --- | --- |
   | CPU busy | `/proc/stat` delta and `top` (`100 - id - wa`) | 5 percentage points |
   | Memory total, used | `/proc/meminfo`, `free -b` | exact; used 2 % or 64 MiB |
   | Load | `/proc/loadavg` | 0.3 or 10 % |
   | Network rx/tx | `/proc/net/dev` without `lo`, `veth*`, `docker*`, `br-*` (the unfiltered sum is shown for information) | 15 % or 128 KiB/s |
   | Disk read/write, IOPS | `/proc/diskstats` over the whole devices in `/sys/block` | 15 % or 128 KiB/s, 10 IOPS |
   | Filesystem size, used, avail | `df -B1` for the same mount point | 1 % or 256 MiB |
   | Container CPU | Engine API stats delta over the window, `CPU % / online CPUs` (`docker stats` short sample shown for information) | 5 percentage points |
   | Container memory | `docker stats` `MEM USAGE` (`usage - inactive_file`) | 5 % or 16 MiB |
   | Container net, block I/O | Engine API stats deltas (`docker stats` `NET I/O`, `BLOCK I/O` totals) | 20 % or 64 KiB/s |

   Binnacle rates are the mean of the 2 s ticks inside the window; the
   reference is one delta over the window. The script also lists the `df`
   mounts Binnacle leaves out (pseudo filesystems and second mounts of one
   device) and the collector states, and exits 1 when any row reads `CHECK`
   (outside tolerance, or unknown in Binnacle while the reference has a
   value).
5. A `CHECK` on a rate row under bursty load is expected; rerun with a longer
   window under steady load. A `CHECK` that persists, any `CHECK` on a size or
   memory total, an unexpected missing filesystem, or a collector that is not
   `healthy` is a defect: note it in `imp-docs/FINDINGS.md`.
6. For the release record, keep the report and the host description (kernel,
   distribution, Docker version, cgroup version, filesystem types). Set
   `VALIDATE_KEEP=1` to keep the raw inputs the script read; review them
   before attaching anything.

The script runs the binary directly on the host, so it validates the
collectors, not the packaged container's view of the host through
`/host/proc`, `/host/sys` and `/host/root`. The sampler tests cover those
paths with fixtures; to check a packaged deployment, open its Host and
Resources pages next to a report taken on the same host and compare.
