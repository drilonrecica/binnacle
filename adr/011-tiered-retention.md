# 011 — Tiered retention and rollups

## Context

Local history needs bounded storage and useful long-term resolution.

## Decision

Use raw data plus rollup tiers. See [Product architecture](../docs/PRODUCT.md#architecture-and-persistence-constraints).

## Consequences

Retention is configurable but validated.

## Alternatives

Indefinite raw retention is rejected.
