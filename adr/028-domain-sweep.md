# ADR 028: Domain sweep

Status: accepted (2026-11-25)

## Context

Binnacle's HTTP checks (ADR 015) are bound to Docker resources: a check
probes one URL of one container's service, and its alert marks that resource
down or degraded. ADR 015 left certificate-expiry checks out of scope.

An operator's public domains are a different thing to watch. They exist
whether or not a container serves them, they fail in ways a container probe
does not see (a lapsed registration, a certificate about to expire at the CDN,
a missing SPF or DMARC record, a security header dropped by a proxy change),
and they change rarely enough that an hourly look is plenty. The owner runs
about thirty of them from one Binnacle.

## Decision

1. **A sweep, not a check.** `internal/sweep` keeps a list of bare domains,
   independent of resources, and probes each on its own interval (10 minutes
   to 24 hours, one hour by default). One probe records: the NS set
   (registered), the A/AAAA set (resolves), the final status of one
   `GET https://<domain>/`, the leaf certificate's issuer, expiry and days
   left, whether `Strict-Transport-Security` and `Content-Security-Policy`
   are on the final response, the apex SPF record and the DMARC `p=` policy.
   A 5xx is recorded as a status (a Cloudflare 525 included), not as a
   failure code.
2. **Same bounds as the checks.** The request goes through the shared
   outbound policy (validated at parse, resolve, redirect and dial time), with
   at most three redirects, 64 KiB of body that is drained and never kept, a
   20-second budget per probe, a one-second scheduler tick, a bounded queue
   and a fixed worker pool (`sweep.max_concurrency`, default 4). The sweep
   shares the checks' private-target opt-in. Failure codes reuse the checks'
   names so the UI labels them the same way.
3. **Latest state plus a change feed.** `sweep_state` holds one row per
   domain, the latest result. A fingerprint over the watched fields (status,
   certificate identity, NS and address sets, SPF, DMARC policy, the two
   headers) is stored with it; when a run's fingerprint differs from the
   stored one, a `domain_change` event naming what moved is written to the
   events table in the same transaction. History is the change feed, not a
   table of every probe.
4. **Five alert families, global scope.** `domain_unreachable` (critical),
   `domain_cert_expiring` (warning, threshold 14 days),
   `domain_dns_missing` (critical), `domain_mail_auth_missing` (warning) and
   `domain_headers_missing` (warning) are evaluated as one step of the
   existing evaluator from `sweep_state`, with target type `domain` and the
   domain name as target, so incidents and notifications work unchanged and
   group per domain. A value the sweep could not observe (no certificate
   without a handshake, no headers without a response, no mail records
   without NS records) holds its family rather than failing or recovering
   it. A domain that is removed resolves whatever was firing for it. The
   `alert_rules` scope check is not widened: the families use the `global`
   scope and are not overridable per domain.
5. **Managed under Settings › Alerting.** `GET/POST /api/v1/sweep/domains`,
   `GET/PATCH/DELETE /api/v1/sweep/domains/{id}` and
   `POST /api/v1/sweep/domains/{id}/run`, behind the same session, CSRF and
   rate-limit guards as the checks. The Domains card shows one row per
   domain: status, HTTP, certificate days, SPF, DMARC, HSTS, CSP, last
   checked, with add, pause, run now and remove.

## Consequences

This supersedes the certificate-expiry exclusion of ADR 015; TCP checks and
arbitrary expressions remain out of scope. The one-server scope in
docs/PRODUCT.md is unchanged: the sweep watches the operator's own domains
from the one Binnacle that monitors their server, it is not a multi-server
monitor. Per-probe history is deliberately not kept; the fingerprint change
feed answers "what changed, and when" within the existing events budget.
Demo mode seeds seven domains through a fixed prober so every state of the
tab is visible without network access.
