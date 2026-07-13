# ADR 016: Local incidents and durable notifications

## Status

Accepted for v0.3.

## Decision

Binnacle groups simultaneously firing alerts by affected entity into a single
local incident. Incident membership and notification intent are written in the
same SQLite transaction as the alert transition. A bounded in-process worker
delivers the durable outbox through generic HTTPS webhooks or TLS-protected
SMTP.

Group keys are deterministic: resources and checks use `resource:<id>`, host
CPU and memory use `host:server`, filesystems use `filesystem:<mount>`, and
Docker or persistence failures use `subsystem:<id>`. Only one open incident may
exist for a key. It resolves automatically when its last firing alert resolves.

Channel destinations and credentials are encrypted with the deployment master
key. Delivery is at least once; consumers deduplicate with the idempotency key.

## Consequences

SQLite remains the only stateful dependency and restart recovery is simple.
Ambiguous network failures can create duplicates. This release intentionally
does not add acknowledgement, assignment, arbitrary templates, provider SDKs,
or an external queue.
