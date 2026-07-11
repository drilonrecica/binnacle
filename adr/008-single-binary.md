# 008 — Single-binary process architecture

## Context

Deployment must stay simple and self-contained.

## Decision

Ship one Go process with embedded frontend assets. See [SPEC §10](../docs/SPEC.md#101-one-binary-internally-modular).

## Consequences

Internal packages remain modular without extra core services.

## Alternatives

Microservices are rejected.
