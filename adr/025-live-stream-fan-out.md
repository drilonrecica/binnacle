# ADR 025: Live stream fan-out and precompressed assets

## Status

Accepted (2026-11-05). Refines ADR 007 ("fan-out is bounded") with the
mechanism. Implemented by P3-09 of the v1.0 plan (findings RUN-5, UI-16).

## Context

Each `/api/v1/live` client held its own Metrics Engine subscription. Every
tick the engine cloned the snapshot per client under its write lock, and
every client ran the alert and Coolify decorators (two SQL queries) and
marshalled the result itself, so live cost grew with the number of open
tabs. `AllowLive` limited the connection rate, not concurrency, and
`live.sse_interval` was parsed but unused. Snapshots and events shared one
channel per client: making room for a snapshot drained the oldest message,
often an event, so a slow client silently lost events. Static assets were
served uncompressed with no validator.

## Decision

1. **One subscription, one encoding.** A hub in `internal/api` holds the
   only engine subscription behind the live stream. It decorates and
   encodes each snapshot once and each event once, and hands the same bytes
   to every client. The hub runs only while at least one client is
   connected; decoration is bounded by a 5 s timeout.
2. **Interval.** Snapshot frames go out at most once per
   `live.sse_interval` (default 2 s). Snapshots published faster are
   coalesced to the latest; events are never coalesced and go out at once.
3. **Separate slots.** The engine subscription and each client queue keep
   the latest snapshot in its own one-element slot and events in an ordered
   queue, so replacing a snapshot never drops an event. A subscriber more
   than 256 events behind is closed instead of skipping events; the browser
   reconnects and resumes with `Last-Event-ID` from the engine's ring.
4. **Resume cursor.** A frame's SSE `id:` is the highest sequence the client
   has been sent, never lower than an earlier frame's: a snapshot's id is
   raised to the last event sent before it. Every event at or below a
   frame's id was sent before that frame.
5. **Concurrency cap.** At most `live.max_clients` streams (default 8,
   1–64) are open at once. The next one gets `503` with `Retry-After` and
   error code `live_capacity`. The connection-rate limit stays in front of
   it.
6. **Precompressed assets.** `pnpm build` writes Brotli and gzip variants of
   compressible files over 1 KiB (`web/scripts/compress.mjs`, Node's own
   zlib, no new dependency). The Go handler serves the best variant the
   client accepts with `Vary: Accept-Encoding`. Every file's ETag is its
   content hash, computed once from the embedded build; each encoding has
   its own ETag, and `If-None-Match` answers `304`.

## Consequences

- Live cost per tick is one clone, one decoration and one marshal however
  many clients are open; per-client work is copying bytes.
- Decorations are shared, so they cannot depend on the requesting session.
  None do today.
- A ninth dashboard tab is refused until another closes or the operator
  raises `live.max_clients`.
- Monitor health counts live clients from the hub, not engine
  subscriptions.

## Alternatives

- Per-client throttling with shared decoration: still clones and encodes
  per client.
- Compressing at request time: CPU per request for content that never
  changes after the build.
