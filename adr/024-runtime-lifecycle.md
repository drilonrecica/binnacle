# ADR 024: Runtime lifecycle, shutdown budgets and the Docker version gate

## Status

Accepted (2026-11-05). Completes ADR 022 §7, which left the order of
component shutdown to this change, and refines the Engine requirement in
`docs/PRODUCT.md` ("fails closed") from "startup exits" to "no Docker call
until a supported Engine is verified". Implemented by P3-08 of the v1.0 plan
(findings RUN-1, RUN-3, RUN-4, RUN-6).

## Context

The v1.0 audit found that a container stop took longer than Docker's 10 s
grace and lost data, and that an unsupported Engine made the container
crash-loop:

- `Server.Shutdown` waits for active connections to go idle. The live stream
  and followed logs never go idle, so shutdown waited out its whole budget
  (15 s, more than Docker's grace) and the process was SIGKILLed.
- The signal context was passed to every component's `Start`. SIGTERM
  cancelled every worker at the same moment, before any `Stop`: queued
  persistence batches were dropped and the database could close under a
  write.
- An unreachable Docker daemon, or one older than 29.5.1, exited 1 at boot.
  The restart policy re-ran migrations and the quick check on every restart,
  and the UI never came up to say why.
- The Go GC did not know the 128 MiB container limit.

## Decision

1. **Start context.** Components receive a context that the signal context
   cancels only while startup is still running (a signal during a migration
   still aborts it). Once every component has started, it is detached: each
   component ends its own work in `Stop`. The run context is cancelled after
   the last `Stop` as a backstop. Components that used to rely on the start
   context being cancelled (sessions cleanup, demo generator) now cancel
   their own context in `Stop`.
2. **Order.** Components stop in reverse registration order: HTTP first
   (no new requests, streams end), then notifications, alerts, checks,
   sessions, Coolify, the collector, the Docker gate and client, then
   persistence (drains), then the database (closes), then the engine.
3. **Budgets.** The whole shutdown is capped at 9 s (`app.ShutdownBudget`).
   Each component gets the smaller of its own budget and what is left of
   that: HTTP 3 s, persistence 3 s, everything else 1 s by default
   (`app.StopBudgeter`). A `Stop` that overruns its budget is abandoned and
   logged by name, and the next component still stops.
4. **Streams.** The HTTP server's `BaseContext` is a context that
   `RegisterOnShutdown` cancels, so every request context, and with it every
   live stream and log follow, ends when `Shutdown` starts. Connections still
   open at the deadline are closed.
5. **Persistence drain.** `Persistence.Stop` stops closing windows, then
   writes every queued batch on a context that only `Stop`'s deadline
   cancels. Batches left at the deadline are counted as dropped and logged.
   The database closes only after that.
6. **Docker version gate.** `dockerapi.Gate` wraps the Docker client. Until
   the version probe sees a supported release, every call except `Version`
   is refused with `ErrEngineUnverified` and the reason (unreachable,
   missing, malformed, prerelease or too old). Nothing runs against an
   unverified daemon, so the requirement stays fail-closed, but startup no
   longer fails: the docker collector is degraded with the reason from the
   first tick, and the UI, history, checks and alerts keep running. The
   probe repeats with backoff from 5 s, doubling to 5 min. Once it passes the
   gate stays open; the socket-access probe runs then. The event stream
   subscribes only after the gate opens.
7. **Memory limit.** At startup, unless `GOMEMLIMIT` is set (the runtime
   applies it), the Go soft memory limit is set to 90 % of the cgroup memory
   limit (cgroup v2 `memory.max`, else v1 `memory.limit_in_bytes`). The 10 %
   left over is for memory the GC does not manage: SQLite's page cache, cgo
   and thread stacks. Without a cgroup limit nothing is set.
8. **Readiness.** `/readyz`, unauthenticated, answers 200 while the
   application is running and the database answers `SELECT 1` within 2 s,
   503 otherwise. It lists every collector's state word but never its reason
   (SEC-21). A degraded collector does not make Binnacle unready: a
   monitoring UI that hides itself when Docker is degraded cannot tell the
   operator why. `/healthz` stays the liveness probe.
9. **Log follows** read every component at once and omit `Until`.

## Consequences

- `docker stop` finishes within Docker's default 10 s grace, and the last
  queued windows are written before the database closes.
- An Engine upgrade no longer needs a Binnacle restart: the gate opens on
  the next probe.
- A deployment that relied on the container exiting to notice an
  unsupported Engine must watch the docker collector state instead (in the
  UI, monitor-health, or `/readyz`).
- New long-lived components must stop their own goroutines in `Stop`; the
  start context is no longer cancelled for them.
