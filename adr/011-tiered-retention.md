# 011 — Tiered retention and rollups

## Context

Local history needs bounded storage and useful long-term resolution.

## Decision

Use raw data plus rollup tiers. See [SPEC §24](../docs/SPEC.md#24-retention-rollups-and-storage-budget).

## Consequences

Retention is configurable but validated.

## Alternatives

Indefinite raw retention is rejected.
