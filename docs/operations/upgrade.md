# Upgrade

Binnacle does not self-update. Upgrades are performed by replacing the container image.

## Update channels

Container tags follow semantic versioning:

- `stable` — release builds only (no prereleases).
- `beta` — beta and release-candidate builds.
- `edge` — development builds.
- Exact version tags such as `v0.6.0` are immutable.

The access and portability work retains schemas 19 through 21. Back up
`binnacle.db` and its WAL/SHM files before replacing the binary. Existing TOTP
enrollment, API-token metadata, preferences, resources, alerts, checks,
incidents, sessions, secrets, settings, and history are preserved.

Advanced authentication and portability now default to disabled. An upgraded
instance with stored TOTP enrollment will refuse startup until
`BINNACLE_FEATURE_ADVANCED_AUTH=true` is set, preventing an administrator from
being silently locked out. Existing API tokens remain stored but inactive while
portability is disabled. Re-enabling either gate requires no migration or data
conversion. Prometheus additionally requires both
`BINNACLE_FEATURE_PORTABILITY=true` and
`BINNACLE_PROMETHEUS_ENABLED=true`.

Pick a channel in your Compose file or Coolify service settings:

```yaml
image: ghcr.io/drilonrecica/binnacle:stable
```

## Upgrade process

1. Verify the host reports Docker Engine 29.5.1 or newer, and upgrade the host
   Engine first if necessary. Binnacle will refuse production startup on an
   older, missing, or malformed daemon version, including older release strings
   whose distribution claims a security backport:

   ```bash
   docker version --format '{{.Server.Version}}'
   ```

   Update production manifests to include the pinned `docker-socket-proxy`
   sidecar. Binnacle must mount only the shared filtered socket. Existing proxy
   authentication ranges must also be replaced with stable exact `/32` or
   `/128` peers; broader ranges now fail startup.

2. Stop Binnacle and copy the closed SQLite database. This ensures the WAL is
   checkpointed before the backup:

   ```bash
   docker compose -f packaging/docker/docker-compose.yml stop binnacle
   docker cp binnacle:/var/lib/binnacle/binnacle.db ./binnacle-backup.db
   ```

3. Check the master key format. Releases after v0.3 accept the key only as 64
   hexadecimal characters or as base64 of 32 bytes; a raw 32-character value
   stops the container at startup with `master encryption key is invalid`.
   Re-encode the same bytes as hex so existing secrets stay readable:

   ```bash
   printf '%s' "$BINNACLE_MASTER_KEY" | od -An -tx1 | tr -d ' \n'
   ```

   Hex and base64 encodings of the same bytes open the same secrets, so no
   re-encryption is needed. Update the image tag and redeploy. If a master key
   is configured directly in the environment, migrate it to a Docker/Coolify
   secret and set `BINNACLE_MASTER_KEY_FILE=/run/secrets/binnacle_master_key`:

   ```bash
   docker compose -f packaging/docker/docker-compose.yml pull
   docker compose -f packaging/docker/docker-compose.yml up -d
   ```

4. Verify the container is healthy:

   ```bash
   curl -f http://127.0.0.1:8080/healthz
   ```

## Migrations

Binnacle runs forward-only SQLite migrations automatically at startup. Before migrating, it checks database integrity and available disk space. A failed migration is logged and the process stops; it does not delete or recreate the database.

Schema 19 adds TOTP and proxy-authentication state; schemas 20 and 21 add hashed
personal API-token metadata and typed, versioned administrator preferences. The
migration chain through schema 21 remains intact.

Downgrades are not supported. If you need to revert, restore from a backup taken before the upgrade.

## Behaviour changes to check

- Webhook signatures in releases after v0.6.0 cover `<X-Binnacle-Timestamp>.<body>` instead
  of the body alone. Update receivers that verify `X-Binnacle-Signature`
  before upgrading, or deliveries fail with their HTTP error until they are
  retried. See [Incidents and notifications](../INCIDENTS_AND_NOTIFICATIONS.md).
- A Coolify URL saved in Settings that resolves to a private address stops
  working unless the deployment sets `BINNACLE_COOLIFY_ALLOW_PRIVATE_TARGETS=true`
  or moves the URL to `BINNACLE_COOLIFY_URL`. Settings → Coolify reports the
  failed connection test with the setting name.
- Outbound checks and notifications additionally refuse carrier-grade NAT,
  NAT64, 6to4 and reserved ranges; see
  [Checks and alerts](../CHECKS_AND_ALERTS.md).

## Coolify upgrades

In Coolify, change the image tag in the service settings and redeploy. Coolify will recreate the container while reattaching the persistent volume.
