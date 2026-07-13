# 013 — No telemetry by default

## Context

Operators expect local monitoring to stay private.

## Decision

Do not send telemetry by default. See the [privacy guarantee](../docs/PRODUCT.md#security-privacy-and-read-only-guarantees).

## Consequences

No analytics or remote SaaS dependency is introduced.

## Alternatives

Usage tracking is rejected.
