# 014 — Bounded queues and graceful degradation

## Context

Slow consumers must not exhaust memory or block collection.

## Decision

Bound queues and explicitly degrade under pressure. See [SPEC §10.3](../docs/SPEC.md#103-concurrency-model).

## Consequences

Components expose overload state and may drop stale work.

## Alternatives

Unbounded queues and goroutine-per-request models are rejected.
