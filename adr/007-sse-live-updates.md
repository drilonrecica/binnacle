# 007 — Server-Sent Events for live updates

## Context

The dashboard needs efficient one-way live updates.

## Decision

Use authenticated SSE. See [Product architecture](../docs/PRODUCT.md#architecture-and-persistence-constraints).

## Consequences

Fan-out is bounded and clients reconnect.

## Alternatives

Polling and WebSockets are not the live transport.
