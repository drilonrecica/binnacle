# Coolify enrichment and administrator access

## Coolify enrichment

Binnacle can use a team-scoped Coolify v4 token with only the `read`
permission. Configure `BINNACLE_COOLIFY_URL` and either
`BINNACLE_COOLIFY_API_TOKEN` or `BINNACLE_COOLIFY_API_TOKEN_FILE`. Environment
configuration is authoritative. Alternatively, configure the URL and token in
Settings; UI token storage requires `BINNACLE_MASTER_KEY` and the token is never
returned.

The integration follows Coolify's [team-scoped authorization
model](https://coolify.io/docs/api-reference/authorization) and the pinned
[v4.1.2 OpenAPI contract](https://github.com/coollabsio/coolify/blob/v4.1.2/openapi.json).
It reads only selected project, environment, application, service, database,
and deployment fields. Do not grant `read:sensitive`, `write`, `deploy`, or
`root`. Binnacle never requests or retains compose content, environment values,
API logs, private keys, or secrets.

Metadata is polled every five minutes and active deployments every 30 seconds.
Requests have a 10-second timeout, two-request concurrency, response/count
limits, redirect rejection, DNS revalidation, and cloud-metadata blocking.
Private Coolify targets are supported. HTTPS is required unless the deployment
explicitly sets `BINNACLE_COOLIFY_ALLOW_INSECURE_HTTP=true`.

The last successful safe metadata cache remains available during an outage.
Coolify degradation is separate from Docker collection. Display precedence is
manual `binnacle.*` labels, Coolify API metadata, then Docker/Compose metadata.

## Local MFA

Settings can enroll RFC 6238 TOTP for local authentication. Enrollment requires
the current password and a configured master key. Binnacle displays a manual
Base32 seed and `otpauth://` URI; no QR library is included. Confirmation
returns ten high-entropy recovery codes once. Only recovery-code hashes and the
encrypted TOTP seed are stored.

Changing MFA revokes other sessions. A recovery code is consumed atomically.
TOTP applies only to local login; an external identity provider owns its MFA.

## Trusted-proxy authentication

Set `BINNACLE_AUTH_MODE` to `local`, `proxy`, or `local_and_proxy` (`local` is
the default). Proxy modes also require:

- `BINNACLE_AUTH_PROXY_CIDRS`: a separate allowlist for immediate proxy peers;
- `BINNACLE_AUTH_IDENTITY_HEADER`: the trusted identity header;
- `BINNACLE_AUTH_ALLOWED_SUBJECT`: the one exact accepted subject.

The normal forwarded-header proxy list does not grant identity. Headers from
untrusted peers are ignored. A same-origin bootstrap maps the exact subject to
the single local administrator and issues normal Binnacle session and CSRF
cookies. In `proxy` mode local login is disabled after setup. Never expose
Binnacle directly while relying on proxy authentication.
