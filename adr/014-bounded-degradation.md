# 014 — Bounded queues and graceful degradation

## Context

Slow consumers must not exhaust memory or block collection.

## Decision

Bound queues and explicitly degrade under pressure. See [Product architecture](../docs/PRODUCT.md#architecture-and-persistence-constraints).

## Consequences

Components expose overload state and may drop stale work.

## Alternatives

Unbounded queues and goroutine-per-request models are rejected.
