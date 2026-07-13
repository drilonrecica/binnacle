# 008 — Single-binary process architecture

## Context

Deployment must stay simple and self-contained.

## Decision

Ship one Go process with embedded frontend assets. See [Product architecture](../docs/PRODUCT.md#architecture-and-persistence-constraints).

## Consequences

Internal packages remain modular without extra core services.

## Alternatives

Microservices are rejected.
