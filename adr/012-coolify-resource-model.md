# 012 — Coolify-first, Docker-compatible resource model

## Context

Coolify metadata improves grouping but Docker remains the runtime basis.

## Decision

Model Coolify when present while retaining Docker compatibility. See [Product purpose](../docs/PRODUCT.md#purpose-and-audience).

## Consequences

The resolver degrades gracefully when metadata is absent.

## Alternatives

Coolify-only identity is rejected.
