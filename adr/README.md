# Architecture decision records

Accepted decisions that bind the implementation. `docs/PRODUCT.md` defines the
product boundaries these records serve; see [GOVERNANCE.md](../GOVERNANCE.md)
for the process. An ADR is never edited once accepted; a later ADR supersedes
or extends it and says so.

| ADR | Decision |
| --- | --- |
| [001](001-go-backend.md) | Go backend |
| [002](002-svelte-frontend.md) | Svelte 5 frontend with runes and TypeScript |
| [003](003-typed-sqlite-storage.md) | Typed SQLite storage |
| [004](004-docker-coolify-deployment.md) | Docker/Coolify primary deployment |
| [005](005-read-only-operations.md) | Permanently read-only operational model |
| [006](006-agpl-license.md) | AGPL-3.0-only licensing |
| [007](007-sse-live-updates.md) | Server-Sent Events for live updates |
| [008](008-single-binary.md) | Single-binary process architecture |
| [009](009-metrics-engine.md) | Metrics Engine as current-state source of truth |
| [010](010-identities.md) | Logical resource and ephemeral container identities |
| [011](011-tiered-retention.md) | Tiered retention and rollups |
| [012](012-coolify-resource-model.md) | Coolify-first, Docker-compatible resource model |
| [013](013-no-telemetry.md) | No telemetry by default |
| [014](014-bounded-degradation.md) | Bounded queues and graceful degradation |
| [015](015-bounded-checks-and-deterministic-alerts.md) | Bounded checks and deterministic alerts |
| [016](016-local-incidents-and-durable-notifications.md) | Local incidents and durable notifications |
| [017](017-bounded-read-only-diagnostics.md) | Bounded read-only diagnostics |
| [018](018-coolify-enrichment-and-access.md) | Coolify enrichment and external access |
| [019](019-portable-read-access-and-personalization.md) | Portable read access and personalization |
| [020](020-product-ui-design-system.md) | Product UI design system and information architecture |
| [021](021-storage-architecture.md) | Storage architecture: pools, incremental rollups, retention, indexes |
| [022](022-collector-architecture.md) | Collector architecture |
| [023](023-master-key-format-and-rotation.md) | Master key format and key-version rotation |
| [024](024-runtime-lifecycle.md) | Runtime lifecycle, shutdown budgets, Docker version gate |
| [025](025-live-stream-fan-out.md) | Live stream fan-out and precompressed assets |
| [026](026-time-to-full-estimator.md) | Time-to-full estimator |
| [027](027-headroom-design-system.md) | Headroom design system (supersedes the visual parts of 020) |
