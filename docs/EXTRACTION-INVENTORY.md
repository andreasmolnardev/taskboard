# Taskboard — Extraction Inventory (Phase 0)

Every `.go` file in `apps/server/internal/app`, `internal/calendar`, and
`internal/reminders`, classified for extraction into shared, versioned Slopstack
Go packages.

## Legend

| class | meaning |
| --- | --- |
| **CORE** | genuinely generic, no Taskboard domain knowledge. Should move upstream into a Slopstack package. Blocking dependencies on Taskboard types are noted. |
| **OPTIONAL-SHARED** | plausible candidate but the abstraction boundary is not obvious. Needs a design decision before extraction. |
| **APP-OWNED** | Taskboard domain logic. Stays in Taskboard. |

**Verdict rule of thumb:** if a file references `todos`, `events`, `lists`,
`calendars`, `address_books`, `contacts`, iCalendar/vCard semantics, or Taskboard
route shapes, it is APP-OWNED even if the *algorithm* is generic — the upstream
package would end up with Taskboard's schema baked into it.

Counts across all 44 `.go` files in the three packages (24 non-test in
`internal/app` plus 6 there, 2 in `internal/calendar`, 4 in `internal/reminders`):
**17 CORE**, **7 OPTIONAL-SHARED**, **20 APP-OWNED**. Non-test line totals:
`internal/app` 8 808 lines across 24 files, `internal/calendar` 187, `internal/reminders` 314.

---

## 1. `apps/server/internal/app` — 40 files

### 1.1 CORE (extract to slopstack)

| file | lines | proposed upstream package | notes |
| --- | --- | --- | --- |
| `config.go` | 117 | `pkg/config` | Env loading is generic (`env`/`envBool`/`envInt` are pure helpers). **But** the field set and `SLOPSTACK_*` prefixes are Taskboard-specific, and `BackupJSONEnabled`/`validateBackupSchedule` are Taskboard additions. Extract the *loader primitives* + a `Config` interface; keep the concrete struct in the app. |
| `openapi.go` | 15 | `pkg/openapi` | `OpenAPIExportCommand` is 100% generic cobra wiring with a `Run` func. Zero domain knowledge. |
| `app.go` (partial) | 249 | split | `Register()` composition root, `activationCode()`, `provisionDefaultContainers()`, `protectUserAdminFlag()`, `newOpenAPI()`, `registerExportCommand()`, `registerAuthRoutes()` (registration-policy route) are extractable. The route table and `registerTodoAPI`/`registerTodoRoutes` are not. See §4. |
| `backup_encryption.go` | 124 | `pkg/backup` | AES-GCM key management is entirely generic. Taskboard has no domain knowledge here at all. |
| `backup_formats.go` | 183 | `pkg/backup` | Creates/restores PocketBase backup archives. PocketBase-level, not Taskboard-level. Contains `cleanupTransientBackupArchives`, which `app.go` calls at bootstrap. |
| `backup_scheduler.go` | 84 | `pkg/backup` | Generic cron + retention pruning of `.zip.enc`/`.json.enc`. Purely filesystem/PocketBase. |
| `backup_command.go` | 230 | `pkg/backup` | Cobra command tree. Generic except for the `SLOPSTACK_BACKUP_RESTORE_RESTARTED` env var name and the `.json` subcommand (Taskboard-specific export format). Flagged: it depends on `loadBackupKey`/`createEncryptedArchive` so it must move together with the two files above. |
| `auth.go` | 24 | `pkg/auth` | `CurrentUser()` is generic; the three `RegistrationRequires*` helpers wrap `Config.RegistrationMode` and would need a config abstraction. |
| `authorization.go` | 15 | `pkg/auth` | `Authorize()` is the intended single enforcement point. Currently a stub (superuser-or-deny) — extract the *interface*, then implement the group lookup once, here, for every derived app. |
| `guest_auth.go` | 49 | `pkg/auth` | Generic dev-only guest bootstrap. Taskboard-specific: the email literal `guest@taskboard.local`. That is the only reason it is not listed as unconditionally CORE — make the address configurable and it is pure. |
| `app_passwords.go` | 150 | `pkg/auth` | HTTP Basic app-password auth + CRUD routes. The *routes* are Taskboard-shaped (`/api/auth/app-passwords`), the auth core is generic. Strong CORE candidate for the auth part; the route handlers need a mount-point parameter. |
| `app_password_throttle.go` | 130 | `pkg/auth` | Pure in-memory sliding-window limiter. Zero domain knowledge. |
| `ownership.go` | 179 | `pkg/auth` **or APP-OWNED** | *Ambiguous.* The *pattern* (override `owner` from `e.Auth.Id`, reject foreign records, validate a container reference) is generic; the concrete rules reference `todos`/`events`/`lists`/`calendars`/`contacts` and the messages ("non-empty containers must be archived") are contract. Recommendation: extract a `OwnershipEnforcer` interface with an app-supplied collection/field spec, keep the Taskboard spec local. Listed CORE because the enforcement algorithm is the reusable part. |
| `schema.go` | 102 | `pkg/schema` | `EnsureCollections` is a generic "declarative top-up" repair loop; only the *table* is Taskboard's. Extract the loop (`addMissingFields`, `addFields`, rule application), keep the table in the app. |
| `caldav_changes.go` | 152 | `pkg/dav` | Sync-journal pattern is generic DAV infrastructure. **Taskboard-specific:** `registerCalDAVChangeHooks` is hard-coded to `events`/`todos`. Extract the journal; parameterise the hooks. |
| `carddav_changes.go` | 127 | `pkg/dav` | Same as above for `contacts`. Taskboard-only file (Piggybudget has no equivalent) but structurally the same pattern. |
| `audit logging` | — | `pkg/audit` | **Missing.** The `audit_logs` collection is declared in `schema.go` but **no code ever writes to it.** Either extract a logging primitive or delete the collection — see `docs/AGENT-NOTES.md`. |

### 1.2 OPTIONAL-SHARED (needs review)

| file | lines | the tension |
| --- | --- | --- |
| `notifications.go` | 506 | Contains three separable things: (a) `EnsureNotificationCollections` — generic schema repair, CORE-ish; (b) the notification/reminder **HTTP routes** with camelCase response shapes — contract, APP-OWNED; (c) `PocketBaseReminderStore` + `reminderTargetCollection` — generic PocketBase persistence adapter. The `in_app`-only channel restriction is a product decision. Needs a split before extraction. |
| `http_integration_test.go` | 509 | Cross-route integration tests. Valuable to share as a conformance suite for the extracted packages, but it asserts Taskboard routes/collections. Recommend: move the generic assertions into `pkg/*_test`, keep Taskboard-specific ones local. |
| `schema_test.go` | 157 | Asserts the Taskboard table. Would need splitting alongside `schema.go`. |
| `backup_command_test.go` | 149 | Tests `NewBackupCommand` through a fake `core.App`. Likely reusable verbatim if the command moves. |
| `backup_encryption_test.go` | 155 | Pure crypto round-trip tests. Generic — should move with `backup_encryption.go` (arguably CORE). |
| `app_passwords_test.go` | 181 | Mostly generic throttling/auth tests; the route tests are Taskboard-shaped. Needs splitting. |
| `caldav_xml.go` | 152 | XML request-body parsing for `PROPFIND`/`REPORT`/`sync-collection`. Generic DAV plumbing, but the property names and `calDAVTimeRange`/`calDAVPropFilter` types are entangled with `caldav.go`'s Taskboard property mappers. Only reusable once `caldav.go` is itself refactored. |

### 1.3 APP-OWNED

| file | lines | why |
| --- | --- | --- |
| `caldav.go` | 977 | Calendar-access protocol over Taskboard's `calendars`/`events`/`todos` + `dav_href`/`dav_data`. Taskboard-specific property mapping and href shapes. |
| `carddav.go` | 876 | Addressbook protocol over `address_books`/`contacts`. |
| `vcard.go` | 395 | vCard 3.0 parse/encode. **Taskboard-only file** (Piggybudget has none). Generic as a format library, but it encodes Taskboard's contact field mapping (`projectVCardToContact`, `sortVCardProperties` ordering) — keep local unless a standalone `pkg/vcard` is worth it. |
| `calendar_io.go` | 596 | ICS import/export bound to Taskboard's field names (`start_local`, `time_mode`, `alarm`, `dav_data`). |
| `occurrences.go` | 480 | Recurrence occurrence/override/cancel over `events` + `recurrence_parent`/`recurrence_id`. |
| `search.go` | 292 | The `searchCollections` table is Taskboard's (which collections, which fields, which `type` label). The paging envelope could be shared; the table cannot. |
| `caldav_test.go`, `carddav_test.go`, `carddav_changes` tests | — | Domain tests. |

---

## 2. `apps/server/internal/calendar` — 2 files

| file | class | rationale |
| --- | --- | --- |
| `recurrence.go` (187) | **CORE** | `Expand(component *ical.Component, window, max) ([]Occurrence, error)` with `ErrInvalidWindow`/`ErrInvalidLimit`/`ErrLimitExceeded`. Pure iCalendar/RRULE math over `go-ical` + `rrule-go`. **Byte-identical to Piggybudget.** This is the single cleanest extraction candidate in the repo — it has no Taskboard type references at all. |
| `recurrence_test.go` (137) | **CORE** | Moves with it; also byte-identical. |

Note: `occurrences.go` (APP-OWNED) consumes this package. Extraction must keep the
import path stable or provide a shim so the app-owned half still compiles.

## 3. `apps/server/internal/reminders` — 4 files

| file | class | rationale |
| --- | --- | --- |
| `reminder.go` (150) | **OPTIONAL-SHARED** | `Reminder`, `TargetKind`, `Channel` (`ChannelInApp` only), `RetryPolicy`. Generic *shape*, but `TargetKind` values are Taskboard's ("todo"/"event") and `Channel` currently admits exactly one channel. Shared only if downstream apps agree on a channel/target vocabulary. |
| `scheduler.go` (62) | **CORE** | `Scheduler.RunDue` with atomic claim + per-item result recording. Fully generic, no Taskboard types. All four files are **byte-identical to Piggybudget.** |
| `memory_store.go` (102) | **CORE** | In-memory `Store` implementation. Test double today, but generic. |
| `reminders_test.go` (188) | **CORE** | Byte-identical to Piggybudget; moves with the package. |

All four reminders files being byte-identical to Piggybudget makes
`internal/reminders` a **zero-risk** first extraction.

---

## 4. `app.go` — the file that must be split, not moved

`app.go` (249 lines) mixes five concerns. Proposed split:

| concern | destination |
| --- | --- |
| `activationCode()`, `protectUserAdminFlag()` | `pkg/auth` (registration policy engine) |
| `provisionDefaultContainers()` | APP-OWNED — hard-codes Taskboard's `Tasks` / `Events and Holidays` / `Contacts` rows |
| `applyBackupConfig()`, `validateBackupSchedule()` usage | `pkg/backup` |
| `newOpenAPI()` security-scheme wiring, `registerExportCommand` | `pkg/openapi` |
| `registerTodoAPI()`, `registerTodoRoutes()`, the `registerRoutes` table | APP-OWNED |

---

## 5. Files that exist in **both** Taskboard and Piggybudget

Comparison method: line-by-line `difflib` over `apps/server/internal/**/*.go`.
Piggybudget was read read-only; nothing was written there.

### 5.1 Byte-identical (highest-priority extraction targets)

| file | lines | class |
| --- | --- | --- |
| `internal/app/app_password_throttle.go` | 130 | CORE |
| `internal/app/app_passwords.go` | 150 | CORE |
| `internal/app/app_passwords_test.go` | 181 | OPTIONAL-SHARED |
| `internal/app/auth.go` | 24 | CORE |
| `internal/app/authorization.go` | 15 | CORE |
| `internal/app/caldav_changes.go` | 152 | CORE (journal) |
| `internal/app/calendar_io.go` | 596 | APP-OWNED |
| `internal/app/calendar_io_test.go` | 391 | APP-OWNED |
| `internal/app/guest_auth.go` | 49 | CORE |
| `internal/app/guest_auth_test.go` | — | CORE |
| `internal/app/notifications.go` | 506 | OPTIONAL-SHARED |
| `internal/app/notifications_test.go` | — | OPTIONAL-SHARED |
| `internal/app/occurrences.go` | 480 | APP-OWNED |
| `internal/app/occurrences_test.go` | — | APP-OWNED |
| `internal/app/openapi.go` | 15 | CORE |
| `internal/app/openapi_test.go` | — | CORE |
| `internal/app/ownership.go` | 179 | CORE / APP-OWNED |
| `internal/app/ownership_test.go` | — | CORE / APP-OWNED |
| `internal/app/search.go` | 292 | APP-OWNED |
| `internal/app/search_test.go` | — | APP-OWNED |
| `internal/calendar/recurrence.go` + test | 187 + 137 | CORE |
| `internal/reminders/memory_store.go` | 102 | CORE |
| `internal/reminders/reminder.go` | 150 | OPTIONAL-SHARED |
| `internal/reminders/reminders_test.go` | 188 | CORE |
| `internal/reminders/scheduler.go` | 62 | CORE |

**24 files are byte-identical across the two repos.** Roughly half of them are
genuinely generic. Every future divergence in these files is duplicated work —
this is the strongest justification for the extraction phase.

> **Correction to the brief's premise.** The stated direction was "Piggybudget
> evolved from Taskboard". The diff says the opposite for several files:
> `calendar_io.go`, `occurrences.go`, `search.go`, `notifications.go`,
> `ownership.go`, `caldav_changes.go` are *identical in both*, while Piggybudget
> **added** `finance.go`, `finance_schema.go`, `asset_groups.go`,
> `transaction_attachments.go` (+ tests, + 4 migrations) and **lacks** Taskboard's
> `backup_encryption.go`, `backup_formats.go`, `backup_scheduler.go`,
> `carddav_changes.go`, `vcard.go`. Taskboard's backup-encryption work
> (commit `640ebf1`, "Add encrypted backup and JSON export support") is **ahead**
> of Piggybudget. So it is not a clean linear fork — expect bidirectional
> divergence, not just Taskboard lagging.

### 5.2 Same file, substantial divergence

| file | identical lines | Taskboard-only content |
| --- | --- | --- |
| `internal/app/carddav.go` | 138 / 876 (16%) | Taskboard's CardDAV is ~2.6× Piggybudget's (876 vs 333 lines) — `PROPFIND`/`REPORT`/`sync-collection`/ETag/CTag are implemented; Piggybudget's is a stub. Taskboard is ahead. |
| `internal/app/backup_command.go` | 145 / 230 (63%) | `.zip.enc` + `.json.enc`, `key-init`, `key-import`, `json` subcommands. Piggybudget only has plain `.zip` via `pb.CreateBackup`. Taskboard is ahead. |
| `internal/app/config.go` | 81 / 117 (69%) | `BackupJSONEnabled`, `validateBackupSchedule`. Piggybudget sets `settings.Backups.Cron = config.BackupCron`; Taskboard zeroes it and schedules its own. **This is a real behavioural fork, not a naming one.** |
| `internal/app/caldav.go` | 842 / 977 (86%) | Taskboard adds `dav_data` round-tripping (`preservedCalDAVData`, `sameCalDAVKnownProperties`) and `PROPFIND` prop-filter support. |
| `internal/app/caldav_xml.go` | 136 / 152 (89%) | Taskboard adds `calDAVPropFilter` / `calDAVTextMatch` (CardDAV-style property filtering in CalDAV REPORT). |
| `internal/app/app.go` | 215 / 249 (86%) | Taskboard: `registerTodoAPI`/`registerTodoRoutes`, `cleanupTransientBackupArchives`, `registerEncryptedBackupScheduler`, `EnsureCardDAVChanges`. Piggybudget: `registerFinanceAPI`, `registerFinanceHooks`, `RegisterAssetGroupHooks/Routes`, `RegisterTransactionAttachmentHooks/Routes`, a different bootstrap order, and a comment about keeping DAV protocol fixtures purely for tests. |
| `internal/app/schema.go` | 96 / 102 (94%) | Taskboard declares `dav_data` on `todos`/`events` and calls `EnsureCardDAVChanges`; Piggybudget calls `EnsureFinanceCollections` instead and has no `dav_data`. |

### 5.3 Files only in one repo

| Taskboard only | Piggybudget only |
| --- | --- |
| `backup_encryption.go`, `backup_encryption_test.go` | `finance.go`, `finance_test.go` |
| `backup_formats.go` | `finance_schema.go` |
| `backup_scheduler.go` | `asset_groups.go`, `asset_groups_test.go` |
| `carddav_changes.go` | `transaction_attachments.go`, `transaction_attachments_test.go` |
| `vcard.go` | |

Differing tests: `caldav_test.go`, `carddav_test.go`, `backup_command_test.go`,
`http_integration_test.go`, `openapi_registration_test.go`, `schema_test.go`.

Before the identity migration, both repos also shared `module
github.com/slopstack/slopstack`; the current shared module is
`github.com/andreasmolnardev/slopstack`. Each app now keeps its own module path.
`apps/server/cmd/slopstack` binary name, the `@slopstack/web` package name, the
`SLOPSTACK_*` env prefix, and the identical `pb_migrations` prefix
(`20260824000000_add_user_is_admin.go` … `20260915000000_add_todo_calendar.go`).
Migrations diverge after that point: Taskboard adds `20260920000000_add_dav_resource_data`
+ `20260920000001_add_carddav_change_journal`; Piggybudget adds
`20260920000000_add_finance_collections`, `20260920000001_extend_finance_collections`,
`20261005000000_add_transaction_attachments`, `20261005000001_add_asset_groups_and_positions`.
**The shared migration history is only safe up to `20260915000000`.** Anything
extracted upstream must not assume a common post-`20260915000000` schema.

---

## 6. Recommended extraction order

Ordered by (confidence ÷ risk). Each step must leave `go build ./...` and
`go test ./...` green.

1. **`internal/reminders`** (4 files, byte-identical, zero Taskboard types) →
   `pkg/reminders`. Simplest possible first move.
2. **`internal/calendar`** (2 files, byte-identical) → `pkg/icalrecurrence`. One
   consumer (`occurrences.go`) to repoint.
3. **`backup_encryption.go` + `backup_formats.go` + `backup_scheduler.go` +
   `backup_command.go`** → `pkg/backup`. Taskboard-only work, but it is the
   feature Piggybudget needs, so upstreaming it is net-new value.
4. **`auth.go` + `authorization.go` + `app_passwords.go` +
   `app_password_throttle.go` + `guest_auth.go`** → `pkg/auth`. Byte-identical
   across both repos. Requires deciding how `RegistrationMode` and the
   guest email become parameters.
5. **`openapi.go`** → `pkg/openapi`. Trivial.
6. **`schema.go`'s repair loop** → `pkg/schema`, keeping the Taskboard table
   local. Requires splitting the file.
7. **`caldav_changes.go` / `carddav_changes.go` journals** → `pkg/dav`, after
   parameterising the record hooks.
8. **`ownership.go`** → last, because it is the one CORE candidate that is
   genuinely also domain-bound. Needs the `OwnershipEnforcer` interface design
   before anything moves.

Explicitly **not** extracting: `caldav.go`, `carddav.go`, `vcard.go`,
`calendar_io.go`, `occurrences.go`, `search.go`, `notifications.go` routes,
`provisionDefaultContainers`, `registerTodo*`.

## 7. Known ambiguities

- **`ownership.go` CORE vs APP-OWNED.** Classified CORE with a caveat. If the
  interface design fails, it stays.
- **`reminder.go`.** Core scheduler + app-specific `TargetKind`/`Channel` in one
  file; may need to split before extraction.
- **`notifications.go`.** Three concerns in 506 lines; cannot move whole.
- **`audit_logs`.** Declared in `schema.go`, written by nothing. Not classifiable
  as an extraction candidate because there is no code — it is dead schema.
- **`authorization.go`.** Classified CORE as a stub. Once groups/permissions are
  implemented it becomes much more valuable to share, but that implementation is
  new work, not extraction.
- **`app_passwords.go`.** The auth core is CORE; the three HTTP routes are
  Taskboard-shaped. Needs a mount point parameter before it can move.
- **Cross-repo direction.** The premise "Piggybudget evolved from Taskboard" does
  not hold uniformly — see §5.1 note. Extraction should assume divergence in
  both directions.
