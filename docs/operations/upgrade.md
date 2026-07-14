# Upgrade

Binnacle does not self-update. Upgrades are performed by replacing the container image.

## Update channels

Container tags follow semantic versioning:

- `stable` — release builds only (no prereleases).
- `beta` — beta and release-candidate builds.
- `edge` — development builds.
- Exact version tags such as `v0.6.0` are immutable.

The v0.6 upgrade applies schemas 20 and 21. Back up `binnacle.db` and its
WAL/SHM files before replacing the binary. Existing resources, alerts, checks,
incidents, sessions, secrets, settings, and history are preserved.

Pick a channel in your Compose file or Coolify service settings:

```yaml
image: ghcr.io/drilonrecica/binnacle:stable
```

## Upgrade process

1. Stop Binnacle and copy the closed SQLite database. This ensures the WAL is
   checkpointed before the backup:

   ```bash
   docker compose -f packaging/docker/docker-compose.yml stop binnacle
   docker cp binnacle:/var/lib/binnacle/binnacle.db ./binnacle-backup.db
   ```

2. Update the image tag and redeploy:

   ```bash
   docker compose -f packaging/docker/docker-compose.yml pull
   docker compose -f packaging/docker/docker-compose.yml up -d
   ```

3. Verify the container is healthy:

   ```bash
   curl -f http://127.0.0.1:8080/healthz
   ```

## Migrations

Binnacle runs forward-only SQLite migrations automatically at startup. Before migrating, it checks database integrity and available disk space. A failed migration is logged and the process stops; it does not delete or recreate the database.

Schemas 20 and 21 add hashed personal API token metadata and typed,
versioned administrator preferences. The migration chain from schemas 17
through 21 preserves resources, history, settings, alerts, incidents, sessions,
and encrypted secrets.

Downgrades are not supported. If you need to revert, restore from a backup taken before the upgrade.

## Coolify upgrades

In Coolify, change the image tag in the service settings and redeploy. Coolify will recreate the container while reattaching the persistent volume.
