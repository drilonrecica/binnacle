# ADR 026: Time-to-full estimator

Status: accepted (2026-11-05, task P3-12)

## Context

The Headroom page (imp-docs/DESIGN.md §Headroom) leads with how full CPU,
memory and each disk are and when the first one runs out. The contract the
frontend builds against is fixed in imp-docs/SPEC.md §P3-12: a `trend` on every
filesystem, a `memoryTrend` on the host, a `role` on every filesystem, a
configurable window, and an alert family `filesystem_full_within_days`. The
spec asks for the `docker` role to come from the daemon's `DockerRootDir`.

## Decision

1. **Least-squares line, computed in the background.** A component
   (`internal/estimator`) runs 15 s after start and then every five minutes,
   or at once when the window setting changes. It reads each current mount's
   used bytes from `filesystem_samples_1m` and memory from `host_rollups_1m`,
   both averaged into 15-minute buckets in SQL (at most 673 rows per series,
   each query an index search), and hands the results to the metrics engine.
   The engine attaches them to every snapshot, so the live stream and the
   REST endpoints carry them and no request computes anything. One run is
   bounded by a 30 s timeout and at most 64 mounts.
2. **Semantics.** `unknown` under three hours of history or without a
   capacity; `steady` when the slope moves usage by less than 1 % of capacity
   over the window; `shrinking` for a negative slope; `filling` otherwise,
   with days to full from the current value to capacity. Confidence is
   1 − RMS residual ÷ max(projected change, 1 % of capacity), clamped to
   [0, 1]. Filesystem capacity is used + available bytes.
3. **Docker data root inferred, not asked.** `GET /info` is outside the socket
   proxy envelope of ADR 022, and widening it would expose more daemon detail
   than Binnacle needs. The Docker loop infers the data root from the named
   volumes it already inspects (`<root>/volumes/<name>/_data`). Without one,
   it assumes `/var/lib/docker`. The host loop marks as `docker` the measured
   mount on the device that holds that path. This is a deliberate deviation
   from the spec's wording, decided with the owner on 2026-11-05.
4. **Alert family.** `filesystem_full_within_days` (built-in, warning,
   threshold 7 days, 15 min trigger and recovery) fails on a confident
   (≥ 0.6) filling estimate under the threshold, recovers when the mount stops
   filling or the projection reaches max(recovery, threshold) days, and holds
   on an unknown or unconfident estimate.

## Consequences

- No new dependency, table or migration; the built-in rule is seeded on
  upgrade like the others.
- Filesystem history is limited by the raw retention (48 h by default), so a
  window longer than that uses what is kept.
- A host whose containers use no named volumes and a non-default data root
  shows that disk as `other` until a named volume appears.
- Estimates lag usage by up to five minutes plus a 15-minute bucket, which is
  negligible at the scale of days.
