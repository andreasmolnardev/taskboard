# Historical Taskboard database fixture

`taskboard-75b827a-populated.zip` is a synthetic PocketBase backup generated
from Taskboard source commit
`75b827a741f3ff537a226f57191f0be5006931cb` (2026-10-11). The historical
Taskboard binary was built from that commit and run with an isolated
`SLOPSTACK_STORAGE_PATH` to bootstrap the database. A helper test compiled from
the same source snapshot cloned that database, seeded a synthetic user, todo,
event, and contact with stable record IDs, and called that version's
`CreateBackup` implementation. The archive contains no user data.

SHA-256: `3b44a7e64c6cc43c34c5391d8c3583057de61038bd722657cd69c2b94ef3149c`.

`apps/server/historical_source_fixture_test.go` restores the archive into a
new temporary directory, boots the current Taskboard runtime against a clone,
runs current migrations and schema repairs, and checks that historical record
IDs and values survive. The archive's migration IDs match the current app's
migration set; this fixture proves historical-source boot/backup/restore
compatibility, while `legacy_schema_compat_test.go` separately exercises a
truncated migration-history upgrade.
