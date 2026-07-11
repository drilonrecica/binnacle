# 007 — Server-Sent Events for live updates

## Context

The dashboard needs efficient one-way live updates.

## Decision

Use authenticated SSE. See [SPEC §18](../docs/SPEC.md#18-live-update-transport).

## Consequences

Fan-out is bounded and clients reconnect.

## Alternatives

Polling and WebSockets are not the alpha transport.
