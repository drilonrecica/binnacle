# 012 — Coolify-first, Docker-compatible resource model

## Context

Coolify metadata improves grouping but Docker remains the runtime basis.

## Decision

Model Coolify when present while retaining Docker compatibility. See [SPEC §16](../docs/SPEC.md#16-resource-model-and-identity).

## Consequences

The resolver degrades gracefully when metadata is absent.

## Alternatives

Coolify-only identity is rejected.
