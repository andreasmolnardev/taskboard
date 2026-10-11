#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fixture_dir=$(mktemp -d "${TMPDIR:-/tmp}/taskboard-compatibility.XXXXXX")
trap 'rm -rf "$fixture_dir"' EXIT HUP INT TERM

cd "$repo_dir"
node scripts/check-taskboard-config.mjs
if [ -z "${GOCACHE:-}" ]; then
	export GOCACHE="$fixture_dir/go-cache"
fi

module_path=$(go list -m)
if [ "$module_path" != "github.com/andreasmolnardev/taskboard" ]; then
	echo "unexpected Go module identity: $module_path" >&2
	exit 1
fi

go build -o "$fixture_dir/taskboard" ./apps/server/cmd/taskboard
if ! "$fixture_dir/taskboard" --help | grep -Fq "Taskboard application server"; then
	echo "built CLI does not identify itself as Taskboard" >&2
	exit 1
fi

taskboard() {
	TASKBOARD_STORAGE_PATH="$fixture_dir/pb_data" \
	TASKBOARD_BACKUP_ENABLED=true \
	TASKBOARD_BACKUP_S3_ENABLED=false \
	TASKBOARD_PUBLIC_URL=http://localhost:8090 \
	"$fixture_dir/taskboard" "$@"
}

taskboard backup key-init >"$fixture_dir/key-init.out"
archive=$(taskboard backup create | awk 'NF { line = $0 } END { print line }')
if ! printf '%s\n' "$archive" | grep -Eq '^manual_[0-9_]+\.zip\.enc$'; then
	echo "backup create returned an unexpected archive name: $archive" >&2
	exit 1
fi
taskboard backup list | grep -Fq "$archive"
taskboard backup restore "$archive" --confirm

if [ ! -s "$fixture_dir/pb_data/data.db" ]; then
	echo "restore did not leave a database in the disposable fixture" >&2
	exit 1
fi
if [ ! -s "$fixture_dir/pb_data/.backup-encryption-key" ]; then
	echo "restore removed the disposable fixture's encryption key" >&2
	exit 1
fi

printf 'Taskboard identity, build, and encrypted backup restore checks passed (%s)\n' "$archive"
