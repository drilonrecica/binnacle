# 003 — Typed SQLite storage

## Context

Historical metrics must remain local and queryable.

## Decision

Use typed SQLite tables, not EAV storage. See [Product architecture](../docs/PRODUCT.md#architecture-and-persistence-constraints).

## Consequences

Schema migrations are owned by Binnacle.

## Alternatives

External databases and generic metric-value rows are rejected.
