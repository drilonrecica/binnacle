# ADR 015: Bounded checks and deterministic alerts

## Context

Binnacle needs local availability checks and actionable alerts without adding an external service, an unbounded scheduler, or a general-purpose rule language.

## Decision

HTTP checks use one bounded queue and a fixed worker pool. Targets are validated at parse, DNS resolution, redirect, and dial time. Environment proxies are disabled. Check results never retain response bodies.

Alert families are explicit Go implementations backed by persistent per-target state. Transitions and normalized events share a database transaction. Silences and deployment grace suppress notifications, not evaluation. Current resource health is decorated at presentation time; collected snapshots remain unchanged.

## Consequences

The implementation is predictable, restart-safe, and resource-bounded. Adding a rule family requires code and tests. Arbitrary expressions, external delivery, incidents, recurring maintenance, TCP checks, and certificate-expiry checks remain out of scope.
