# 010 — Stable logical resource and ephemeral container instance identities

## Context

Deployments replace container IDs while logical services persist.

## Decision

Separate stable logical resource IDs from container instance IDs. See [SPEC §16](../docs/SPEC.md#16-resource-model-and-identity).

## Consequences

History and UI grouping survive replacements.

## Alternatives

Container IDs as resource identities are rejected.
