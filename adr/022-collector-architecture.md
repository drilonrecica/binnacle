# ADR 022: Collector architecture

## Status

Accepted (2026-11-05). Extends ADR 009 (Metrics Engine as current-state source
of truth) and ADR 014 (bounded queues and graceful degradation); both remain in
force. Implemented by the P2 collection tasks of the v1.0 plan (P2-01 host
namespace and filesystems, P2-02 host metric accuracy, P2-03 Docker state and
events, P2-04 failure isolation and bounded collection).

## Context

The v1.0 audit found that the production sampler
(`internal/collector/production/sampler.go`) does not meet the bounded-work
and explicit-degradation promises of `docs/PRODUCT.md`, and that several host
metrics are read from the wrong namespace:

- **One loop for everything** (COL-6). `Sampler.run` reads the host, then
  walks every container serially with `Inspect` and `Stats`, then publishes.
  No call has a timeout beyond the sampler context. One hung `Stats` call
  stops host metrics as well as container metrics, and a slow Docker daemon
  stretches every host sample. `MaxDockerConcurrency` is stored on the sampler
  but never used; the bounded worker pool in
  `internal/collector/docker/collector.go` is dead code.
- **One failed container fails the tick** (COL-5). A `404` for a container
  that exited between `List` and `Stats` returns an error from
  `collectDocker`. The sampler then republishes `lastResources` at the new
  timestamp, so the persisted history shows a flat line where there was an
  outage, and the alert evaluator sees fresh-looking samples.
- **Container state is event-driven but the events are unreliable** (COL-4).
  The metadata cache is filled once per container and refreshed only by
  events. Docker's health action is `health_status: healthy`; the sampler
  matches the exact string `health_status`, so health never refreshes.
  `die`, `stop` and `kill` set the cached state to the action name rather
  than `exited`, which maps to `unknown`. When the events channel closes
  (daemon restart, socket proxy restart) it is set to `nil` and never
  reopened, after which state is frozen until the process restarts.
- **Host network and mounts come from the container's own namespace**
  (COL-1, COL-2). `/host/proc` is the host's procfs, but `net/dev` and
  `self/mountinfo` under it resolve through the reader's own PID, which is
  Binnacle's container. Filesystem metrics are additionally parsed with the
  wrong parser and are empty in production, so disk-full alerts never fire.
- **Docker events carry no resource id** (EVT-3). `events.NormalizeDocker`
  sets only `ContainerInstance`; the restart-storm and OOM-loop counters
  group by `resource_id` and therefore never count anything.
- **Caches never shrink and rates use the wall clock** (COL-8). `previousStats`
  and the metadata cache keep every container id ever seen, and elapsed time
  for rates comes from `time.Now().UTC()`, which drops the monotonic reading.

The constraints that shape the decision:

- `docs/PRODUCT.md`: read-only toward Docker, local-first, bounded work,
  degradation reported rather than hidden, one Go process.
- The production packaging (`packaging/docker/docker-compose.yml`,
  `packaging/coolify/binnacle.yaml`) runs Binnacle as the non-root user
  `binnacle` (uid 65532) with `read_only: true`, `no-new-privileges`, and a
  socket proxy that allows only `GET` on `/_ping`, `/version`,
  `/containers/json`, `/containers/{id}/(json|stats|logs)` and `/events`.
  Every decision below must work inside that envelope.
- The Docker client (`internal/dockerapi`) already issues one-shot stats
  (`Stream: false`, no previous sample), so a `Stats` call is one cgroup read
  on the daemon side and Binnacle computes deltas from its own previous
  sample. That property is kept.
- The settings already define `Collection.HostInterval` and
  `Collection.ContainerInterval` (both 2 s by default, minimum 1 s), but only
  the host interval is used today.

## Decision

### 1. Two loops in two goroutines, one merged snapshot

The sampler starts three goroutines under one context: a **host loop**, a
**Docker loop**, and an **events goroutine** owned by the Docker loop (§5).
Each loop owns its own state and never touches the other's:

| Loop | Interval | Owns |
| --- | --- | --- |
| host | `Collection.HostInterval` | previous CPU, network and disk counters; host failure counter |
| docker | `Collection.ContainerInterval` | metadata cache, `previousStats`, resource set, Docker failure counter, events stream |

Each loop runs collect-then-wait: the timer for the next tick starts after
the current collection finishes, so a slow tick delays the next one rather
than stacking ticks. A loop never runs two collections concurrently.

The sampler keeps one small `merged` structure under a mutex: the latest host
observation with its boot id and collector health, the latest resource set
with its collector health, and the pending events. After every tick, the loop
that ran updates its part and calls `Engine.Publish` with a snapshot built
from both parts. `Snapshot.At` is the publish time; `HostObservation.At` and
`ResourceSnapshot.LastSeenAt` carry the time of the tick that produced them,
so a consumer can tell how old each half is. Publish frequency is at most
`1/HostInterval + 1/ContainerInterval`; the SSE stream already throttles to
`Live.SSEInterval`, and persistence takes the latest snapshot once per raw
interval, so the extra publishes cost one mutex and one slice copy each.

The engine's `Collectors` map keeps its two entries, `host` and `docker`, but
`CollectorHealth.FreshAt` changes meaning: it is the time of the loop's last
**successful** tick, and a failed tick leaves it unchanged. Consecutive
failures per loop keep the existing thresholds (3 → degraded, 6 → down).
Monitor-health reports the duration of each loop separately and the state of
the events stream (§5); the shape is decided by P3-10.

### 2. Docker tick: List is the source of truth, Inspect fills in metadata

Every Docker tick runs under a tick deadline of 20 s and performs:

1. **`List`** (`all=true`) with a 5 s call timeout. `dockerapi.Container`
   gains the `State` field that the list summary already carries. A `List`
   failure is the only error that fails the tick (§3).
2. **Prune.** Every id in the metadata cache or in `previousStats` that is
   not in the list result is removed. This bounds both maps to the number of
   containers that currently exist; the Docker event `destroy` (§5) removes
   entries earlier when it arrives, but the tick does not depend on it.
3. **Re-inspect on change.** A container is inspected (5 s call timeout,
   through the bounded pool of §4) when it is not in the cache, when the
   state reported by `List` differs from the cached state, when an event
   marked it for re-inspection (§5), or when the cached entry is older than
   10 minutes. Everything else uses the cached metadata. The cached `State`
   is always overwritten with the `List` value, so the running state shown in
   the UI is at most one container interval old even when events are lost.
   Health (`healthy`, `unhealthy`, `starting`) comes from `Inspect` and from
   `health_status:` events (§5); the free-text `Status` column of the list
   summary is not parsed.
4. **`Stats`** for every container whose `List` state is `running`, through
   the bounded pool of §4, one-shot, with a 5 s call timeout. The response
   carries a monotonic sample time (`time.Now()` without `.UTC()`), and the
   UTC copy is derived from it for publishing and storage. Rates use the
   monotonic elapsed time between two samples of the same container id. A
   container whose counters went backwards (restart with the same id) gets
   `nil` rates for that interval and its previous sample is replaced.
5. **Group and publish.** Containers are grouped into resources as today,
   the resource id is derived from the stable key, and the resource set is
   published through the merge of §1.

The identity-resolution and grouping code does not change; this ADR only
changes how the inputs are obtained and what happens when they fail.

### 3. Failure isolation: per-container errors skip the container

Within a tick that listed successfully:

- An `Inspect` or `Stats` error for one container, including `404` because
  the container disappeared after `List`, a call timeout, or the tick
  deadline being reached before that container was processed, affects only
  that container. Its component is published with status `unknown`, runtime
  state as reported by `List`, every metric pointer `nil`, and its
  `previousStats` entry is dropped so the next successful sample cannot
  compute a rate across the gap. The resource that contains it is still
  published; its aggregates sum only the components that reported.
- The tick counts as a success for the failure counter. The `docker`
  collector health carries the count and the first error text in its reason
  (`3 of 30 containers skipped: …`) so partial failure is visible in the UI
  and in monitor-health without flipping the collector state.
- Errors are logged through the app logger at `warn` with the container id,
  coalesced to one line per container per minute, so a flapping container
  cannot flood the log.

When `List` fails, the tick fails: the failure counter increments and the
collector reason carries the error. The resource set is republished with the
identities from the last successful tick, every metric pointer `nil`, status
`unknown`, `LastSeenAt` unchanged, and a new `Stale` flag set to `true`.
Nothing that looks like a fresh measurement is ever republished; a chart
shows a gap and the resource list shows the last-known containers greyed
out, which is what an operator needs to see during a daemon restart.

`ResourceSnapshot` gains `Stale bool` (JSON `stale`). Persistence writes
resource and container rows only for resources whose `Stale` is false, so
the stored history has no rows at all for the outage and the gap classifier
(ADR 021 §4, P3-06) reports it as a gap. The host row is still written
because the host loop was not affected.

### 4. Bounded concurrent collection with per-call timeouts

The dead `StatsCollector` in `internal/collector/docker/collector.go` is
revived as the tick's worker pool and generalised: it takes the list of ids
to process and a function to run per id, runs `MaxDockerConcurrency` workers
(default 4, from `Docker.MaxConcurrency`), gives every call its own 5 s
timeout derived from the tick context, and returns the per-id result or
error. Its `last` map, which kept the newest successful sample for
republishing, is removed: §3 forbids republishing. The pool is used for both
the re-inspect set and the `Stats` set of a tick.

The `dockerapi.Limited` wrapper keeps bounding every call to the daemon from
anywhere in the process (collector, onboarding, diagnostics, version check),
but log reads get their own semaphore: `ReadLogs` is a long-lived follow that
today holds a collection slot for as long as a browser tab is open, so four
open log views stop collection entirely. `Limited` carries two semaphores,
`collection` sized `MaxDockerConcurrency` and `logs` sized 4, and `ReadLogs`
acquires only the latter. The log semaphore is a constant, not a setting,
for the same reason as the reader pool in ADR 021.

Per-tick cost with one-shot stats is bounded by
`ceil(running / MaxDockerConcurrency) × call latency`, about 100 ms for 30
containers on the supported targets, and by the 20 s tick deadline in the
worst case. Containers not reached before the deadline are treated as
per-container failures (§3), the tick still publishes, and the deadline hit
is part of the collector reason.

### 5. Events: resubscribe with backoff, re-discover on reconnect

The events goroutine subscribes with `Docker.Events(ctx)` and forwards each
event to the Docker loop over a bounded channel of 256. When the channel
from the client closes for any reason other than context cancellation, the
goroutine resubscribes with exponential backoff: 1 s doubling to a 60 s cap,
±20 % jitter, reset to 1 s after a stream has stayed open for 60 s. While
disconnected, the `docker` collector reason carries `events disconnected`
and the state is reported in monitor-health; the loop keeps working because
§2 takes state from `List` regardless. On every successful resubscription the
goroutine asks the Docker loop for a **full re-discover**: the next tick
re-inspects every container, so metadata changes that happened while the
stream was down (renames, label edits through recreate) are picked up. If
the forwarding channel is full, the oldest event is dropped, the drop is
counted, and a full re-discover is requested instead; losing an event can
only delay a metadata refresh by one tick, never lose state.

The Docker loop applies events to the cache immediately on receipt, between
ticks, so the UI reflects a stop or a health change before the next `List`:

| Action | Cache effect |
| --- | --- |
| `die`, `stop`, `kill` | state `exited`; drop `previousStats` for the id |
| `pause` | state `paused` |
| `start`, `unpause`, `restart`, `rename`, `update` | mark for re-inspect on the next tick |
| `destroy` | remove from cache and `previousStats` |
| `health_status: <value>` (prefix match) | health `<value>` from the suffix; no inspect needed |
| `oom` | no cache change; normalised as `container_oom` |
| anything else (`exec_*`, `attach`, …) | ignored |

Events are normalised as today and published with the next tick's snapshot,
bounded to 128 pending events with the oldest dropped and counted.

**Resource id on events** (EVT-3). Before publishing, the loop resolves the
container to its resource: the cached metadata gives labels and name, the
existing identity resolution gives the stable key, and the stable key gives
the resource id. For a container not yet in the cache (a `create` or `start`
seen before the first tick), the loop inspects it once through the pool with
the usual timeout. If resolution fails, the event is published with an empty
`ResourceID` and logged at `debug`; the restart-storm and OOM-loop counters
will not see that one event, which is the correct degradation.

### 6. Host loop: PID 1 is the host namespace root

`Paths.HostProc` (`/host/proc` in packaging, `/proc` for a standalone binary)
stays the mount of the host's procfs. Files that depend on the reader's
namespace are read through **PID 1**, which in the host's procfs is the
host's init process:

| Data | Path | Why |
| --- | --- | --- |
| mounts | `<HostProc>/1/mountinfo` | the host mount namespace; parsed as mountinfo (fields after the `-` separator), not as `/proc/mounts` |
| network | `<HostProc>/1/net/dev` | the host network namespace |
| CPU, memory, load, uptime, disk I/O, boot id | `<HostProc>/stat`, `meminfo`, `loadavg`, `uptime`, `diskstats`, `sys/kernel/random/boot_id` | system-wide, not namespaced |
| whole-device filter for disk I/O | `<HostSys>/block` | sysfs is not namespaced |

Both PID 1 files are readable without ptrace permission: procfs serves
`mountinfo` and `net/*` of any visible process to any reader. The host loop
verifies at start that `<HostProc>/1/mountinfo` is readable and, when it is
not, reports the `host` collector degraded with the reason rather than
silently falling back to the container's own view.

**Filesystem usage needs a path inside the host mount namespace**, and
`<HostProc>/1/root/<target>` is not usable for it: traversing
`/proc/<pid>/root` requires `PTRACE_MODE_READ` access to that process, which
the kernel grants only to the same uid or to `CAP_SYS_PTRACE`, and which the
`docker-default` AppArmor profile on the supported Ubuntu and Debian targets
denies toward unconfined host processes even for root. Binnacle runs as uid
65532 without capabilities; `/proc/1/root` is `EACCES` for it while
`/proc/1/mountinfo` and `/proc/1/net/dev` read fine. The decision is the
convention used by `node_exporter`: the packaging bind-mounts the host root
read-only with slave propagation,

```yaml
- /:/host/root:ro,rslave
```

and a new path setting `Paths.HostRoot` (`BINNACLE_HOST_ROOT`, `/host/root`
in packaging, `/` for a standalone binary) is the prefix under which the
host loop calls `statfs(2)` on `<HostRoot>/<target>` for every retained
mount. `rslave` makes mounts that appear on the host after the container
started visible without a restart; `ro` is applied recursively by the
supported Docker Engine on the supported kernels. The collector only calls
`statfs` under `HostRoot` and never opens a file there; no other component
receives the setting.

Mount handling:

- Mounts are deduplicated by `maj:min`, keeping the shortest target; pseudo
  filesystems (`proc`, `sysfs`, `cgroup*`, `tmpfs` under `/run` and `/dev`,
  `overlay`, `nsfs`, `squashfs` snaps, and the other kernel-backed types) are
  dropped by type.
- The filesystem that holds `DataDir` is always kept, identified by the
  `maj:min` of `statfs(DataDir)`, because the storage budget's free-disk floor
  (ADR 021 §7) depends on it.
- A target whose `statfs` fails (`EACCES` because a path prefix is not
  searchable by uid 65532, `ENOENT` because it is hidden by a later mount) is
  skipped and counted; the count and the first path are part of the `host`
  collector reason. Nothing is fabricated.
- When `HostRoot` is configured but is not a mount point (an upgrade that
  did not add the volume), the host loop publishes no filesystems and reports
  the `host` collector degraded with the reason
  `host root not mounted; filesystem metrics unavailable`. Every other host
  metric keeps flowing.

Rates in the host loop use a monotonic `time.Now()` for elapsed time and its
UTC copy for the observation timestamp, the same rule as §2.

### 7. Shutdown

`Stop` cancels the sampler context, waits for the three goroutines with the
caller's deadline, and only then closes the Docker client, so an in-flight
`Stats` call is cancelled by its context rather than failing on a closed
transport. The ordering of component shutdown is decided in P3-08; this ADR
only requires that the loops are joinable.

### 8. Tests

The findings are fixed only when a test fails without the fix. The sampler
tests use a scripted fake Docker client whose `List`, `Inspect`, `Stats` and
`Events` are programmed per tick, including a `Stats` that blocks until its
context is done and an events channel that is closed on demand. The required
cases are:

- a blocked `Stats` for one container: the tick completes within its
  deadline, that component is `unknown` with `nil` metrics, the other
  containers report, and the host loop publishes at least once meanwhile;
- a `404` from `Stats`: the container is skipped, the tick succeeds, the
  resource is still published, `previousStats` no longer holds the id;
- a `List` error: the published resources are `Stale` with `nil` metrics and
  unchanged `LastSeenAt`, and a persistence batch built from that snapshot
  writes no resource rows;
- a closed events channel: resubscription happens with increasing delays and
  the next tick re-inspects every container;
- `health_status: unhealthy` by prefix sets health; `die`, `stop` and `kill`
  map to `exited`; `destroy` prunes both maps;
- a Docker event published with the resource id of its container;
- fixtures for `/proc/1/mountinfo` and `/proc/1/net/dev` in the formats the
  kernel writes, and a test that fails with the old `/proc/mounts` parser;
- more containers than `MaxDockerConcurrency`: never more than that many
  `Stats` calls in flight, asserted by the fake.

## Consequences

- **Host metrics no longer depend on Docker.** A hung daemon, a slow socket
  proxy or one stuck container stalls at most one Docker tick, and that tick
  is bounded by 20 s; CPU, memory, load, network and filesystem samples keep
  their 2 s cadence.
- **`ContainerInterval` becomes effective.** The two intervals were already
  configurable; the Docker interval was ignored.
- **Honest outages.** History shows gaps instead of flat lines during a
  daemon restart; the live UI shows last-known containers marked stale and
  components marked unknown. `ResourceSnapshot.stale` is a new field on the
  live stream and the snapshot API, documented in `docs/INTEROPERABILITY.md`
  as an additive change.
- **Health changes are visible within a tick.** State comes from `List` every
  tick, health from events by prefix, and restart-storm and OOM-loop alerts
  can fire because events carry the resource id.
- **One new host mount.** The packaging gains `/:/host/root:ro,rslave`, and
  `BINNACLE_HOST_ROOT` is documented in `docs/operations/install.md`. The
  mount is read-only, the container stays non-root, and the collector uses it
  only for `statfs`. An operator who does not add the mount after upgrading
  loses filesystem metrics and is told so in the UI, nothing else changes.
  This widens what the Binnacle container can read on the host to anything
  world-readable; the alternative that needed no new mount is rejected below
  because it does not work for a non-root container.
- **Memory is bounded by the number of existing containers.** Both per-container
  maps are pruned every tick; a host that churns containers no longer grows
  the sampler.
- **Log follows cannot starve collection.** Log reads have their own
  semaphore; collection keeps its `MaxDockerConcurrency` slots.
- **More goroutines, still a fixed number.** Three per sampler plus the
  worker pool of at most `MaxDockerConcurrency` goroutines during a tick. No
  goroutine is created per container or per event.
- **Configuration.** One new setting (`HostRoot`). Call timeouts, the tick
  deadline, the backoff bounds, the pending-event cap and the log semaphore
  size are constants documented here so the bounds cannot be configured away.
- **No new dependencies.** Everything uses the standard library, `x/sys/unix`
  and the existing Docker client.

## Alternatives

- **One loop with per-call timeouts.** Rejected: host samples would still
  wait up to the whole Docker tick, and the tick deadline alone would stretch
  the host cadence to 20 s in the worst case.
- **Three or more loops (one per Docker call type, or one per container).**
  Rejected: a goroutine per container is the unbounded model ADR 014 rejects,
  and splitting `Inspect` from `Stats` into separate loops gains nothing once
  both run through the same bounded pool.
- **Events as the only source of container state.** Rejected: the stream is
  lost on every daemon or proxy restart and events can be dropped under
  load; `List` once per tick is one request and authoritative. Events remain
  the fast path for health and stop transitions between ticks.
- **Inspecting every container every tick.** Rejected: `N` requests per
  2 s for metadata that rarely changes; re-inspect on state change, on event,
  on reconnect and on age keeps metadata fresh at a bounded cost.
- **Streaming stats (`Stream: true`, one connection per container).**
  Rejected: one long-lived connection per container is unbounded in the
  same way as a goroutine per container, and one-shot stats are already a
  single cgroup read.
- **Republishing the last successful sample on failure.** Rejected (COL-5):
  it hides outages, and the alert evaluator cannot distinguish a stale value
  from a fresh one.
- **Parsing the list summary's `Status` text for health.** Rejected: the text
  (`Up 3 hours (healthy)`) is a display string without a compatibility
  promise; `Inspect` and the `health_status:` event are the API.
- **`<HostProc>/1/root/<target>` for `statfs`.** Rejected: it needs
  `PTRACE_MODE_READ` on host PID 1, which the non-root `binnacle` user does
  not have, and which the `docker-default` AppArmor profile denies on the
  supported targets regardless of uid. Running the container as root with
  `CAP_SYS_PTRACE` and `apparmor=unconfined` to make it work is a larger
  privilege grant than a read-only root mount, and contradicts the hardening
  the packaging already has.
- **Mounting only the filesystems the operator names.** Rejected as the
  default: the set is unknown in advance and a mount added later would be
  invisible. An operator can still point `BINNACLE_HOST_ROOT` at a narrower
  bind mount; the collector then reports the targets it cannot reach.
- **Keeping one semaphore for collection and logs.** Rejected: a follow holds
  its slot for the lifetime of a browser tab, so `MaxDockerConcurrency` open
  log views stop collection entirely.
