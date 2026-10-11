# Taskboard — Agent Notes (open technical debt)

Phase 1 identity update: the current module path is
`github.com/andreasmolnardev/taskboard`, the web package is `@taskboard/web`,
and the CLI is `taskboard`. This file's Phase 0 identity inventory and ordered
migration suggestions below describe the earlier baseline. Runtime reads
`TASKBOARD_*` first and supports `SLOPSTACK_*` as a legacy fallback. The Compose
volume name and PocketBase data path stay unchanged.

Phase 0 companion to `docs/ARCHITECTURE.md`, `docs/COMPATIBILITY.md`,
`docs/EXTRACTION-INVENTORY.md` and `docs/baseline.json`.

Those four describe *what is*. This file describes **what is wrong**, and it
exists so that later phases start from measured facts instead of guesses. Every
claim below was checked against the source tree at the Phase 0 baseline. Where
this file disagrees with an existing `docs/` file the disagreement is recorded
explicitly in §7 with the command that settles it — it is not silently
overwritten.

Read this before touching anything in Phase 1.

---

## 0. How to re-verify this file

The four measurement commands this document leans on most:

```sh
# 1. counts in the three server packages
cd apps/server
for p in app calendar reminders; do
  echo "internal/$p: total=$(ls internal/$p/*.go|wc -l) \
nontest=$(ls internal/$p/*.go|grep -vc _test) \
loc=$(cat internal/$p/*.go|wc -l)"
done

# 2. which rules the Ensure* repair passes overwrite (see §3)
grep -n 'ListRule == nil' internal/app/schema.go
grep -n 'collection.ListRule = &ownerRule' internal/app/*.go

# 3. which data dir the process resolves
grep -n 'StoragePath' internal/app/config.go
grep -n 'DefaultDataDir' cmd/slopstack/main.go

# 4. live schema, read-only
python3 -c "import sqlite3;c=sqlite3.connect('file:apps/server/pb_data/data.db?mode=ro',uri=True);print(list(c.execute('select name from _collections where system=0')))"
```

Live databases were read with `mode=ro`. **Never boot the server during the
audit** — `OnBootstrap` writes to `_collections` on every start (§3).

---

## 1. Module identity is still the template's

This is the pre-migration identity audit. Taskboard now uses its own Go module,
binary, package, and environment identities. The historical baseline and file
locations below record what the audit found before those changes.

| surface | file | current value |
| --- | --- | --- |
| Go module path | `go.mod:1` | former value: `module github.com/slopstack/slopstack` |
| Go binary | `apps/server/cmd/slopstack/` | directory and binary named `slopstack` |
| CLI root command | `apps/server/cmd/slopstack/main.go:21` | `pb.RootCmd.Use = "slopstack"` |
| web package | `apps/web/package.json` | `"name": "@slopstack/web"` |
| root package | `package.json:2` | `"name": "slopstack"` |
| env prefix | `apps/server/internal/app/config.go:45-63` | `SLOPSTACK_*` |
| docker service + volume | `docker-compose.yml` | service `slopstack`, volume `slopstack-data` |
| docker image tag | `Makefile` `docker:` target | `docker build -t slopstack .` |
| air build output | `apps/server/.air.toml` | `go build -o ./tmp/slopstack` |
| migration prose | `apps/server/pb_migrations/README.md` | "Slopstack creates its initial core collections…" |
| env var for restore re-exec | `internal/app/backup_command.go` | `SLOPSTACK_BACKUP_RESTORE_RESTARTED` |

Full list of files containing the literal `SLOPSTACK_` (11):

```
README.md
apps/server/.air.toml
apps/server/internal/app/backup_command.go
apps/server/internal/app/config.go
docker-compose.yml
docs/ARCHITECTURE.md
docs/AUTH.md
docs/COMPATIBILITY.md
docs/DEPLOYMENT.md
docs/EXTRACTION-INVENTORY.md
docs/baseline.json
```

The last six are documentation — renaming code will make them drift unless they
are updated in the same commit.

### 1.1 CRITICAL CONSTRAINT for the rename

**Renaming the Go module must not, under any circumstances:**

1. **rename or recreate the PocketBase database.** The data directory is chosen
   by `SLOPSTACK_STORAGE_PATH` (default the *relative* `pb_data`, `config.go:51`)
   and passed to PocketBase as `DefaultDataDir` (`main.go:20`). It is a
   filesystem path, completely independent of the Go module path.
2. **change any collection ID.** IDs are PocketBase-generated (`pbc_113564862`
   for `todos`, `pbc_1687431684` for `events`, …) and are stored *inside records*
   — `todos.list`, `todos.calendar`, `events.calendar`, `contacts.address_book`,
   `caldav_changes.calendar`, `caldav_changes.address_book`,
   `group_permissions.group`, `group_members.group`, `user_permissions.user`.
   Breaking one orphans rows silently. See `docs/COMPATIBILITY.md` §C1.
   Verified helpful fact: `pb_migrations/*.go` resolve collections with
   `FindCollectionByNameOrId("<name>")` and **do not hardcode `pbc_*` ids**
   (4 × `users`, 1 × `todos`, 1 × `contacts`, 1 × `app_passwords`), so new
   migrations can keep resolving by name.
3. **move `pb_data`.** See §5 — there are already *two* of them, which is the
   problem this constraint exists to prevent growing. `docs/COMPATIBILITY.md` §C5.
4. **invalidate existing user credentials.** `users` rows carry PocketBase
   password hashes and `tokenKey`. Nothing in the rename touches the `users`
   collection or `auth.go`/`guest_auth.go`/`app_passwords.go` semantics, so a
   pure import-path change is safe. The risk is *collateral*: dropping and
   recreating a collection, or moving the data dir, silently re-mints auth.
   See `docs/COMPATIBILITY.md` §C3.

Practical reading: the module rename is a **textual** operation over
`go.mod`, `*.go` import lines, and a handful of build files. It must not
include any migration, any `Ensure*` change, or any data-directory change. If a
rename commit touches `pb_migrations/`, stop.

### 1.2 The `SLOPSTACK_` → `TASKBOARD_` rename is a separate, breaking change

`SLOPSTACK_STORAGE_PATH` is read by deployed instances (Docker sets it to
`/app/pb_data`; `docs/DEPLOYMENT.md` documents it). Renaming it silently makes a
deployed server fall back to the default `pb_data` relative path — which under a
different working directory is a **different, empty database** (§5). If the
rename happens, `config.go` must accept the old name as a deprecated fallback and
log a warning, and `docs/DEPLOYMENT.md` must document the migration. Do this
after, not together with, §1.1's module rename.

---

## 2. `internal/app` is a single-package monolith

### 2.1 Size (measured)

| package | `.go` files | non-test files | LOC (all) | LOC (non-test) |
| --- | --- | --- | --- | --- |
| `apps/server/internal/app` | **39** | 24 | 8 808 | **6 204** |
| `apps/server/internal/calendar` | 2 | 1 | 324 | 187 |
| `apps/server/internal/reminders` | 4 | 3 | 502 | 314 |
| **total** | **45** | **28** | **9 634** | **6 705** |

Largest files: `caldav.go` 977, `carddav.go` 876, `calendar_io.go` 596,
`notifications.go` 506, `occurrences.go` 480, `vcard.go` 395, `search.go` 292,
`app.go` 249, `backup_command.go` 230, `backup_formats.go` 183,
`ownership.go` 179.

One concern per file, no seams: config, auth, authz, ownership, CalDAV, CardDAV,
vCard, ICS, recurrence, search, notifications/reminders, backups, OpenAPI
plumbing, and the `users` lifecycle all sit in `package app`. There is no
`internal/auth`, `internal/dav`, `internal/backup`, `internal/ical`.

### 2.2 Why this blocks Phase 3

`docs/EXTRACTION-INVENTORY.md` classifies 17 files CORE, 7 OPTIONAL-SHARED and
20 APP-OWNED. The classification cannot be *executed* while the package is
monolithic, for three concrete reasons:

1. **Every cross-file dependency is intra-package and implicit.** `caldav.go`
   calls `AuthenticateAppPassword`, `authorizeDAVRequest`, `CurrentUser` and
   helpers from `app_passwords.go`/`auth.go`/`ownership.go` without any
   interface. Moving CalDAV out of the package requires those helpers to have
   moved first, in the right order.
2. **Every symbol is exported.** Nothing is package-private, so the compiler
   cannot tell you which of the 24 files actually *need* which others. The
   dependency set has to be re-derived by hand during extraction.
3. **`app.go` is the composition root and imports everything.** Splitting it
   (`EXTRACTION-INVENTORY.md` §4) is what creates the first real seam.

**Therefore: split `internal/app` into `internal/auth`, `internal/backup`,
`internal/dav`, `internal/ical`, `internal/notify` (or an equivalent set) before
Phase 3 extraction begins.** A pure file-move split inside the repo — no
behaviour change, no package moves — is enough, and is safe to do with the
existing tests green. That split is the deliverable; the actual upstream
extraction is Phase 3.

---

## 3. Schema is declared twice, and three of the repair passes are destructive

### 3.1 Migration count

**17** `.go` migration files exist in `apps/server/pb_migrations/`
(`README.md` is also in that directory and is not a migration):

```
20260824000000_add_user_is_admin.go
20260908000000_add_registration_fields.go
20260908000001_add_app_passwords.go
20260908000002_add_taskboard_collections.go
20260908000003_add_icalendar_fields.go
20260908000004_add_carddav_collections.go
20260912000000_add_entry_containers.go
20260912000001_add_notifications.go
20260913000000_add_temporal_and_container_state.go
20260913000001_add_notification_actions.go
20260913000002_add_recurrence_overrides.go
20260913000003_add_caldav_hrefs.go
20260913000004_add_caldav_change_journal.go
20260914000000_add_carddav_resource_hrefs.go
20260915000000_add_todo_calendar.go
20260920000000_add_dav_resource_data.go
20260920000001_add_carddav_change_journal.go
```

Of these, **15 are applied** to both live databases; the last two
(`20260920000000`, `20260920000001`) are **pending**. `_migrations` holds 23 rows
in total — the 15 Taskboard migrations plus 8 PocketBase internal ones.

(`docs/ARCHITECTURE.md` §4 and `docs/baseline.json` `schemaDrift` both say 18 on
disk / 16 applied. See §7.)

### 3.2 The `Ensure*` functions run on EVERY boot

`internal/app/app.go:20-45`, inside `OnBootstrap`, **after**
`e.App.RunAppMigrations()` at line 27:

```
cleanupTransientBackupArchives → RunAppMigrations → EnsureCollections
  → EnsureNotificationCollections → EnsureCalDAVChanges → EnsureCardDAVChanges
  → applyBackupConfig
```

They are not gated on "is there a pending migration". Every `serve` runs all
seven. `docs/COMPATIBILITY.md` §C12 freezes this order — the order is
load-bearing, the *unconditionalness* is the debt.

### 3.3 Do they overwrite admin-UI rule changes? **Yes, for four collections.**

This was determined by reading `schema.go` and the three `Ensure*` functions,
not assumed. The behaviour splits three ways:

| pass | collections | rule behaviour on every boot |
| --- | --- | --- |
| `EnsureCollections` (`schema.go:30-64`) | `users`, `app_passwords`, `groups`, `permissions`, `group_permissions`, `user_permissions`, `group_members`, `todos`, `events`, `lists`, `calendars`, `address_books`, `contacts`, `audit_logs` | rule applied **only if `collection.ListRule == nil`** (`schema.go:34`). An existing non-nil ListRule is left alone. |
| `EnsureNotificationCollections` (`notifications.go`) | `notifications`, `reminders` | **all five rules overwritten unconditionally** |
| `EnsureCalDAVChanges` (`caldav_changes.go`) | `caldav_changes` | **all five rules overwritten unconditionally** |
| `EnsureCardDAVChanges` (`carddav_changes.go`) | `carddav_changes` | **all five rules overwritten unconditionally** |

The last three assign `collection.ListRule, ViewRule, CreateRule, UpdateRule,
DeleteRule = &ownerRule, …` with no guard, and call `app.Save(collection)`
unconditionally. So **an administrator who relaxes, locks, or customises the
API rules for `notifications`, `reminders`, `caldav_changes` or `carddav_changes`
in the PocketBase admin UI has the change silently reverted on the next server
restart**, with no log line. This is a live footgun: the admin UI invites people
to edit rules, and four collections silently reject the edit.

Two sub-cases of the `EnsureCollections` guard are worth writing down because
they surprise people in opposite directions:

- Editing **only** `View`/`Update`/`Delete` while `ListRule` stays non-nil →
  the edit **survives**. That is correct but accidental.
- Setting `ListRule` to empty (the PocketBase idiom for "lock this collection")
  → `ListRule == nil` becomes true → on the next boot **all five rules are reset
  to `@request.auth.id = owner`**, re-opening view/update/delete. An admin
  *intending* to lock a collection gets it made *more* permissive.

`EnsureCollections` is otherwise additive-only: `addMissingFields` adds absent
fields and never removes, retypes, reorders or re-tightens existing ones. Field
*constraints* on pre-existing fields are never touched — a field declared
`required: true` in `schema.go` that exists un-required in the database is left
alone. That is drift in the other direction, and it is deliberate.

### 3.4 Redundant work at every boot

`EnsureCollections` itself calls `EnsureCalDAVChanges` and `EnsureCardDAVChanges`
at `schema.go:65-70`, and `app.go:38-42` calls both **again**. Each of those
passes ends in an unconditional `app.Save`. Result: `caldav_changes` and
`carddav_changes` are saved twice per boot, and `notifications`/`reminders` are
saved once per boot, purely to re-assert rules that were already asserted.

Fix (small, safe, and it makes the drift auditable): drop the duplicate calls
from `schema.go:65-70`, and add an explicit "rules differ from declared ⇒ log a
warning before overwriting" line to the three destructive passes. That converts
a silent revert into a visible event.

### 3.5 Declared-but-absent drift

`schema.go` declares `todos.dav_data` and `events.dav_data`, which migration
`20260920000000` adds and which are **not** in either live database because that
migration is pending. `carddav_changes` is likewise missing. Anything extracted
upstream must not assume `dav_data` or `carddav_changes` exist — see
`docs/COMPATIBILITY.md` §4.1.

---

## 4. Inherited-but-unused files

"Used" below means **reachable in the production module graph starting at
`apps/web/src/main.tsx`**, with the `@/` → `src/` alias resolved
(`tsconfig.json` `paths`, mirrored in `vite.config.ts:8`). Test files are *not*
treated as consumers — a module imported only from a `*.test.tsx` is unused
code. Reproduction of this method is in §0.

### 4.1 Web: 51 of 60 shadcn UI components are dead

`apps/web/src/components/ui/` contains **60** components. **9** are reachable;
**51** are not.

| reachable component | imported by |
| --- | --- |
| `button` | `container-manager.tsx`, `ui/dialog.tsx` |
| `input`, `label`, `switch`, `textarea` | `container-manager.tsx` |
| `command` | `tabs/search-tab.tsx` |
| `dialog` | `ui/command.tsx` (transitively, via `search-tab`) |
| `tabs` | `tabs/calendar-tab.tsx`, `tabs/settings-tab.tsx`, `app/create-composer.tsx` |
| `tooltip` | `main.tsx` (`TooltipProvider`) |

The 51 unreachable ones — all inherited shadcn scaffolding with no importer
anywhere in the production graph:

```
accordion, alert, alert-dialog, aspect-ratio, attachment, avatar, badge,
breadcrumb, bubble, button-group, calendar, card, carousel, chart, checkbox,
collapsible, combobox, context-menu, direction, drawer, dropdown-menu, empty,
field, hover-card, input-group, input-otp, item, kbd, marker, menubar, message,
message-scroller, native-select, navigation-menu, pagination, popover, progress,
radio-group, resizable, scroll-area, select, separator, sheet, sidebar, skeleton,
slider, sonner, spinner, table, toggle, toggle-group
```

> **Correction to the existing docs.** `docs/baseline.json`
> (`inheritedButUnused.webUiComponentsUnused`) and `docs/ARCHITECTURE.md` §3.2
> both list **`dialog` as unused**. It is used — `ui/command.tsx` imports it, and
> `command` is used by `search-tab.tsx`. The correct counts are **9 used / 51
> unused**, not 8 / 52. See §7.

Note the trap this illustrates: a plain grep for `from "@/components/ui/…"`
returns hits for `separator`, `input-group`, `skeleton`, `sheet` and `toggle`,
but **every one of those hits comes from inside another dead file**
(`sheet.tsx`, `dialog.tsx`, …). Grep cannot answer the question; only graph
reachability can.

Also dead: **`apps/web/src/hooks/use-mobile.ts`**. Its only importer is
`components/ui/sidebar.tsx`, which is itself one of the 51 dead components.
Taskboard's navigation is the hand-written `components/sidebar.tsx`. Transitively
dead through two hops.

### 4.2 Server

| item | evidence |
| --- | --- |
| `audit_logs` collection | declared at `schema.go:27`, **never written or read by any code**. `grep -rn 'audit_logs\|auditLog' apps/server --include=*.go` returns exactly one hit — the declaration. Dead schema. |
| `Authorize()` (`internal/app/authorization.go`) | **zero callers**: `grep -rn 'Authorize(' apps/server --include=*.go \| grep -v 'func Authorize'` is empty. Dead *and* it would deny every non-superuser if it were ever wired in. Enforcement today comes from PocketBase API rules + `registerOwnershipHooks`. |
| Todo HTTP API | **Resolved since the Phase 0 audit:** five Huma operations now provide owner-scoped list/create/read/update/delete handlers through the PocketBase router bridge. See `docs/COMPATIBILITY.md` §Current runtime snapshot. |
| `groups`, `permissions`, `group_members`, `group_permissions`, `user_permissions` | collections exist with empty (superuser-only) API rules and are never read by any handler. Scaffolding. |
| `Config.SecureCookies`, `Config.LogLevel` | declared (`config.go:22,24`), populated from `SLOPSTACK_SECURE_COOKIES` / `SLOPSTACK_LOG_LEVEL` (`config.go:48,50`), and **read nowhere** in the server. Setting them does nothing. |

### 4.3 Build/CI leftovers

| item | evidence |
| --- | --- |
| `go.mod` marks **every** dependency `// indirect` | `pocketbase`, `huma/v2`, `cobra`, `go-ical`, `rrule-go`, `dbx` are all imported directly by non-test code yet all carry `// indirect`. `go mod tidy` would strip those comments. Cosmetic, but it hides the real direct/indirect split and makes the file useless for dependency review. |
| `.golangci.yml` is never run | present at repo root; `ci.yml` has no golangci step. `docs/baseline.json` records `golangciLint: false`. Dead config. |
| CI's generated-artifact drift gate is a no-op | `ci.yml` runs `pnpm generate:api`, `pnpm generate:client`, then `git diff --exit-code`. But **both outputs are gitignored** (`.gitignore:13` `openapi/openapi.json`, `.gitignore:14` `apps/web/src/api/generated/`). `git diff` does not see ignored files, so that step can never fail on generated drift. The "CI regenerates and diffs this file" claim in `docs/COMPATIBILITY.md` §3.7 is **not** what CI actually does. |

### 4.4 Verified *not* unused

Guards against over-pruning: `internal/app/app_password_throttle.go` is live
(it backs `AuthenticateAppPassword`), and the `internal/calendar` and
`internal/reminders` packages both have real consumers (`occurrences.go` and
`notifications.go` respectively). `internal/reminders/memory_store.go` is used by
`reminders_test.go` only — it is a test double today, as
`docs/EXTRACTION-INVENTORY.md` §3 already notes.

---

## 5. The duplicate `pb_data` — root one is authoritative-for-nothing

### 5.1 Which directory the server resolves

`config.go:51`:

```go
StoragePath: env("SLOPSTACK_STORAGE_PATH", "pb_data"),
```

`main.go:20`:

```go
pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: config.StoragePath, ...})
```

The default is the **relative** path `pb_data`, so it resolves against the
**process working directory**, not against the binary or the repo root. Which
`pb_data` is used is therefore decided purely by *how the binary is launched*:

| launcher | cwd | data dir |
| --- | --- | --- |
| `pnpm dev:server` → `air -c .air.toml` (`.air.toml` `root = "."`, `apps/server/.air.toml`) | `apps/server` | **`apps/server/pb_data`** |
| `make migrate` → `go run ./apps/server/cmd/slopstack migrate up` | repo root | `pb_data` (root) |
| `make build` → `go build ./apps/server/cmd/slopstack`, then `./slopstack serve` | repo root | `pb_data` (root) |
| `Dockerfile` `WORKDIR /app` + `docker-compose.yml` `SLOPSTACK_STORAGE_PATH: /app/pb_data` | `/app` | `/app/pb_data` (volume `slopstack-data`) |

`Makefile`'s `migrate` and `build` targets run **from the repo root**, which is
exactly how the root directory got created. `pnpm dev:server` `cd`s into
`apps/server` first, which is why the dev loop uses the nested one.

### 5.2 Which one holds the data — measured

Both `data.db` files were opened read-only:

| | `apps/server/pb_data/data.db` | `pb_data/data.db` (root) |
| --- | --- | --- |
| collections (incl. system) | 22 | 22 |
| collection IDs | identical | identical |
| applied migrations | identical 23-row set | identical 23-row set |
| **app records (non-system rows)** | **35** | **0** |
| `users` rows | **2** | **0** |
| `todos` / `events` / `contacts` | 5 / 5 / 4 | 0 / 0 / 0 |
| `caldav_changes` | 10 | 0 |
| `data.db` size | 299 008 B (+ 1.2 MB `-wal`) | 372 736 B |
| `auxiliary.db` | 716 800 B (+ 1.0 MB `-wal`) | 24 576 B |
| last write | 2026-09-19 23:23 (WAL) | 2026-09-19 23:25 |

**Verdict.**

- **`apps/server/pb_data` is authoritative.** It is the one `pnpm dev:server`
  uses, it is the only one with data, and it is the only one holding user
  records.
- **The root `pb_data/` is an empty, stale shell.** Its schema was bootstrapped
  (`EnsureCollections` created the collections and the repair passes wrote rules
  into `_collections`) but **not one application row was ever written to it**.
  Its larger `data.db` (372 KB vs 299 KB) is schema/bookkeeping overhead, not
  data. It is not "an older copy of the same data" — it is a data-less
  duplicate that looks authoritative because it sits at the repo root where a
  root-run binary would pick it up.

**The actual hazard is the reverse of the one assumed so far.** The risk is not
that someone reads the wrong database; it is that someone runs `make migrate` or
`./slopstack serve` from the root, bootstraps against the **empty** root
database, and believes they are operating on their real data. Any destructive
root-run command (`backup restore`) would target an empty instance.

Do not delete it as part of the migration without a decision
(`docs/COMPATIBILITY.md` §C5). The Phase 1 fix that actually removes the hazard
is to make the path explicit rather than relative — see §8 step 5.

---

## 6. Other technical debt verified in this repo

### 6.1 Audit of the four findings from the sibling Piggybudget audit

The sibling repo surfaced these; each was **re-verified here independently**.
Two reproduce, two do not. Do not carry the sibling's conclusions over.

| finding (from Piggybudget audit) | Taskboard result | evidence |
| --- | --- | --- |
| OTP / activation code logged to stdout | **REPRODUCES** | `app.go:165` — `log.Printf("registration activation code email=%s code=%s", e.Record.GetString("email"), code)` inside `OnRecordCreateRequest("users")` when `RegistrationMode == RegistrationOTP`. |
| `confirm` query param checked by key presence, not value | **DOES NOT REPRODUCE** | There is no `confirm` query parameter anywhere. `grep -rn 'URL.Query'` in `internal/app` returns only `search.go:84` and `occurrences.go:63`. The confirm gate is a proper cobra **bool flag**: `backup_command.go:208` `BoolVar(&confirmed, "confirm", false, …)` and `backup_command.go:192` `if !confirmed { … }` — value-checked. |
| unauthenticated guest token minting | **PARTIALLY REPRODUCES — different shape** | See §6.2. |
| backup cron not validated | **DOES NOT REPRODUCE** | `config.go` `validateBackupSchedule` rejects `maxKeep < 1` *and* parses the expression via `cron.New().Add(...)`, returning `invalid backup configuration: …`. It is called from `applyBackupConfig` (`app.go:76`) **before** any use, so a bad cron fails bootstrap loudly. `registerEncryptedBackupScheduler` additionally surfaces `pb.Cron().Add` errors (`backup_scheduler.go`). This is the area where Taskboard is *ahead* of the sibling. |

### 6.2 Guest auth: gated, but the default configuration enables it on all interfaces

`POST /api/auth/guest` (`app.go:209`) is registered only when
`config.Development` is true, i.e. `SLOPSTACK_DEVELOPMENT=true`. So it is not
unconditionally exposed — but the surrounding defaults make accidental exposure
likely:

- `.env.example:7` ships **`SLOPSTACK_DEVELOPMENT=true`**. Copying the example
  env file — the documented setup path — turns guest minting on.
- `apps/server/.air.toml` hardcodes `SLOPSTACK_DEVELOPMENT=true` in `full_bin`,
  and `.env.example:1` defaults `SLOPSTACK_HTTP_ADDRESS=0.0.0.0:8090`. The dev
  server therefore listens on **all interfaces** with guest minting enabled.
- `ensureGuestUser` (`guest_auth.go`) creates or **repairs** a single fixed
  account `guest@taskboard.local`, force-setting `disabled=false`,
  `isAdmin=false`, `verified=true`, `registration_status="active"` on every
  call. There is exactly **one** guest identity for the whole instance: every
  anonymous caller receives a token for the *same* account, so guest data is
  shared and any guest write is visible to every other guest.
- `SLOPSTACK_DEVELOPMENT` also drives `DefaultDev: config.Development`
  (`main.go:20`), which changes PocketBase's own behaviour. One flag, two
  unrelated consequences.

Recommended later phase: bind the guest route on a loopback-only listener or
require an explicit second opt-in flag, and make `.env.example` ship
`SLOPSTACK_DEVELOPMENT=false` with a comment explaining how to turn it on.

### 6.3 Activation codes have no delivery channel

`activationCode()` (`app.go:139`) generates 6 digits, stores it in
`users.activation_code`, sets `registration_status="awaiting_activation"`, and
prints it to stdout. There is no mailer: no SMTP config in `config.go`, no
`mailyak` usage in Taskboard's own code (it is a transitive PocketBase
dependency). In OTP registration mode **a new user cannot ever be activated
except by someone with server log access**. Combined with §6.1 row 1, the
activation code is both undeliverable and written to a log that is commonly
shipped to a log aggregator. Decide in a later phase: wire a mailer, or drop OTP
mode from the product surface.

### 6.4 Untested UI surfaces

11 web test files / 34 tests exist. Of the **production-reachable** modules, the
untested ones are the ones that matter:

| untested, app-owned | note |
| --- | --- |
| `src/app.tsx` | the ~21 KB single-page shell — the largest component in the app, zero tests |
| `src/router.tsx` | route table; `calendar-tab` has `validateSearch` reading `?month=` |
| `src/api/client.ts`, `src/api/pocketbase.ts` | all server-state access |
| `src/components/app/auth-screen.tsx` | login/register UI |
| `src/components/app/create-composer.tsx`, `notifications-popover.tsx` | only `create-composer.helpers.test.ts` covers the pure helpers |
| `src/components/tabs/upcoming-tab.tsx`, `calendar-tab.tsx`, `people-tab.tsx`, `search-tab.tsx`, `settings-tab.tsx` | 5 of 6 tabs have no test; **`lists-tab.tsx` is the only tab that does** |
| `src/components/sidebar.tsx`, `entry-row.tsx`, `create-fields.tsx` | |

Covered today: `data.ts`, `theme.ts`, `container-manager.tsx`,
`calendar-month.tsx`, `full-month-calendar.tsx`, `time-grid/` (`layout`,
`position`, `segments`, `time-grid.tsx`), `lists-tab.tsx`.

The pattern worth noting: the tested code is the *hard* logic (layout math,
positioning, recurrence segments); the untested code is the *composition* —
which is where the auth flow and the tab routing live.

### 6.5 OpenAPI surface is almost entirely undocumented

At the Phase 0 audit, `openapi/openapi.json` contained one stub operation for
`GET /api/todos`. **Resolved for Todo CRUD:** the current document contains five
live Huma operations for list/create/read/update/delete, served through the
PocketBase request context bridge. Other custom routes remain outside the Huma
contract; the web app continues to use PocketBase directly for its collection
CRUD.

### 6.6 Toolchain

- `air@latest` is fetched on every `pnpm dev` (`package.json` `dev:server`) —
  unpinned and network-dependent, so a dev environment is not reproducible.
- `docs/COMPATIBILITY.md` §2 carries a note that CI pins pnpm 11 / node 24 while
  the local baseline is pnpm 11.8.0 / node 22.23.2. **That note is now stale**:
  commit `d95d14a` pinned CI to exactly `1.27.1` / `22.23.2` / `11.8.0`. The
  matching `docs/baseline.json` fields `toolVersions.ciNode` (`"24"`),
  `ciPnpm` (`"11"`) and `ciMatchesLocalToolchain` (`false`) were not updated.
  `scripts/check-baseline.py` compares `ci.yml` against its own hardcoded
  constants and passes, so it will not catch this. See §7.

---

## 7. Discrepancies found against the existing `docs/` files

These are recorded rather than fixed, because fixing them means editing four
committed files and that is Phase 1's call. Each was verified with a command.

| # | existing doc says | actual | how verified |
| --- | --- | --- | --- |
| D1 | `ARCHITECTURE.md` §4 and `baseline.json` `schemaDrift.migrationsOnDisk`: **18** migrations | **17** `.go` migration files | `ls apps/server/pb_migrations/*.go \| wc -l` |
| D2 | `baseline.json` `schemaDrift.migrationsAppliedToLiveDb`: **16** | **15** Taskboard migrations applied (23 `_migrations` rows incl. 8 PocketBase internal) | `select file from _migrations` on both `data.db` files |
| D3 | `baseline.json` `inheritedButUnused.webUiComponentsUnused` and `ARCHITECTURE.md` §3.2 list **`dialog`** as unused; both count 8 used / 52 unused | `dialog` **is** used, transitively via `ui/command.tsx` ← `tabs/search-tab.tsx`. Correct: **9 used / 51 unused** | reachability walk from `src/main.tsx` with `@/` resolved |
| D4 | `ARCHITECTURE.md` §3.1 says `components/ui/` has "**78** shadcn/Radix components"; §3.2 of the same file says 60 | **60** | `ls apps/web/src/components/ui \| wc -l` |
| D5 | `EXTRACTION-INVENTORY.md` intro: "**44** `.go` files in the three packages (24 non-test in `internal/app` plus **6** there, 2 in `internal/calendar`, 4 in `internal/reminders`)" | **45** files total: 28 non-test (app 24, calendar 1, reminders 3), 17 test files (app 15, calendar 1, reminders 1). The "6 there" is the test-file count for `internal/app` and should be 15; the calendar/reminders figures are totals, not non-test counts — the arithmetic mixes the two conventions | §0 command 1 |
| D6 | `EXTRACTION-INVENTORY.md` intro: "`internal/app` **8 808** lines across **24** files" as the *non-test* total | 8 808 is the **all-files** total. Non-test is **6 204** across 24 files (15 test files account for 2 604 lines) | `cat internal/app/*.go \| wc -l` vs the non-test subset |
| D7 | `EXTRACTION-INVENTORY.md` §1 heading: "`apps/server/internal/app` — **40** files" | **39** files | `ls internal/app/*.go \| wc -l` |
| D8 | `COMPATIBILITY.md` §3.7: "CI regenerates and diffs this file, so it is part of the compatibility surface" | `openapi/openapi.json` is gitignored (`.gitignore:13`), so CI's `git diff --exit-code` cannot observe it. Same for `apps/web/src/api/generated/`. The gate is ineffective | `ci.yml` + `.gitignore` |
| D9 | `COMPATIBILITY.md` §2 note and `baseline.json` `ciNode`/`ciPnpm`/`ciMatchesLocalToolchain`: CI pins node 24 / pnpm 11, mismatched | CI now pins node 22.23.2 / pnpm 11.8.0 / go 1.27.1 (commit `d95d14a`). `baseline.json` still records the old values | `.github/workflows/ci.yml` env block |
| D10 | `EXTRACTION-INVENTORY.md` §1.1 refers to "audit logging" as a **missing** CORE candidate and points at `docs/AGENT-NOTES.md` | This file does not invent one: there is no audit code, so there is nothing to classify. `audit_logs` is dead schema (D-note in §4.2). The extraction inventory's "either extract a logging primitive or delete the collection" decision still stands | `grep -rn 'audit_logs' apps/server --include=*.go` → 1 hit |

None of D1–D10 affects the **compatibility contract** in
`docs/COMPATIBILITY.md` §7 — C1–C12 are unaffected. D1/D2 shift which migration
is "last applied" but the pending pair and the `dav_data` / `carddav_changes`
drift conclusion are unchanged.

---

## 8. Recommended Phase 1 order

Goal: give Taskboard an independent identity with the database, collection IDs
and credentials untouched.

### 8.0 Ordering constraint — adoption first, rename last

**Adopting upstream packages and renaming the module cannot be one commit.**

- If you adopt first: shared code lives at
  `github.com/slopstack/slopstack/pkg/<name>` and the app imports it. Because
  the app's module *is* `github.com/slopstack/slopstack`, these are
  self-imports — legal in Go, no `require`, no `replace`.
- If you rename first: the module becomes e.g. `github.com/<org>/taskboard`, and
  every `github.com/slopstack/slopstack/pkg/...` import becomes an **external**
  module reference. You would need `require github.com/slopstack/slopstack` plus
  `replace github.com/slopstack/slopstack => ./` to point back at the same
  source tree — a module that depends on itself. That is a self-import cycle
  and does not resolve cleanly.

So: **adoption → adoption → … → rename, last.**

### 8.1 The sequence

Each step must leave `go build ./...`, `go test ./...` (from `apps/server`),
`pnpm -r lint`, `pnpm -r test` and `python3 scripts/check-baseline.py` green.

1. **Fix the baseline docs first (§7).** Correct D1–D9 in
   `docs/baseline.json`, `docs/ARCHITECTURE.md`, `docs/EXTRACTION-INVENTORY.md`
   and `docs/COMPATIBILITY.md`, and extend `scripts/check-baseline.py` to assert
   the migration count and the `ui/` reachability count so they cannot drift
   again. Doing this first means every later phase has a trustworthy gate.
2. **Split `internal/app` (§2.2)** into `internal/auth`, `internal/backup`,
   `internal/dav`, `internal/ical`, `internal/notify`. Pure file moves and
   package renames — no behaviour change, no package moves. This is the
   prerequisite for Phase 3 and the highest-value Phase 1 item.
3. **Adopt upstream packages into the split tree**, in the order given by
   `docs/EXTRACTION-INVENTORY.md` §6: `internal/reminders` → `internal/calendar`
   → `backup_*` → `auth`/`app_passwords`/`guest_auth` → `openapi` → the
   `schema.go` repair loop → the DAV journals → `ownership`. Each lands under
   `github.com/slopstack/slopstack/pkg/<name>` while the module is still
   `github.com/slopstack/slopstack`, so imports are self-imports.
4. **Fix the schema repair passes (§3.3, §3.4)**: delete the duplicate
   `EnsureCalDAVChanges`/`EnsureCardDAVChanges` calls from `schema.go:65-70`, and
   make the three destructive passes log a warning when they are about to
   overwrite a rule that differs from the declared one. Behaviour-preserving
   except for the new log line.
5. **Make the data directory explicit (§5).** This is the highest-risk item and
   it belongs *before* any rename. Stop resolving `pb_data` relative to the
   process CWD: default to a path anchored at the repo root, and have
   `make migrate` / `make build` run from `apps/server` like `pnpm dev:server`
   does. Then decide — explicitly, as a separate commit — whether the empty root
   `pb_data/` is deleted or kept. **Do not** do this in the same commit as the
   `SLOPSTACK_STORAGE_PATH` rename.
6. **Rename the module (§1.1)** — last. One commit: `go.mod` module line, every
   `github.com/slopstack/slopstack/...` import, the `cmd/slopstack` directory
   and binary, `pb.RootCmd.Use`, `@slopstack/web` → `@taskboard/web` and the
   `@slopstack/web` filter reference in the root `package.json` scripts,
   `package.json` `name`, `docker-compose.yml` service/volume names, the
   `Makefile` docker tag, `.air.toml`, and `pb_migrations/README.md`. Hard
   constraint: **no `pb_migrations/` file, no `schema.go`, no `Ensure*`, and no
   data-directory change may appear in this commit.**
7. **Rename the env prefix (§1.2)** — separate, breaking commit, with a
   deprecated fallback for `SLOPSTACK_*` and a `docs/DEPLOYMENT.md` note.
8. **Prune inherited dead code (§4)**: delete the 51 unused `ui/`
   components and `hooks/use-mobile.ts`; evaluate `audit_logs`, `Authorize()`,
   and `Config.SecureCookies` / `Config.LogLevel`. The Todo API item from that
   original checklist is implemented in the current checkout.

Steps 1–2 are safe and unblock everything. Steps 6–8 are where a mistake costs
data, which is why they come last and are one-commit-each.

---

## 9. Not verified / open questions

Recorded so a later phase does not mistake silence for a clean bill of health.

- **`openapi/openapi.json` and `apps/web/src/api/generated/` are gitignored**
  and absent from git, so their content could not be reviewed against a
  committed source of truth. The one-operation finding in §6.5 was read from
  the working-tree copy only.
- **No test runs were executed during this audit.** All "pass" results in
  `docs/COMPATIBILITY.md` §1 and `docs/baseline.json` are inherited from the
  earlier Phase 0 baseline, not re-measured here. This file adds no new test
  results.
- **Neither live database was booted.** Rule-overwrite behaviour in §3.3 is
  determined by reading `schema.go`, `notifications.go`, `caldav_changes.go` and
  `carddav_changes.go` plus `app.go`'s bootstrap order — it is a static reading
  of code that runs on every boot, not an observed execution. A read-only
  reproduction test (boot a throwaway `pb_data` with a deliberately relaxed
  rule, restart, assert) would upgrade this from "determined by inspection" to
  "demonstrated", and is a good candidate for a `http_integration_test.go`
  addition.
- **The 2 `users` rows in `apps/server/pb_data` were not inspected.** Whether
  one is the `guest@taskboard.local` account created by `ensureGuestUser` is
  unconfirmed and was deliberately not read.
- **`docs/DEPLOYMENT.md`, `docs/AUTH.md` and `docs/API.md` were not audited
  line-by-line.** They are short (76 / 28 / 41 lines) but their content was not
  cross-checked against the code beyond the `SLOPSTACK_` occurrence list in §1.
- **Piggybudget was not re-examined** for this document. §6.1 reports only what
  was independently checked in Taskboard's source.
