# 010 — Stable logical resource and ephemeral container instance identities

## Context

Deployments replace container IDs while logical services persist.

## Decision

Separate stable logical resource IDs from container instance IDs. See [Product architecture](../docs/PRODUCT.md#architecture-and-persistence-constraints).

## Consequences

History and UI grouping survive replacements.

## Alternatives

Container IDs as resource identities are rejected.
