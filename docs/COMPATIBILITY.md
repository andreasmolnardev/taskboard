# Taskboard — Compatibility Contract and Runtime Status

## Current runtime snapshot

The detailed inventory below is a historical Phase 0 baseline frozen at commit
`75b827a`. It records the source and behavior at that revision; it is not a
description of the current checkout. The current runtime facts below were
checked against the current source tree.

- The server executable and PocketBase root command are named `taskboard`.
- The server exposes the shared backup command tree as `taskboard backup`.
  Supported subcommands are `create`, `json`, `key-init`, `key-import`,
  `list`, and `restore`.
- Runtime configuration uses `TASKBOARD_*`. For matching settings, the shared
  config lookup still accepts the old `SLOPSTACK_*` name; `TASKBOARD_*` wins
  when both are set. The restore re-exec guard is
  `TASKBOARD_BACKUP_RESTORE_RESTARTED`.
- Taskboard serves live Huma Todo CRUD operations through the PocketBase router:
  `GET` and `POST /api/todos`, plus `GET`, `PATCH`, and `DELETE
  /api/todos/{id}`. Huma handlers use the authenticated PocketBase request
  event and scope every read and write to that user. The existing PocketBase
  collection API remains available to the web client.

The sections marked **Phase 0 historical baseline** below preserve the exact
older snapshot and its original command names and environment variables.

---

## Phase 0 historical baseline

### 1. Baseline commands and results

Run from `apps/server` and the repo root respectively.

| # | command | cwd | result |
| --- | --- | --- | --- |
| 1 | `go build ./...` | `apps/server` | **PASS** |
| 2 | `go test ./...` | `apps/server` | **PASS** |
| 3 | `pnpm -r lint` | repo root | **PASS** (eslint over `apps/web/src`) |
| 4 | `pnpm -r test` | repo root | **PASS** — 11 test files, 34 tests |

`go test ./...` detail:

```
?   github.com/slopstack/slopstack/apps/server/cmd/slopstack  [no test files]
ok  github.com/slopstack/slopstack/apps/server/internal/app       20.3s
ok  github.com/slopstack/slopstack/apps/server/internal/calendar   1.4s
ok  github.com/slopstack/slopstack/apps/server/internal/reminders  1.1s
?   github.com/slopstack/slopstack/apps/server/pb_migrations      [no test files]
```

`pnpm -r test` runs `vitest run --passWithNoTests` in `apps/web`. The 11 test
files / 34 tests are:

```
src/data.test.ts
src/theme.test.ts
src/components/calendar-month.test.tsx
src/components/full-month-calendar.test.tsx
src/components/container-manager/container-manager.test.tsx
src/components/tabs/lists-tab.test.tsx
src/components/app/create-composer.helpers.test.ts
src/components/time-grid/layout.test.ts
src/components/time-grid/position.test.ts
src/components/time-grid/segments.test.ts
src/components/time-grid/time-grid.test.tsx
```

`pnpm -r build` (`tsc -b && vite build`) also passes but is not part of the
recorded baseline; CI runs it.

### 2. Tool versions (Phase 0 historical baseline)

| tool | version |
| --- | --- |
| go | `go1.27.1 darwin/arm64` |
| node | `v22.23.2` |
| pnpm | `11.8.0` (pinned via `packageManager` in root `package.json`) |
| go directive in `go.mod` | `go 1.26.0` |
| PocketBase (Go dep) | `github.com/pocketbase/pocketbase v0.39.6` |
| Huma | `github.com/danielgtaylor/huma/v2 v2.38.0` |
| chi (via PocketBase) | `v5.3.1` |
| cobra | `v1.10.2` |
| pocketbase JS SDK | `^0.26.5` |
| React | `^19.2.4` |
| TanStack Router / Query | `^1.133.15` / `^5.90.12` |
| Vite / TS / Vitest | `^7.3.1` / `^5.9.3` / `^4.0.16` |

> **Note.** CI now pins the recorded `pnpm 11.8.0`, `node 22.23.2`, and
> `go1.27.1` toolchain. The `go 1.26.0` directive remains a minimum language
> version rather than the CI compiler version.

---

### 3. HTTP route surface (Phase 0 historical baseline)

Routes registered on PocketBase's chi router (`e.Router.*` inside
`internal/app`). Verified by grep across the whole package; there are no other
route registration sites.

### 3.1 Documentation / OpenAPI

| method | path | file | notes |
| --- | --- | --- | --- |
| GET | `/openapi.json` | `app.go:180` | serves the Huma document |
| GET | `/openapi.yaml` | `app.go:181` | same document as YAML |
| GET | `/docs` | `app.go:188` | Redoc HTML from the CDN; always on |

### 3.2 Auth

| method | path | file | auth | notes |
| --- | --- | --- | --- | --- |
| GET | `/api/auth/registration-policy` | `app.go:203` | none | `{"mode":"disabled\|approval\|otp"}` |
| POST | `/api/auth/guest` | `app.go:209` | none | **only registered when `SLOPSTACK_DEVELOPMENT=true`**. Returns `{token, record{id,collectionId,collectionName,email,verified,display_name}}`. Idempotently creates/repairs the single guest user `guest@taskboard.local` |
| GET | `/api/auth/app-passwords` | `app_passwords.go:78` | Bearer | `[{id,name,created}]`, newest first (`-created`) |
| POST | `/api/auth/app-passwords` | `app_passwords.go:85` | Bearer | body `{name}`; **201** with `{id,name,secret}` — `secret` is returned exactly once |
| DELETE | `/api/auth/app-passwords/{id}` | `app_passwords.go:117` | Bearer | 204; 404 if the token is not owned by the caller |

### 3.3 Calendar

| method | path | file | notes |
| --- | --- | --- | --- |
| POST | `/api/calendars/import` | `calendar_io.go:28` | multipart; requires auth |
| GET | `/api/calendars/{id}/export.ics` | `calendar_io.go:29` | requires auth; `text/calendar` |
| GET | `/api/calendar/occurrences` | `occurrences.go:53` | recurrence expansion window |
| POST | `/api/calendar/occurrences/{masterId}/override` | `occurrences.go:54` | create/modify a single-instance override |
| POST | `/api/calendar/occurrences/{masterId}/cancel` | `occurrences.go:55` | add an `EXDATE` for one instance |

### 3.4 Notifications & reminders

| method | path | file | notes |
| --- | --- | --- | --- |
| GET | `/api/notifications` | `notifications.go:120` | list |
| PATCH | `/api/notifications/{id}/read` | `notifications.go:121` | idempotent |
| POST | `/api/notifications/{id}/snooze` | `notifications.go:122` | body `{until}` (RFC3339, must be future) |
| POST | `/api/notifications/{id}/dismiss` | `notifications.go:123` | sets `dismissed_at` |
| POST | `/api/notifications/read-all` | `notifications.go:124` | |
| POST | `/api/reminders` | `notifications.go:125` | body `{targetKind,targetId,at,offsetMinutes,channel}`; **409** `{error:"reminder already exists"}` on duplicate |

### 3.5 Search

| method | path | file | notes |
| --- | --- | --- | --- |
| GET | `/api/search` | `search.go:76` | deep search across 5 collections; params `query,type,from,to,container,completed,status,page,perPage` |

> The comment above `registerSearchRoutes` claims the route is "intentionally not
> wired into `registerRoutes` yet". That is **stale** — `app.go:198` calls it.
> The route is live.

### 3.6 DAV (all registered as `e.Router.Any`, i.e. every method)

| method | path | file |
| --- | --- | --- |
| ANY | `/.well-known/caldav` | `caldav.go:50` → 308 → `/caldav/` |
| ANY | `/caldav` | `caldav.go:51` |
| ANY | `/caldav/{path...}` | `caldav.go:52` |
| ANY | `/.well-known/carddav` | `carddav.go:51` → 301 XML redirect → `/carddav/` (**auth is required first**) |
| ANY | `/carddav` | `carddav.go:52` |
| ANY | `/carddav/{path...}` | `carddav.go:53` |

CalDAV advertises `DAV: 1, 3, calendar-access`, realm `Taskboard CalDAV`, and
`Allow: OPTIONS, PROPFIND, REPORT, GET, HEAD, PUT, DELETE`.
CardDAV advertises `DAV: 1, addressbook`, realm `Taskboard CardDAV`, same `Allow`.
CalDAV body limit 10 MiB; query result limit 500.

#### 3.7 Todos (Phase 0 historical behavior)

| method | path | file | notes |
| --- | --- | --- | --- |
| GET | `/api/todos` | Phase 0 `app.go:242` | At this historical snapshot it required auth and returned a hard-coded empty array; `listTodos` was not connected to a live Huma adapter. |

At Phase 0, `openapi/openapi.json` contained only a documentation stub. Current
source registers the five live operations listed in the runtime snapshot above;
the Huma adapter serves those same handlers through the PocketBase route bridge.
CI regenerates the OpenAPI document and typed client from those operations.

---

### 4. Collections and access rules (Phase 0 historical baseline)

Read from the live database `apps/server/pb_data/data.db` (`_collections` table).
`ensureCol` = the rule is also declared in `internal/app/schema.go`.

### 4.1 App collections

| collection | id | type | list / view / create / update / delete | declared in schema.go |
| --- | --- | --- | --- | --- |
| `users` | `_pb_users_auth_` | auth | `id = @request.auth.id` / `id = @request.auth.id` / **(empty = superuser)** / `id = @request.auth.id` / `id = @request.auth.id` | yes (fields only, no rule) |
| `todos` | `pbc_113564862` | base | `@request.auth.id = owner` ×5 | yes |
| `events` | `pbc_1687431684` | base | `@request.auth.id = owner` ×5 | yes |
| `lists` | `pbc_3277857102` | base | `@request.auth.id = owner` ×5 | yes |
| `calendars` | `pbc_3774112919` | base | `@request.auth.id = owner` ×5 | yes |
| `address_books` | `pbc_2492096707` | base | `@request.auth.id = owner` ×5 | yes |
| `contacts` | `pbc_1930317162` | base | `@request.auth.id = owner` ×5 | yes |
| `notifications` | `pbc_2301922722` | base | `@request.auth.id = owner` ×5 | no — `notifications.go` |
| `reminders` | `pbc_163143500` | base | `@request.auth.id = owner` ×5 | no — `notifications.go` |
| `caldav_changes` | `pbc_2483696242` | base | `@request.auth.id = owner` ×5 | no — `caldav_changes.go` |
| `carddav_changes` | **absent from live DB** | base | `@request.auth.id = owner` ×5 (will be set by `EnsureCardDAVChanges` on next boot) | no — `carddav_changes.go` |
| `app_passwords` | `pbc_2899921234` | base | **all five empty** ⇒ superuser/SDK-only through the REST API | yes (fields only) |
| `groups` | `pbc_3346940990` | base | all five empty | yes (fields only) |
| `permissions` | `pbc_3709660955` | base | all five empty | yes (fields only) |
| `group_permissions` | `pbc_2509884351` | base | all five empty | yes (fields only) |
| `user_permissions` | `pbc_2593766271` | base | all five empty | yes (fields only) |
| `group_members` | `pbc_714390402` | base | all five empty | yes (fields only) |
| `audit_logs` | `pbc_681515208` | base | all five empty | yes (fields only) |

`carddav_changes` is declared in `carddav_changes.go` and created on first boot
with fields `id, owner, address_book, href, etag, sequence, deleted, changed_at`
and indexes `idx_carddav_changes_owner_sequence` (`owner, sequence`) and
`idx_carddav_changes_owner_book_href` (`owner, address_book, href`). It is not in
the live database because that database predates the bootstrap run that would
create it.

**Recorded migration state of the live DB:** applied migrations stop at
`20260915000000_add_todo_calendar.go`. The three newest migrations —
`20260920000000_add_dav_resource_data.go`, `20260920000001_add_carddav_change_journal.go`
— have **not** been applied to `apps/server/pb_data`. That is why `dav_data` is
absent from `todos`/`events` and `carddav_changes` is missing. The other two
boot-time repair passes have already run, so `notifications`, `reminders`, and
`caldav_changes` exist.

**Declared-but-absent drift:** `schema.go` declares `todos.dav_data` and
`events.dav_data`, which are **not** in the live database. Any shared-package
migration must not assume `dav_data` exists.

### 4.2 Field names (live)

```
address_books   : id, owner, name, description, color
app_passwords   : id, user, name, secret_hash
audit_logs      : id, actor, action, metadata
caldav_changes  : id, owner, calendar, href, resource_kind, resource_id, etag,
                  sequence, deleted, changed_at
calendars       : id, owner, name, description, color, timezone, archived, is_default
carddav_changes : id, owner, address_book, href, etag, sequence, deleted, changed_at
contacts        : id, owner, address_book, uid, formatted_name, given_name,
                  family_name, organization, title, email, phone, address, notes,
                  vcard, dav_href
events          : id, owner, title, description, start_date, end_date, all_day,
                  uid, dtstamp, duration, status, location, categories, rrule,
                  exdate, organizer, attendees, url, class, geo, alarm, start_local,
                  end_local, calendar, timezone, time_mode, recurrence_parent,
                  recurrence_id, recurrence_cancelled, dav_href
group_members   : id, group, user
group_permissions: id, group, permission
groups          : id, name, description
lists           : id, owner, name, description, color, archived, is_default
notifications   : id, owner, title, body, target_kind, target_id, reminder_key,
                  read, read_at, snoozed_until, dismissed_at
permissions     : id, name, category
reminders       : id, owner, key, target_kind, target_id, at, offset, due_at,
                  channel, status, attempts, next_attempt_at, last_error, delivered_at
todos          : id, owner, title, completed, due_date, description, uid, dtstamp,
                  start_date, duration, status, completed_at, percent_complete,
                  priority, categories, rrule, exdate, organizer, attendees, url,
                  class, geo, alarm, start_local, list, calendar, timezone,
                  time_mode, due_local, dav_href
user_permissions: id, user, permission, denied
users           : id, password, tokenKey, email, emailVisibility, verified, name,
                  avatar, created, updated, isAdmin, activation_code,
                  registration_status, display_name, disabled
```

---

### 5. Auth mechanisms (Phase 0 historical baseline)

### 5.1 `auth.go` — thin helpers

`RegistrationAllowed` / `RegistrationRequiresApproval` / `RegistrationRequiresOTP`
read `Config.RegistrationMode`. `CurrentUser(event)` returns
`event.UnauthorizedError("authentication required", nil)` when `event.Auth` is
nil, otherwise `event.Auth`. PocketBase owns sessions, verification, and password
reset; there is no Taskboard login route.

### 5.2 Registration policy — `app.go:152 protectUserAdminFlag`

`OnRecordCreateRequest("users")`:
- superuser ⇒ pass through unchanged
- `RegistrationDisabled` ⇒ 403 `"registration is disabled"`
- otherwise force `isAdmin=false`, `disabled=true`, and
  - OTP mode: set `activation_code` (6 random decimal digits) and
    `registration_status="awaiting_activation"`, and **log the code to stdout**
    (`registration activation code email=… code=…`) — there is no mailer
  - approval mode: `registration_status="awaiting_approval"`

`OnRecordUpdateRequest("users")`: a non-superuser can never change `isAdmin`; it
is reset to the original value.

### 5.3 Guest auth — `guest_auth.go`

`POST /api/auth/guest` (development only) ensures a single user
`guest@taskboard.local` exists and returns a PocketBase auth token. If the guest
record exists but is `disabled`, `isAdmin`, or unverified, it is repaired to
`disabled=false`, `isAdmin=false`, `registration_status="active"`,
`verified=true`. The password is a random 24-byte base64url app password.

### 5.4 App passwords — `app_passwords.go`, `app_password_throttle.go`

CalDAV/CardDAV use HTTP Basic where the username is the user's **email address**
and the password is an app-specific secret.

`AuthenticateAppPassword(app, username, password, clientIP...)`:
1. rate-limit keys: `username` (lowercased/trimmed), `ip` (if supplied), `user`
   (record id). Limiter: 15-minute window, 3 failures, backoff 1s → 60s, max 4096
   tracked entries (`app_password_throttle.go`).
2. find `users` by exact `email`; no record ⇒ record a failure
3. reject `disabled` users
4. compare against every `app_passwords.secret_hash` (bcrypt, `DefaultCost`); on
   match reset the limiter and return the user
5. otherwise record a failure and return `errInvalidAppPassword`

Any failure returns the same opaque error; the caller decides whether to answer
401. Secrets are 24 random bytes, base64url raw; only the bcrypt hash is stored.
`secret_hash` is declared `hidden` in `schema.go`.

`AuthenticateAppPassword` is exported and takes an **optional** variadic
`clientIP`, so existing single-argument callers must keep compiling.

### 5.5 `authorization.go`

`Authorize(event, permission)` currently:
- `CurrentUser` → 401 if unauthenticated
- `HasSuperuserAuth()` ⇒ allow
- otherwise `event.ForbiddenError("missing permission: "+permission)`

No group or permission-table lookup is implemented. The `groups`, `permissions`,
`group_members`, `group_permissions`, `user_permissions` collections exist but are
never read by any handler — the permission model is scaffolding.

### 5.6 Ownership enforcement — `ownership.go`

`registerOwnershipHooks` binds `OnRecordCreateRequest` / `UpdateRequest` /
`DeleteRequest` for `todos`, `events`, `lists`, `calendars`, `contacts`.
Rules enforced:
- non-superuser without auth ⇒ 401
- on create, `owner` is **overwritten** with `e.Auth.Id`
- on update/delete, `Original().GetString("owner") != e.Auth.Id` ⇒ 403
  `"record is not owned by the current user"`
- container must be set and owned: `todos` → `list`, `events` → `calendar`
  (`containerField`), else 400 `"<field> is required"` / `"container not found"` /
  `"container is archived"`
- deleting a non-empty `lists`/`calendars` container ⇒ 400
  `"non-empty containers must be archived"` (query limited to 1 record)
- deleting an `events` master (`recurrence_parent == ""`) also deletes all of its
  `events` overrides
- `contacts.address_book` must be set and owned: 400 `"address_book is required"` /
  `"address book not found"`
- PocketBase API rules still gate reads

### 5.7 Default provisioning — `app.go:provisionDefaultContainers`

On `users` create-success, for the new owner:
`Tasks` (`#87c4a8`, list), `Events and Holidays` (`#3b6ea8`, calendar),
`Contacts` (`#7657a8`, address book), each with `is_default=true`. Idempotent
(existence checked by `owner`+`name`).

---

### 6. Backup subsystem and CLI (Phase 0 historical baseline)

### 6.1 Files

| file | role |
| --- | --- |
| `backup_encryption.go` | key management + AES-GCM: `createBackupKey`, `importBackupKey`, `loadBackupKey`, `encryptBackup`, `decryptBackup`. Key file `.backup-encryption-key` in the data dir, mode `0600`, dir chmod `0700`. 32 random bytes, base64url raw |
| `backup_formats.go` | `createEncryptedArchive` (`manual_*`/`scheduled_*` → `.zip.enc`), `restoreEncryptedArchive`, `createEncryptedJSON` (→ `.json.enc`), `cleanupTransientBackupArchives` (called on every bootstrap) |
| `backup_scheduler.go` | `registerEncryptedBackupScheduler` — cron job named `encrypted-backups` using `SLOPSTACK_BACKUP_CRON`; emits archive, optional JSON, then `pruneEncryptedBackups` |
| `backup_command.go` | the cobra command tree |

### 6.2 CLI surface

PocketBase's own root commands (`serve`, `migrate`, `superuser`,
`migratecollections`, …) plus, from Taskboard:

```
slopstack openapi export                        # writes openapi/openapi.json
slopstack backup create                         # prints the new .zip.enc name
slopstack backup json                           # prints the new .json.enc name
slopstack backup key-init                       # creates the key, prints it once
slopstack backup key-import                     # reads the key from stdin (<=1024 bytes)
slopstack backup list                           # NAME/SIZE/MODIFIED, newest first
slopstack backup restore <exact-name> --confirm  # destructive, restarts the process
```

Backup filename safety: `validateBackupName` requires
`^@?[a-z0-9_-]+\.zip(\.enc)?$` and length ≤ 150; anything else errors with
`unsafe backup name %q: use an exact archive name from 'backup list'`.
`restore` requires `--confirm` and is guarded by `SLOPSTACK_BACKUP_RESTORE_RESTARTED=1`
so the re-exec happens exactly once.

### 6.3 PocketBase settings coupling

`applyBackupConfig` (`app.go:62`) runs at bootstrap:
- `settings.Meta.AppURL` ← `SLOPSTACK_PUBLIC_URL`
- always sets `settings.Backups.Cron = ""` (**disables** PocketBase's own cron
  backups — Taskboard schedules its own encrypted ones)
- when `SLOPSTACK_BACKUP_ENABLED=false`: clears cron and disables S3, returns
- otherwise sets `CronMaxKeep = SLOPSTACK_BACKUP_MAX_KEEP`, fills the S3 config,
  and calls `settings.Backups.Validate()`
- `validateBackupSchedule` rejects `maxKeep < 1` and any un-parseable cron
  expression, both with a message prefixed `invalid backup configuration:`

Retention prunes `.zip.enc` and `.json.enc` independently, keeping the `keep`
newest of each.

---

### 7. Compatibility rule set (Phase 0 historical baseline)

These are the invariants that **must not change** while functionality moves into
shared Slopstack packages. Each is a hard requirement, not a preference.

### C1 — Collection names and IDs
Every collection name in §4.1 and every PocketBase collection id (e.g.
`pbc_113564862` for `todos`) is stored inside existing records, backup archives,
`dav_href` values, and client-side code. Renaming or recreating a collection
orphans data. Never drop-and-recreate a collection to reshape it.

### C2 — Stored field data
Field names, types, and nullability in §4.2 are read by the web app, by
`search.go`'s `queryFields`/`containerField`/`dateField` table, by
`calendar_io.go`'s ICS mapping, by the DAV property mappers, and by existing
records. Adding a field is safe; renaming, retyping, or dropping one is not.
Migration must be additive.

### C3 — Existing user credentials and auth
`users` records, their PocketBase password hashes and `tokenKey`s must remain
valid. Registration, verification, approval, OTP, and password reset all
continue to be handled by PocketBase. `disabled` / `isAdmin` /
`registration_status` / `activation_code` semantics (including the
"non-superuser can never set `isAdmin`" invariant) must be preserved exactly.

### C4 — App-password records and Basic auth
`app_passwords.secret_hash` bcrypt hashes must keep verifying. CalDAV/CardDAV
Basic auth must keep accepting *email as username*. The rate-limiter window
(15 min / 3 failures / 1s→60s backoff) and the opaque error are behaviour, not
implementation detail. `AuthenticateAppPassword`'s variadic `clientIP` signature
is part of the exported contract.

### C5 — `pb_data` locations and on-disk layout
`SLOPSTACK_STORAGE_PATH` (default `pb_data`) must keep resolving the same way,
`data.db`/`auxiliary.db`/WAL files must stay where PocketBase expects them,
`.backup-encryption-key` must stay inside the data dir with mode `0600`, and the
backups filesystem must stay at `<dataDir>/backups`. Backup archives must stay
restorable by `backup restore` with the existing name pattern.

The stray repo-root `pb_data/` (see `docs/ARCHITECTURE.md` §6) is a second,
independent database. It must not be deleted as part of the migration without a
decision, and nothing should be moved out of or into it automatically.

### C6 — `/api/*` request/response shapes
Paths, methods, status codes, JSON keys (including camelCase in notifications
and occurrences responses), and the multipart field names used by
`POST /api/calendars/import` are consumed by clients. `GET /api/todos` must keep
returning `[]` (200). The generated `openapi/openapi.json` is diffed by CI, so
adding an operation changes a checked artifact — do that deliberately.

### C7 — DAV (CalDAV/CardDAV) behaviour
Realm strings `Taskboard CalDAV` / `Taskboard CardDAV`, the `DAV` and
`MS-Author-Via` header values, the `Allow` header, `/.well-known/*` redirect
status codes (308 for CalDAV, 301 for CardDAV, which requires auth first),
calendar and address-book href shapes, ETag computation, `CTag`, the
`sync-collection` journals in `caldav_changes` / `carddav_changes`, and the
10 MiB / 500 result limits are all client-visible.

### C8 — ICS import/export
`POST /api/calendars/import` and `GET /api/calendars/{id}/export.ics` must keep
round-tripping: `VEVENT` ⇄ `events`, `VTODO` ⇄ `todos`, UID matching for
updates, `rrule`/`exdate`/`attendees`/`organizer`/`alarm`/`class`/`geo`
preservation, and the `dav_data` raw-payload preservation path.

### C9 — Search behaviour
`GET /api/search` must keep returning the same envelope
`{page, perPage, totalItems, totalPages, items[]}` with the same `type` values
(`todo`, `event`, `list`, `calendar`, `contact`), the same default `perPage=20`
and `perPage` cap of 100, the same field precedence (e.g. a contact's title is
`formatted_name`, its description is `notes`), and the same ownership scoping.

### C10 — Notification and reminder semantics
`GET /api/notifications` filters out dismissed records and snoozed-until-future
records, ordered `-created`. `snooze` clears `read`/`read_at` and requires a
future RFC3339 `until`. `dismiss` sets `dismissed_at` and is irreversible via the
API. `POST /api/reminders` must keep returning **409** on a duplicate key, must
only accept the `in_app` channel (others are 400), and the reminder key uniqueness
indexes (`idx_reminders_key` unique, `idx_notifications_reminder_key` unique) must
survive.

### C11 — Ownership and container invariants
The rules in §5.6 are enforced twice (PocketBase rules for reads, hooks for
writes). Moving enforcement into a shared package must not weaken either half,
and the 400/403 message strings are relied on by the client.

### C12 — Bootstrap ordering
`OnBootstrap` must keep the order: cleanup transient backups → run migrations →
`EnsureCollections` → `EnsureNotificationCollections` → `EnsureCalDAVChanges` →
`EnsureCardDAVChanges` → `applyBackupConfig`. Several of these repair older
databases and are not equivalent to a fresh migration run.

## Synthetic legacy-schema regression

`go test ./apps/server -run TestHistoricalTaskboardBackupUpgradePreservesRecords -count=1`
uses `tests.NewTestApp()` to create a disposable database, runs the registered
app migrations only through the `20260915000000_add_todo_calendar.go` cutoff,
seeds a user, todo, event, and contact, then archives and extracts it into a
separate temporary data directory. It applies the pending registered migrations
and current bootstrap schema repairs twice,
asserting that collection IDs, record IDs, and representative values survive
and that newer fields and the CardDAV journal are added. Because some older
migration callbacks call the live `EnsureCollections` repair function, the
fixture removes schema introduced after the selected cutoff before archiving.
This is reproducible migration-history coverage, not a database produced by an
older released binary; Phase 7 still requires validating disposable copies of
populated real-schema databases from older releases, including restore.
