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
