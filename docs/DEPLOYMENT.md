# Deployment

## Docker

```sh
docker compose up --build
```

PocketBase data persists in the `slopstack-data` volume.

## Required production settings

- Set `SLOPSTACK_PUBLIC_URL` to the public origin.
- Set `SLOPSTACK_SECURE_COOKIES=true` behind HTTPS.
- Set a durable `SLOPSTACK_STORAGE_PATH`.
- Keep PocketBase data outside ephemeral container storage.

## Backups

Scheduled backups use PocketBase's transaction-safe backup engine. Docker enables one backup each day at 03:00 and keeps the newest 14 archives. Local archives are stored under `pb_data/backups`.

Set these values to change the schedule:

```sh
SLOPSTACK_BACKUP_ENABLED=true
SLOPSTACK_BACKUP_CRON="0 3 * * *"
SLOPSTACK_BACKUP_MAX_KEEP=14
```

Local archives share the same storage volume as the live database. This protects against bad writes, but not disk or volume loss. Production systems should use private S3-compatible storage:

```sh
SLOPSTACK_BACKUP_S3_ENABLED=true
SLOPSTACK_BACKUP_S3_BUCKET=taskboard-backups
SLOPSTACK_BACKUP_S3_REGION=us-east-1
SLOPSTACK_BACKUP_S3_ENDPOINT=https://s3.example.com
SLOPSTACK_BACKUP_S3_ACCESS_KEY=...
SLOPSTACK_BACKUP_S3_SECRET=...
SLOPSTACK_BACKUP_S3_FORCE_PATH_STYLE=false
```

Use a private bucket, TLS, a dedicated least-privilege key, versioning, and server-side encryption. Keep credentials in the deployment secret store. Do not commit them.

Use the server CLI for manual backup work. These commands use the configured local or S3 backup store:

```sh
slopstack backup create
slopstack backup list
slopstack backup restore <exact-name-from-list> --confirm
```

`backup create` prints the new archive name. `backup restore` rejects paths and other unsafe names, requires an exact archive name, and will not run without `--confirm`. Restore replaces the live PocketBase data and restarts the process. Stop traffic and other app instances first. There is no app HTTP restore endpoint.

Keep at least twice the data size free during backup and restore. Restore a copy into an isolated instance first when possible. Run a restore drill after setup and at least monthly. A backup is not proven until restore succeeds.

### Safe restore drill

Run the repository smoke test after the service is built and backup storage is configured:

```sh
./scripts/backup-restore-smoke-test.sh
```

The script creates a manual archive, then restores it into a temporary data directory through a one-off container. It never restores the live `slopstack-data` volume. With the compose defaults it stages the local archive automatically; when local storage has no copy, the isolated restore uses the configured S3-compatible store. The new manual archive remains in the configured backup store for normal retention cleanup.

For a different Compose file or service name, set `COMPOSE_FILE` or `BACKUP_COMPOSE_SERVICE`:

```sh
COMPOSE_FILE=/path/to/docker-compose.yml BACKUP_COMPOSE_SERVICE=slopstack ./scripts/backup-restore-smoke-test.sh
```

The drill needs Docker access and enough temporary disk space for the restored data. Do not put credentials in the script or command line; provide S3 credentials through the deployment secret store as described above.

## Frontend

The frontend is a static Vite build. Keep `/api` proxied to the Go runtime or set `VITE_API_URL` to the deployed API origin.

## Upgrades and rollback

Use a tagged release or a pinned image digest. Avoid deploying `latest`, so the exact prior app build is known. Run one app instance while PocketBase migrations run; do not start a new release beside the old instance against the same data directory.

Before each release:

1. Read the release notes and check for required configuration or manual steps.
2. Confirm the latest off-host backup is present. Create a manual backup with `slopstack backup create` and record the exact archive name.
3. Record the currently deployed release tag or image digest. Plan a maintenance window if the release changes schema or causes downtime.
4. Stop new traffic, then stop the app before replacing its image or binary.

For a source checkout using this repository's Compose file, build the release while the current service is still running, then stop and replace it:

```sh
git checkout <release-tag>
docker compose build slopstack
docker compose stop slopstack
docker compose up -d --no-deps slopstack
```

PocketBase applies registered migrations during startup. Watch startup logs until migrations finish, then check the health endpoint and sign in. Smoke test create, edit, delete, search, import/export, and any CalDAV or CardDAV clients you use before reopening traffic.

### Roll back

First stop traffic and stop the app. If the migration was explicitly designed to keep the prior release compatible, redeploy the recorded prior image and start one instance. Check the health endpoint and verify data before reopening traffic.

For a source checkout, use the same Compose steps with the recorded prior release tag.

If the prior release cannot use the new schema, restore the pre-upgrade backup instead of trying to reverse an applied migration. Keep the app stopped, select the exact archive recorded before the upgrade, and use the documented `slopstack backup restore <archive> --confirm` CLI. Then deploy the prior app release and start it once. Restoring replaces current data, including writes made after that backup. Test the restore on an isolated copy first when possible.

Treat migrations as forward-only in production: never edit an applied migration or assume its down function is a safe release rollback. Add a new migration for later schema changes. Keep the app release, migration code, and backup archive together in the release notes so the operator can identify a compatible rollback path.
