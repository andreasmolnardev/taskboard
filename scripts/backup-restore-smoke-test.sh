#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(dirname "$script_dir")
compose_file=${COMPOSE_FILE:-"$repo_dir/docker-compose.yml"}
service=${BACKUP_COMPOSE_SERVICE:-slopstack}

if ! command -v docker >/dev/null 2>&1; then
	echo "docker is required" >&2
	exit 1
fi
if [ ! -f "$compose_file" ]; then
	echo "compose file not found: $compose_file" >&2
	exit 1
fi

compose() {
	docker compose -f "$compose_file" "$@"
}

archive=$(compose run --rm --no-deps "$service" backup create | awk 'NF { line = $0 } END { print line }')
if ! printf '%s\n' "$archive" | grep -Eq '^@?[a-z0-9_-]+\.zip$'; then
	echo "backup create did not return an archive name" >&2
	exit 1
fi

# RestoreBackup replaces its data directory, so use a host temp directory rather
# than the live compose volume. Root is used only for the temporary bind mount.
drill_dir=$(mktemp -d "${TMPDIR:-/tmp}/taskboard-backup-restore.XXXXXX")
cleanup() {
	rm -rf "$drill_dir"
}
trap cleanup EXIT

mkdir -p "$drill_dir/backups"
if compose run --rm --no-deps "$service" sh -c 'test -f "/app/pb_data/backups/$1"' sh "$archive"; then
	compose run --rm --no-deps --user root -v "$drill_dir:/drill" "$service" \
		sh -c 'cp "/app/pb_data/backups/$1" "/drill/backups/$1"' sh "$archive"
else
	echo "local archive not found; restore will use the configured remote backup store" >&2
fi

compose run --rm --no-deps --user root -v "$drill_dir:/app/pb_data" "$service" \
	backup restore "$archive" --confirm

if [ ! -s "$drill_dir/data.db" ]; then
	echo "restore completed without a data.db in the isolated data directory" >&2
	exit 1
fi

printf 'backup restore smoke test passed: %s\n' "$archive"
