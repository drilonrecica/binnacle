# ADR 018: Coolify enrichment and external access

Status: Accepted for v0.5

## Decision

Binnacle may optionally enrich stable Docker-derived resources from the
team-scoped, read-only Coolify v4 API. Only bounded identity, project,
environment, domain, category, deployment status, and safe commit metadata are
accepted. Environment values, compose content, API logs, secrets, and other
sensitive endpoints are outside the client contract.

The last successful bounded enrichment cache is persisted. Coolify failure is
reported separately and never degrades Docker collection. Manual Binnacle
labels remain authoritative, followed by Coolify API metadata and then
Docker/Compose fallback.

Local administrator access may be strengthened with RFC 6238 TOTP and one-time
recovery codes. TOTP seeds use the existing master-key encryption boundary and
are never returned after confirmation.

External identity is delegated to a trusted reverse proxy rather than adding
an OIDC client. Proxy identity is accepted only from a separate CIDR allowlist,
through a configured header, for one exact subject. It bootstraps the normal
same-origin Binnacle session and CSRF cookies and maps to the single local
administrator. Untrusted identity headers are ignored.

## Consequences

The integration adds no control-plane permissions and Docker monitoring
continues independently. Deployments must configure proxy trust explicitly;
forwarded-header trust alone never grants identity. Upstream authentication and
MFA remain the proxy provider's responsibility.
