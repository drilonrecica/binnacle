# Coolify enrichment and administrator access

## Coolify enrichment

Binnacle can use a team-scoped Coolify v4 token with only the `read`
permission. Configure `BINNACLE_COOLIFY_URL` and either
`BINNACLE_COOLIFY_API_TOKEN` or `BINNACLE_COOLIFY_API_TOKEN_FILE`. Environment
configuration is authoritative. Alternatively, configure the URL and token in
Settings; UI token storage requires `BINNACLE_MASTER_KEY` or
`BINNACLE_MASTER_KEY_FILE` and the token is never returned. Prefer a Coolify
secret mounted under `/run/secrets` and point `BINNACLE_MASTER_KEY_FILE` to it.

The stored token is bound to the origin (scheme, host and port) it was saved
with. Saving a URL on the same origin keeps it; moving the URL to another
origin requires the token again, so a stored token is never sent to a host it
was not entered for. "Test connection" always uses the token typed into the
form and never the stored one.

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
A Coolify URL configured from the environment (`BINNACLE_COOLIFY_URL`) may
point at a private address; the operator chose it at deployment time. A URL
entered in Settings is held to the same outbound policy as checks and
notifications and reaches a private address only when the deployment sets
`BINNACLE_COOLIFY_ALLOW_PRIVATE_TARGETS=true`; otherwise the connection test
fails with a message naming that setting. HTTPS is required unless the
deployment explicitly sets `BINNACLE_COOLIFY_ALLOW_INSECURE_HTTP=true`.

The last successful safe metadata cache remains available during an outage.
Coolify degradation is separate from Docker collection. Display precedence is
manual `binnacle.*` labels, Coolify API metadata, then Docker/Compose metadata.

## Local MFA

TOTP and trusted-proxy authentication are implemented but disabled by default
pending advanced-auth acceptance testing. Enable them with
`BINNACLE_FEATURE_ADVANCED_AUTH=true`. Coolify enrichment is independent and
remains available while advanced authentication is off. If stored TOTP
enrollment exists, Binnacle refuses to start with the gate off so the
administrator cannot be locked out; enrollment data is never deleted.

Settings can enroll RFC 6238 TOTP for local authentication. Enrollment requires
the current password and a configured master key. Binnacle displays a manual
Base32 seed and `otpauth://` URI; no QR library is included. Confirmation
returns ten high-entropy recovery codes once. Only recovery-code hashes and the
encrypted TOTP seed are stored.

Enabling MFA signs out every other session; the one that enrolled stays
signed in so the recovery codes stay on screen until they are saved. Turning
MFA off signs out every session, that one included. A recovery code is
consumed atomically.
A TOTP code is accepted once: Binnacle stores the last accepted 30-second step
for the account and refuses any code for that step or an earlier one, so a code
seen over someone's shoulder cannot be replayed inside the clock-drift window.
TOTP applies only to local login; an external identity provider owns its MFA.

## Sessions and the local password

Sessions slide with activity up to `BINNACLE_SESSION_IDLE_TIMEOUT` (default
12 h, at most 7 days) and never outlive
`BINNACLE_SESSION_ABSOLUTE_LIFETIME` (default 30 days, at most 90 days); both
can also be changed in Settings → Access. A session bootstrapped from a
trusted proxy ends within 24 hours regardless of the configured lifetime, so a
revoked upstream identity is re-checked at least daily. Activity is recorded at
most once a minute (or every half idle timeout when that is shorter), so a
polling tab does not write the session on every request; the idle expiry can
lag the latest request by up to that interval. Sessions are not bound
to a browser user agent or network prefix: that binding signs people out on
browser updates and network changes without stopping a stolen cookie.

The live dashboard stream and log follow re-check the session on every
20-second heartbeat and close when it has been signed out, expired, or ended by
an MFA or password change. The check does not count as activity, so a tab left
open still idles out. When the stream or any API call finds the session gone,
the web app ends it and opens the sign-in page with `next` set to the page that
was open, so signing in again returns there.

Settings → Access can change the administrator password. The current password
is required, the request is CSRF-protected and login-throttled, and every other
session is signed out while the one making the change stays signed in.

## Trusted-proxy authentication

Set `BINNACLE_AUTH_MODE` to `local`, `proxy`, or `local_and_proxy` (`local` is
the default). Proxy modes require `BINNACLE_FEATURE_ADVANCED_AUTH=true` and:

- `BINNACLE_AUTH_PROXY_CIDRS`: exact immediate proxy peers, expressed only as
  `/32` IPv4 or `/128` IPv6 prefixes;
- `BINNACLE_AUTH_IDENTITY_HEADER`: the trusted identity header;
- `BINNACLE_AUTH_ALLOWED_SUBJECT`: the one exact accepted subject.

The proxy must have a stable address. It must remove client-supplied
`X-Forwarded-For`, `X-Forwarded-Proto`, and the configured identity header,
then set fresh values from the authenticated request. Appending to an incoming
identity header is unsafe; Binnacle rejects duplicate and comma-joined identity
values. The normal forwarded-header proxy list (`BINNACLE_TRUSTED_PROXY_CIDRS`,
which may name whole networks) does not grant identity; only the exact peers in
`BINNACLE_AUTH_PROXY_CIDRS` do. Headers from untrusted peers are ignored. A same-origin bootstrap maps the exact subject
to the single local administrator and issues normal Binnacle session and CSRF
cookies. In `proxy` mode local login is disabled after setup. Never expose
Binnacle directly while relying on proxy authentication.

For Nginx, overwrite the identity value after `auth_request`; do not forward
the client's value:

```nginx
auth_request /verify;
auth_request_set $binnacle_subject $upstream_http_x_forwarded_user;
proxy_set_header X-Forwarded-User $binnacle_subject;
proxy_set_header X-Forwarded-For $remote_addr;
proxy_set_header X-Forwarded-Proto $scheme;
```

For Traefik/Coolify, place a Headers middleware that clears the identity header
before ForwardAuth, then list that header under ForwardAuth
`authResponseHeaders` so it is copied from the authentication response. Put the
middlewares in that order and configure the resulting stable Traefik container
address as an exact Binnacle proxy CIDR. Set `BINNACLE_PUBLIC_URL` as well so
the same-origin bootstrap and the Secure cookies do not depend on
`X-Forwarded-Proto` (see [install.md](operations/install.md#reverse-proxies-and-https)).
