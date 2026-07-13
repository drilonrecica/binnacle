# 005 — Permanently read-only operational model

## Context

Monitoring credentials are high-risk.

## Decision

Binnacle observes; it does not mutate workloads. See the [read-only guarantee](../docs/PRODUCT.md#security-privacy-and-read-only-guarantees).

## Consequences

No Docker proxy or control actions are added.

## Alternatives

Remediation and deployment control are rejected.
