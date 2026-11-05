# Installation

Binnacle is distributed as a container image. The supported paths are Coolify
(one-click service), Docker Compose, or GHCR.

> **Current release:** For production, pin the immutable `v0.6.0` image tag.
> The `stable` tag tracks the latest stable release. Qualify a source-built
> `local` image before using it in production.

> **Qualification gates:** advanced authentication and portability are
> implemented but disabled in packaged defaults. Leave both feature flags off
> unless you are explicitly running their acceptance tests.

## Requirements

- Linux host (x86_64 or arm64)
- Docker Engine 29.5.1 or newer with a reachable Unix socket
- Read access to `/proc`, `/sys`, `/etc/passwd`, and `/etc/os-release` on the host
- A persistent volume mounted at `/var/lib/binnacle`

## Generate a setup token

Binnacle checks the daemon release during production startup and fails closed
if the version is missing, malformed, or older than 29.5.1. Upgrade the host
Engine before deploying Binnacle. A distribution security backport reported
under an older release string does not satisfy this requirement. Demo mode
does not connect to Docker and has no Engine requirement.

On first start, Binnacle requires a high-entropy setup token. Generate one before deploying:

```bash
export BINNACLE_SETUP_TOKEN="$(openssl rand -hex 32)"
```

Store it in a password manager. After the first administrator is created, the setup token is permanently disabled and cannot be reused.

## Install with Coolify

1. Add the Binnacle service template to your Coolify instance (or use **Docker Compose Empty** and paste `packaging/coolify/binnacle.yaml`).
2. In **General**, change the generated **Service Name** (`service-...`) to `binnacle`.
3. Deploy the stack. Coolify generates a persistent 64-character setup token automatically.
4. Expose the service on your chosen domain. Coolify's proxy routes to container port `8080`.
5. Copy `SERVICE_HEX_64_BINNACLE` from the stack environment, open the URL, and complete onboarding.

The Coolify template mounts the host `/proc`, `/sys`, `/etc/passwd`,
`/etc/os-release`, and the Docker socket. It runs `read_only: true` with
`no-new-privileges` and a 128 MiB memory limit.

### Password hashing and the memory limit

Passwords are hashed with Argon2id at 32 MiB, three passes, and up to four
lanes. At most two hashes or verifications run at once across the whole
process, so password work peaks at 64 MiB inside the 128 MiB limit. A login,
setup claim, or MFA password check that arrives while both slots are busy is
refused with `429 server_busy` and `Retry-After: 1`; the browser retries after
a second. Hashes written by earlier releases at 64 MiB keep verifying, but each
one takes both slots until the password is changed.

The template intentionally exposes only first-run setup and optional Coolify
enrichment variables. Binnacle's secure defaults apply to advanced
authentication, portability, private-network access, Prometheus, and
notification limits. Edit the Compose file only when deliberately enabling one
of those features.

Coolify intentionally prefixes Docker containers, networks, and volumes with
the stack UUID. Keep those generated names: they prevent collisions between
multiple Binnacle deployments. The Compose service and volume names themselves
are already stable (`binnacle`, `binnacle-docker-socket-proxy`,
`binnacle-data`, and `binnacle-docker-api`).

Both Coolify Compose files use the published Binnacle image. They do not require
the repository or a local Docker build context on the Coolify server.

## Install with Docker Compose

For a published release:

```bash
git clone https://github.com/drilonrecica/binnacle.git
cd binnacle
export BINNACLE_SETUP_TOKEN="$(openssl rand -hex 32)"
docker compose -f packaging/docker/docker-compose.yml up -d
```

Then open `http://127.0.0.1:8080` and complete onboarding.

To evaluate the unreleased source tree, build the local image and explicitly
override the Compose image:

```bash
make image
export BINNACLE_IMAGE=ghcr.io/drilonrecica/binnacle:local
docker compose -f packaging/docker/docker-compose.yml up -d
```

This uses the same mounts, limits, and persistent volume as the published
release path. Do not treat an unqualified development image as a release.

## Bootstrap credentials

Instead of using the setup token, you can bootstrap the admin account from an environment variable or Docker secret:

```yaml
environment:
  BINNACLE_ADMIN_USERNAME: admin
  BINNACLE_ADMIN_PASSWORD_FILE: /run/secrets/binnacle_admin_password
```

When bootstrap credentials are present and no user exists, Binnacle creates the account automatically. The setup token is still required for public/non-loopback first runs unless you are using local development mode.

## Docker socket security

The Docker Unix socket is not logically read-only. Mounting it read-only (`:ro`)
does not prevent container mutation through the Engine API. The production
Compose and Coolify manifests therefore mount the daemon socket only into a
pinned socket-proxy sidecar. Binnacle receives a separate Unix socket that
allows only ping, version, container list/inspect/stats/logs, and events reads.
Do not replace this with a direct daemon-socket mount in production. A direct
mount remains possible only as an explicit legacy deployment override.

For Docker secrets, mount the master key as a file rather than exposing it in
the container environment:

```yaml
services:
  binnacle:
    environment:
      BINNACLE_MASTER_KEY_FILE: /run/secrets/binnacle_master_key
    secrets:
      - binnacle_master_key
secrets:
  binnacle_master_key:
    file: ./secrets/binnacle-master-key
```

The file must contain a 32-byte key written as 64 hexadecimal characters or as
base64, for example from `openssl rand -hex 32`, and should be readable only by
the Binnacle container. Binnacle refuses to start with a key in any other form
and names the accepted formats in the error. `BINNACLE_MASTER_KEY` remains
supported for upgrades, but do not configure it together with
`BINNACLE_MASTER_KEY_FILE`.

## Reverse proxies and HTTPS

Binnacle listens on plain HTTP; a reverse proxy terminates TLS. Binnacle must
still know that the browser used HTTPS, because the same-origin check expects
`https://` origins, session cookies need the `Secure` attribute, and HSTS is
sent only on secure responses. Two settings provide that knowledge:

- `BINNACLE_PUBLIC_URL` — the origin browsers use, such as
  `https://binnacle.example.com`. When set it is authoritative: the `Origin` of
  every mutation must match it, cookies are `Secure` when it is `https`, and
  HSTS follows. Only the scheme, host and port are used. Prefer this setting;
  it does not depend on the proxy's address.
- `BINNACLE_TRUSTED_PROXY_CIDRS` — hosts or networks whose `X-Forwarded-For`
  and `X-Forwarded-Proto` headers are believed, for example `172.18.0.0/16`
  for the Docker network the proxy shares with Binnacle. Without a public URL,
  the rightmost `X-Forwarded-Proto` value from a trusted peer decides the
  scheme. With or without one, the first untrusted address in
  `X-Forwarded-For` (read right to left across every header line) identifies
  the client for rate limiting and session fingerprints. A `/0` prefix is
  refused.

Without either setting, a deployment behind an HTTPS proxy accepts logins but
rejects every mutation with `403 csrf_invalid`, and Binnacle logs a warning at
startup. The Coolify files set both: `BINNACLE_PUBLIC_URL` from Coolify's
`SERVICE_URL_BINNACLE_8080` magic variable and the RFC 1918 ranges as trusted
proxies, because the container publishes no host port and only other
containers can reach it. Narrow the ranges to the proxy's network if untrusted
workloads share it. The plain Docker Compose file leaves both empty, since its
loopback port makes the Docker gateway the peer for every local client. The
identity allowlist `BINNACLE_AUTH_PROXY_CIDRS` is separate and stays exact.

`scripts/e2e-tls-proxy.sh` exercises both settings through a TLS-terminating
Caddy container; CI runs it against the freshly built image.

### HTTP hardening

Every response, including the UI document, its assets, `/healthz` and
`/metrics`, carries `Content-Security-Policy` (same-origin only,
`frame-ancestors 'none'`), `X-Frame-Options: DENY`,
`X-Content-Type-Options: nosniff`, `Referrer-Policy:
strict-origin-when-cross-origin` and an `X-Request-ID`. Binnacle cannot be
embedded in a frame. A proxy that adds its own CSP should not loosen these.

The listener waits at most 10 s for request headers, closes idle keep-alive
connections after 120 s and refuses request headers above 64 KiB. Each request
other than the live stream (`/api/v1/live`) and followed logs
(`/api/v1/logs?follow=true`) gets a 60 s budget. If the listener fails, the
process exits non-zero so the container restarts.

Each request is logged once as an `http request` JSON line on stderr. The line
carries the client's network prefix (`/24` for IPv4, `/56` for IPv6), never
the full address, and redacts token query parameters. Health probes and asset
requests are logged at debug level. `/healthz` returns only `{"state": ...}`
and does not reveal the version.

## Environment variables

Key variables you may need to set at deployment time:

- `BINNACLE_SETUP_TOKEN` — required for first install.
- `BINNACLE_DATA_DIR` — defaults to `/var/lib/binnacle`.
- `BINNACLE_HOST_PROC` — defaults to `/host/proc`.
- `BINNACLE_HOST_SYS` — defaults to `/host/sys`.
- `BINNACLE_HOST_PASSWD` — defaults to `/host/etc/passwd` for process username resolution.
- `BINNACLE_LOGS_MAX_LINES` — bounded log response ceiling; defaults to `5000`.
- `BINNACLE_LOGS_MAX_RESPONSE_BYTES` — bounded log byte ceiling; defaults to `1048576`.
- `BINNACLE_LOGS_REDACTION_PATTERNS` — up to 16 additional RE2 patterns separated by `||`.
- `BINNACLE_DOCKER_SOCKET` — the packaged default is the filtered socket at `/var/run/binnacle-docker/docker.sock`; standalone binaries default to `/var/run/docker.sock`.
- `BINNACLE_MASTER_KEY` — 64-character hex or base64 encoding of a 32-byte key that encrypts notification, Coolify and MFA secrets.
- `BINNACLE_MASTER_KEY_FILE` — absolute path to a file containing the master key; preferred for production secrets.
- `BINNACLE_COOLIFY_URL` and `BINNACLE_COOLIFY_API_TOKEN[_FILE]` — optional read-only Coolify enrichment.
- `BINNACLE_FEATURE_ADVANCED_AUTH` — enables TOTP and trusted-proxy authentication; defaults to `false` pending qualification.
- `BINNACLE_FEATURE_PORTABILITY` — enables personal API tokens, exports, and eligibility for Prometheus; defaults to `false` pending qualification.
- `BINNACLE_PROMETHEUS_ENABLED` — enables token-authenticated `/metrics` only when portability is also enabled; defaults to `false`.
- `BINNACLE_AUTH_MODE` — `local`, `proxy`, or `local_and_proxy`; defaults to `local`. Proxy modes require advanced authentication to be enabled.
- `BINNACLE_AUTH_PROXY_CIDRS`, `BINNACLE_AUTH_IDENTITY_HEADER`, and `BINNACLE_AUTH_ALLOWED_SUBJECT` — required together for proxy modes; CIDRs must be exact `/32` or `/128` immediate peers.
- `BINNACLE_PUBLIC_URL` — the origin browsers use (`https://binnacle.example.com`); drives the same-origin check, `Secure` cookies and HSTS behind a TLS-terminating proxy. See [Reverse proxies and HTTPS](#reverse-proxies-and-https).
- `BINNACLE_TRUSTED_PROXY_CIDRS` — hosts or networks (`/32`, `/128`, or a Docker network such as `172.18.0.0/16`) allowed to supply `X-Forwarded-For` and `X-Forwarded-Proto`; `/0` is refused.
- `BINNACLE_NOTIFICATIONS_ALLOW_PRIVATE_TARGETS` — private webhook/SMTP opt-in; defaults to `false` and requires restart.
- `BINNACLE_NOTIFICATIONS_MAX_CONCURRENCY` — delivery workers; defaults to `4`.
- `BINNACLE_NOTIFICATIONS_QUEUE_CAPACITY` — dispatch queue; defaults to `1000`.
- `BINNACLE_NOTIFICATIONS_DELIVERY_TIMEOUT` — per-attempt timeout; defaults to `15s`.
- `BINNACLE_NOTIFICATIONS_REMINDER_INTERVAL` — open-incident reminders; defaults to `2h`.

Additional settings are available in the authenticated Settings interface. See
the [product boundaries](../PRODUCT.md) for the supported runtime and security
model.
