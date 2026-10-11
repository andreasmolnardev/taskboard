# Deployment

## Docker

```sh
docker compose up --build
```

PocketBase data persists in the existing `slopstack-data` volume. Keep this volume name when upgrading an existing Compose deployment.

`slopstack.yaml` follows the canonical schema v1 and records Taskboard's
identity, environment prefix, runtime defaults, tool versions, image targets,
and release version. Check it with `pnpm config:check` before changing
deployment settings. Keep credentials in environment variables or mounted
secret files.

Preview a multi-platform Buildx build with:

```sh
pnpm deploy
```

This is a dry run. It prints the command and does not build or push. The Go
build downloads the pinned Slopstack runtime from the Go module proxy; a sibling
Slopstack checkout is not required. After reviewing the command and configuring
Docker credentials, `pnpm deploy --push` executes it and pushes the configured
image.

Plan a release version change with:

```sh
pnpm release --version 1.2.3
```

This is also a dry run. `pnpm release --version 1.2.3 --apply` updates only
`release.version` and `image.tag` in `slopstack.yaml`; it creates no Git tag and
does not publish an image or release. Review and commit the metadata update,
then deploy separately.

Compose and the image both check PocketBase's `/api/health` endpoint. Compose
allows 30 seconds for the process to stop so PocketBase can close cleanly.

## Required production settings

- Set `TASKBOARD_PUBLIC_URL` to the public origin.
- Set `TASKBOARD_SECURE_COOKIES=true` behind HTTPS.
- Set a durable `TASKBOARD_STORAGE_PATH`.
- Existing deployments may continue to set `SLOPSTACK_*`; matching `TASKBOARD_*` variables take precedence.
- Compose retains the existing `slopstack-data` volume and `/app/pb_data` mount so existing data stays at the same location.
- Keep PocketBase data outside ephemeral container storage.

## Backups

Scheduled backups use PocketBase's transaction-safe backup engine. Docker enables one backup each day at 03:00 and keeps the newest 14 archives. Local archives are stored under `pb_data/backups`.

Set these values to change the schedule:

```sh
TASKBOARD_BACKUP_ENABLED=true
TASKBOARD_BACKUP_CRON="0 3 * * *"
TASKBOARD_BACKUP_MAX_KEEP=14
```

Local archives share the same storage volume as the live database. This protects against bad writes, but not disk or volume loss. Production systems should use private S3-compatible storage:

```sh
TASKBOARD_BACKUP_S3_ENABLED=true
TASKBOARD_BACKUP_S3_BUCKET=taskboard-backups
TASKBOARD_BACKUP_S3_REGION=us-east-1
TASKBOARD_BACKUP_S3_ENDPOINT=https://s3.example.com
TASKBOARD_BACKUP_S3_ACCESS_KEY=...
TASKBOARD_BACKUP_S3_SECRET=...
TASKBOARD_BACKUP_S3_FORCE_PATH_STYLE=false
```

Use a private bucket, TLS, a dedicated least-privilege key, versioning, and server-side encryption. Keep credentials in the deployment secret store. Do not commit them.

Use the server CLI for manual backup work. These commands use the configured local or S3 backup store:

```sh
taskboard backup create
taskboard backup list
taskboard backup restore <exact-name-from-list> --confirm
```

`backup create` prints the new archive name. `backup restore` rejects paths and other unsafe names, requires an exact archive name, and will not run without `--confirm`. Restore replaces the live PocketBase data and restarts the process. Stop traffic and other app instances first. There is no app HTTP restore endpoint.

Keep at least twice the data size free during backup and restore. Restore a copy into an isolated instance first when possible. Run a restore drill after setup and at least monthly. A backup is not proven until restore succeeds.

### Safe restore drill

Run the disposable-fixture compatibility check:

```sh
pnpm compatibility:fixture
```

This checks the Go module identity, builds the CLI into a temporary directory,
creates a new PocketBase database there, initializes a fixture-only recovery
key, writes an encrypted archive, and restores it. The script disables S3 for
its subprocesses and removes only its own temporary directory on exit. It does
not mount the Compose volume or contact the configured backup store. Use it
after changes to backup encryption or module identity. CI runs it using the
published Slopstack module version pinned in `go.mod`.

The legacy smoke-test path delegates to the same disposable fixture check:

```sh
./scripts/backup-restore-smoke-test.sh
```

The check needs the pinned Go toolchain and temporary disk space. It does not
need Docker credentials or production backup credentials.

Before release, run the disposable fixture check on each supported platform.
Startup validates backup cron and retention settings before running application
migrations. Take and verify an encrypted backup before deploying a new version;
the startup check cannot protect against an application migration defect.

## Frontend

The frontend is a static Vite build copied into the image and served by the Go
runtime. `TASKBOARD_PUBLIC_DIR` selects the static directory and defaults to
`public`; client-side routes fall back to `index.html`, while API, DAV, and admin
paths remain backend-owned. During development, Vite serves the frontend and
proxies `/api` to PocketBase. Set `VITE_API_URL` when the API uses a different
public origin.
