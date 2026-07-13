# Recovery

## Disk-full condition

When the database or WAL grows past configured thresholds, Binnacle enters a degraded persistence state:

- **Warning** — expired data is cleaned aggressively.
- **Critical** — additional expired cleanup runs.
- **Emergency** — raw 10-second persistence pauses; rollups, settings, and events are preserved.

The live Metrics Engine and SSE continue to work during storage pressure. Free
disk space or reduce retention. Binnacle reevaluates the database budget every
minute and automatically resumes raw persistence after usage falls below the
emergency threshold; no restart is required.

## Corruption

If startup migration fails with an integrity error:

1. Stop the container.
2. Copy the database files to a safe location:

   ```bash
   docker cp binnacle:/var/lib/binnacle /tmp/binnacle-recovery
   ```

3. Attempt an integrity check on a copy:

   ```bash
   sqlite3 /tmp/binnacle-recovery/binnacle.db "PRAGMA integrity_check;"
   ```

4. If the database is corrupt, restore from your most recent backup or start with a fresh database. Binnacle does not automatically repair or delete a corrupt database.

## Consistent database backup

The production image does not include the SQLite CLI. Stop Binnacle before
copying the database so the process closes SQLite and checkpoints its WAL:

```bash
docker compose -f packaging/docker/docker-compose.yml stop binnacle
docker cp binnacle:/var/lib/binnacle/binnacle.db ./binnacle-backup.db
docker compose -f packaging/docker/docker-compose.yml start binnacle
```

Do not copy an open `binnacle.db` without its WAL. For an online backup, use a
trusted host-side SQLite tool and its backup API against the persistent volume.

## Reset monitoring history

From the Settings page you can delete history for one resource, data before a date, or all monitoring history. These operations require typed confirmation and run in bounded batches. They do not delete users or configuration.

## Restart and logs

```bash
docker compose -f packaging/docker/docker-compose.yml restart binnacle
docker compose -f packaging/docker/docker-compose.yml logs -f binnacle
```

Check `level=ERROR` entries for migration, disk, or collector failures.
