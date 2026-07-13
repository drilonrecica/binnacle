# 001 — Go backend

## Context

Binnacle needs a small, single-process Linux service.

## Decision

Use Go for the backend. See [Product architecture](../docs/PRODUCT.md#architecture-and-persistence-constraints).

## Consequences

Static deployment and standard-library HTTP are preferred; CGO is accepted for SQLite.

## Alternatives

Node.js and multi-service runtimes add operational dependencies.
