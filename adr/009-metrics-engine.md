# 009 — Metrics Engine as current-state single source of truth

## Context

Live reads must not contend with historical persistence.

## Decision

Keep current state in the Metrics Engine. See [Product architecture](../docs/PRODUCT.md#architecture-and-persistence-constraints).

## Consequences

Collectors and SSE do not query SQLite for live state.

## Alternatives

Database-driven live dashboards are rejected.
