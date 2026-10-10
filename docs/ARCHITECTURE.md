# Taskboard — Architecture

Baseline audit, Phase 0. Everything here was verified against the working tree at
commit `75b827a` ("wip: checkpoint pre-migration working tree"). No behaviour is
described that is not present in code.

---

## 1. Repository layout

```
taskboard/
├── apps/
│   ├── server/                     Go / PocketBase backend
│   │   ├── cmd/slopstack/main.go   the only binary entrypoint
│   │   ├── internal/app/           the application package (~44 .go files)
│   │   ├── internal/calendar/      iCalendar recurrence expansion
│   │   ├── internal/reminders/     reminder domain model + scheduler
│   │   ├── pb_migrations/          18 PocketBase JS migrations (Go files)
│   │   ├── pb_data/                live dev database
│   │   └── .air.toml               hot-reload config for `air`
│   └── web/                        React 19 + Vite + TanStack SPA
│       └── src/
│           ├── app.tsx             single-page shell (21 KB)
│           ├── router.tsx          TanStack Router route table
│           ├── api/                PocketBase client + generated OpenAPI client
│           ├── components/         app UI (tabs/, app/, time-grid/, container-manager/, ui/)
│           ├── hooks/use-mobile.ts
│           └── lib/utils.ts
├── openapi/openapi.json            generated API contract (gitignored)
├── pb_data/                        STRAY root-level database (see §7)
├── docs/                           this audit + existing docs
├── scripts/, skills/, .github/workflows/
└── Dockerfile, docker-compose.yml, Makefile, go.mod, package.json
```

### 1.1 Copy-paste identity problem (must be fixed in Phase 1)

Taskboard was copied from the Slopstack template and still carries the template's
identity verbatim:

| Surface | Current value | Should be |
| --- | --- | --- |
| `go.mod` module | `github.com/slopstack/slopstack` | `github.com/<org>/taskboard` |
| `apps/web/package.json` name | `@slopstack/web` | `@taskboard/web` |
| root `package.json` name | `slopstack` | `taskboard` |
| root `package.json` packageManager | `pnpm@11.8.0` | — |
| `pnpm dev:server` | `go run github.com/air-verse/air@latest` | — |
| binary name | `apps/server/cmd/slopstack` | — |
| CLI root command | `pb.RootCmd.Use = "slopstack"` | — |
| all env vars | `SLOPSTACK_*` | `TASKBOARD_*` |
| Docker compose service / image | `slopstack`, `slopstack-data` | — |

The `SLOPSTACK_*` env prefix is load-bearing for deployed instances; changing it is
a breaking configuration change and is tracked in `docs/AGENT-NOTES.md`.
`pb.RootCmd.Use = "slopstack"` also leaks into CLI help text and, because
`restore` re-executes the process, into the restart command line.

---

## 2. apps/server

### 2.1 Entrypoint — `cmd/slopstack/main.go` (~60 lines)

1. `app.LoadConfig()` reads every setting from the environment (see §5).
2. `pocketbase.NewWithConfig(...)` with `DefaultDataDir: config.StoragePath`
   (default `pb_data`) and `DefaultDev: config.Development`.
3. `newOpenAPI(config)` builds a **separate** Huma API backed by its own
   `http.NewServeMux` / `humago` adapter. The bearer security scheme is named
   `pocketbaseAuth`.
4. `app.Register(pb, config, api)` — see §2.3.
5. `registerExportCommand` adds `slopstack openapi export` (writes
   `openapi/openapi.json`).
6. `pb.RootCmd.AddCommand(app.NewBackupCommand(pb).Command())` — see §2.6.

PocketBase owns all its stock commands (`serve`, `migrate`, `superuser`,
`migratecollections`, …). This app adds `openapi export` and `backup …`.

### 2.2 `internal/app` — the monolith

All server behaviour lives in one Go package (`package app`) of roughly 44 files
(~600 KB). It mixes, without any internal boundary:

- configuration (`config.go`)
- authentication and registration policy (`auth.go`, `guest_auth.go`,
  `app_passwords.go`, `app_password_throttle.go`)
- authorization (`authorization.go`) and ownership enforcement (`ownership.go`)
- declarative schema repair (`schema.go`) and DAV change journals
  (`caldav_changes.go`, `carddav_changes.go`)
- CalDAV (`caldav.go`, 977 lines), CardDAV (`carddav.go`, 876 lines),
  vCard parsing/serialisation (`vcard.go`, 395 lines)
- ICS import/export (`calendar_io.go`, 596 lines)
- recurrence occurrence expansion (`occurrences.go`, 480 lines)
- search (`search.go`, 292 lines)
- notifications + reminders wiring (`notifications.go`, 506 lines)
- backup subsystem (`backup_command.go`, `backup_encryption.go`,
  `backup_formats.go`, `backup_scheduler.go`)
- OpenAPI surface plumbing (`openapi.go`, `app.go`)

There is **no** `internal/auth`, `internal/dav`, `internal/backup`, … package. See
`docs/EXTRACTION-INVENTORY.md` for the file-level split.

### 2.3 `Register()` — the wiring seam

`internal/app/app.go:18` is the single composition root. In order:

**OnBootstrap**
1. `cleanupTransientBackupArchives(e.App.DataDir())`
2. `e.App.RunAppMigrations()`
3. `EnsureCollections(e.App)` — repair/creation of the core schema
4. `EnsureNotificationCollections(e.App)`
5. `EnsureCalDAVChanges(e.App)`
6. `EnsureCardDAVChanges(e.App)`
7. `applyBackupConfig(e.App, config)` — maps config onto PocketBase settings

**Other hooks**
- `protectUserAdminFlag` — registration policy + anti-privilege-escalation
- `registerOwnershipHooks` — owner/container enforcement on writes
- `registerCalDAVChangeHooks`, `registerCardDAVChangeHooks` — sync journals
- `registerReminderScheduler`, `registerEncryptedBackupScheduler` — cron jobs
- `OnRecordAfterCreateSuccess("users")` → `provisionDefaultContainers` — seeds
  `Tasks` (list), `Events and Holidays` (calendar), `Contacts` (address book)

**OnServe** → `registerRoutes` (§6 of `docs/COMPATIBILITY.md`).

**Huma** → `registerTodoAPI(api)` registers a *documentation-only* operation
`listTodos` (`GET /api/todos`) whose handler returns `nil, nil`; the real route is
a plain PocketBase router handler in `registerTodoRoutes`. This is deliberate:
the Huma layer exists purely to publish the `/api/*` contract in `openapi.json`.

### 2.4 `internal/calendar`

`recurrence.go` (+ test). Single exported entry point:

```go
func Expand(component *ical.Component, windowStart, windowEnd time.Time,
            maxOccurrences int) ([]Occurrence, error)
```

Wraps `github.com/emersion/go-ical` + `github.com/teambition/rrule-go`. Expands
into the half-open window `[windowStart, windowEnd)`. Returns `ErrLimitExceeded`
rather than a partial result. Sentinels: `ErrInvalidWindow`, `ErrInvalidLimit`,
`ErrLimitExceeded`.

### 2.5 `internal/reminders`

- `reminder.go` — `Reminder` value type, status transitions
  (`delivered`, `deliveryFailed`), `RetryPolicy` with `Validate()`
- `memory_store.go` — in-memory `Store` used by tests
- `scheduler.go` — `Scheduler{Store, Sender, RetryPolicy, BatchSize}` with
  `RunDue(ctx, now)`: claims one bounded batch atomically, sends each, records the
  result; a failure does not abort the rest of the batch
- `reminders_test.go`

---

## 3. apps/web

- **React 19.2 + Vite 7 + TypeScript 5.9.** Single SPA, no SSR.
- **TanStack Router** — `src/router.tsx` defines a flat route tree under one root
  route whose component is `App`:

  | path | component |
  | --- | --- |
  | `/` | Upcoming tab |
  | `/calendar` | Calendar tab (`validateSearch` reads `?month=`) |
  | `/lists` | Lists tab |
  | `/people` | People tab |
  | `/search` | Search tab |
  | `/settings` | Settings tab |
  | `/auth/login` | Auth screen |
  | `/auth/register` | Auth screen |

  There is no nested layout, no loader, and no auth guard route — auth state is
  handled inside `app.tsx`.
- **TanStack Query 5** for server state; **PocketBase JS SDK 0.26** (`pocketbase`
  dependency, `src/api/pocketbase.ts`) for the realtime/records API.
- **Generated OpenAPI client** — `src/api/generated/` produced by
  `@hey-api/openapi-ts` from `../../openapi/openapi.json` via
  `pnpm generate:client`. The whole directory is **gitignored**; CI regenerates it
  and fails on diff.
- `src/data.ts` (shared PocketBase collection/field descriptors) and
  `src/theme.ts` (theme parsing) are the only non-component logic modules.
- `src/styles.css` is 35 KB of hand-written CSS (Tailwind v4 is installed and
  used only for the utility layer in components).

### 3.1 Component groups

| path | contents | used by |
| --- | --- | --- |
| `components/tabs/` | `upcoming-tab`, `calendar-tab`, `lists-tab`, `people-tab`, `search-tab`, `settings-tab` | `app.tsx` via the route table |
| `components/app/` | `auth-screen`, `create-composer` (+ `.helpers`), `notifications-popover` | `app.tsx` |
| `components/time-grid/` | `time-grid.tsx`, `layout.ts`, `position.ts`, `segments.ts`, `dates.ts`, `types.ts` | `calendar-tab.tsx` |
| `components/container-manager/` | `container-manager.tsx` + `index.ts` | `calendar-tab.tsx`, `lists-tab.tsx`, `settings-tab.tsx` |
| top level | `sidebar.tsx`, `entry-row.tsx`, `create-fields.tsx`, `calendar-month.tsx`, `full-month-calendar.tsx` | `app.tsx`, tabs, composer |
| `components/ui/` | 78 shadcn/Radix components | see §3.2 |
| `hooks/use-mobile.ts` | viewport breakpoint hook | only `components/ui/sidebar.tsx`, which is itself unused ⇒ transitively dead |

### 3.2 Inherited-but-unused web code

`components/ui/` ships **60** components; only **8** are imported anywhere in
`src/`:

| component | imported by |
| --- | --- |
| `button`, `input`, `label`, `switch`, `textarea` | `components/container-manager/container-manager.tsx` |
| `command` | `components/tabs/search-tab.tsx` |
| `tabs` | `calendar-tab.tsx`, `settings-tab.tsx`, `create-composer.tsx` |
| `tooltip` | `main.tsx` (`TooltipProvider`) |

The remaining **52** are inherited shadcn scaffolding never referenced by
Taskboard (verified: no import of the file anywhere under `src/`):

```
accordion, alert, alert-dialog, aspect-ratio, attachment, avatar, badge,
breadcrumb, bubble, button-group, calendar, card, carousel, chart, checkbox,
collapsible, combobox, context-menu, dialog, direction, drawer, dropdown-menu,
empty, field, hover-card, input-group, input-otp, item, kbd, marker, menubar,
message, message-scroller, native-select, navigation-menu, pagination, popover,
progress, radio-group, resizable, scroll-area, select, separator, sheet, sidebar,
skeleton, slider, sonner, spinner, table, toggle, toggle-group
```

Note `components/ui/sidebar.tsx` (the shadcn sidebar) is unused — Taskboard's
nav is the hand-written `components/sidebar.tsx`.

Server-side inherited-but-unused:

| item | note |
| --- | --- |
| `internal/app/authorization.go` | `Authorize()` exists and is a stub: it allows superusers and **forbids everyone else**; no group or `permissions`/`user_permissions` lookup is implemented. `groups`, `permissions`, `group_members`, `group_permissions`, `user_permissions` collections exist with **empty API rules** (admin-only via PocketBase superuser) but are never read by any route. |
| `search.go:73` comment | says `registerSearchRoutes` "is intentionally not wired into `registerRoutes` yet" — **stale**; `app.go:198` does call it, so `GET /api/search` is live. |
| `registerTodoRoutes` | returns a hard-coded empty array; the Huma op is a stub. Placeholder endpoint, not inherited, but it is dead surface. |
| `internal/app/app_password_throttle.go` | not unused — it backs `AuthenticateAppPassword`. Listed here only because it is a copy-identical file across repos. |

---

## 4. Schema ownership

Schema is defined **twice**, which is the primary drift risk.

**(a) `apps/server/pb_migrations/*.go`** — 18 PocketBase migrations, dated
`20260824…` → `20260920000001`, imported for side effects by `main.go`:

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

**(b) `internal/app/schema.go` `EnsureCollections()`** — a declarative table of
`{name, auth, fields[], rule}` that runs on **every boot**, after migrations. It
is written as a *repair* pass, not an authority:

- if the collection exists → `addMissingFields` only adds absent fields; it never
  removes, retypes, or reorders, and never touches existing field constraints
- a `Rule` is only applied when `collection.ListRule == nil`
- if the collection is missing → it is created from scratch with the declared rule
  applied to **all five** rules

`EnsureNotificationCollections`, `EnsureCalDAVChanges` and `EnsureCardDAVChanges`
are stronger: they set all five rules **unconditionally** and add missing indexes.
So `notifications`, `reminders`, `caldav_changes`, `carddav_changes` are
schema.go-owned, while `users`/`todos`/`events`/… are migration-owned with
schema.go acting as a top-up.

**Observed drift.** `schema.go` declares `todos.dav_data` and `events.dav_data`
text fields; migration `20260920000000_add_dav_resource_data` is what actually
adds them and the live database contains neither (see `docs/baseline.json` field
lists). `schema.go` also declares `users` with no rule, while the live `users`
collection carries `id = @request.auth.id` on list/view/update/delete — set by
migration `20260824000000_add_user_is_admin`, never touched by `EnsureCollections`.

---

## 5. Configuration

`internal/app/config.go` — no config file, environment only.

| Env var | Default | Maps to |
| --- | --- | --- |
| `SLOPSTACK_HTTP_ADDRESS` | `0.0.0.0:8090` | PB `--http` flag default |
| `SLOPSTACK_PUBLIC_URL` | `http://localhost:8090` | OpenAPI server URL + `settings.Meta.AppURL` |
| `SLOPSTACK_SECURE_COOKIES` | `false` | `Config.SecureCookies` (**declared, never read**) |
| `SLOPSTACK_REGISTRATION_ENABLED` | `true` | `false` ⇒ `RegistrationDisabled` |
| `SLOPSTACK_REGISTRATION_MODE` | `approval` | `disabled` \| `approval` \| `otp` |
| `SLOPSTACK_LOG_LEVEL` | `info` | `Config.LogLevel` (**declared, never read**) |
| `SLOPSTACK_STORAGE_PATH` | `pb_data` | PocketBase data dir |
| `SLOPSTACK_DEVELOPMENT` | `false` | `DefaultDev`; enables the guest route and Redoc |
| `SLOPSTACK_BACKUP_ENABLED` | `false` | master switch for PB cron + S3 + the app's own cron |
| `SLOPSTACK_BACKUP_JSON_ENABLED` | `false` | also emit encrypted JSON export on schedule |
| `SLOPSTACK_BACKUP_CRON` | `0 3 * * *` | app-level scheduler cron |
| `SLOPSTACK_BACKUP_MAX_KEEP` | `14` | retention for `.zip.enc` and `.json.enc` |
| `SLOPSTACK_BACKUP_S3_{ENABLED,BUCKET,REGION,ENDPOINT,ACCESS_KEY,SECRET,FORCE_PATH_STYLE}` | off/empty | PocketBase S3 backup target |
| `SLOPSTACK_BACKUP_RESTORE_RESTARTED` | — | internal, guards the restore re-exec loop |

`applyBackupConfig` deliberately **zeroes** `settings.Backups.Cron` — PocketBase's
own cron backups are disabled and replaced by `registerEncryptedBackupScheduler`,
which produces the encrypted archives instead.

---

## 6. Data locations

| path | what | gitignored |
| --- | --- | --- |
| `apps/server/pb_data/` | the live dev database (`data.db`, `auxiliary.db`, WALs, `.notify/`) | yes |
| `pb_data/` (repo root) | **stray** second database — `data.db` 372 KB + `auxiliary.db`, last written 2026-09-19 | yes |
| `apps/server/tmp/` | `air` build output + `./tmp/slopstack` binary | yes |
| `<StoragePath>/.backup-encryption-key` | 32-byte base64url AES key, mode `0600` | yes |
| `<StoragePath>/backups/` | PocketBase backups filesystem: `*.zip`, `*.zip.enc`, `*.json.enc` | yes |
| `apps/server/pb_data/*-wal`, `*-shm` | uncommitted SQLite WAL (1.2 MB WAL present) | yes |

The root `pb_data/` is a **duplicate** produced by running the binary from the
repo root (the Makefile's `go build ./apps/server/cmd/slopstack` and a bare
`./slopstack serve` do exactly that). Its collection set and schema must not be
assumed identical to `apps/server/pb_data`; `apps/server/pb_data` is the one the
dev loop uses because `pnpm dev:server` `cd`s into `apps/server` first.

---

## 7. Dev loop

| command | effect |
| --- | --- |
| `pnpm dev` | `concurrently` runs `dev:server` + `dev:web` |
| `pnpm dev:server` | `cd apps/server && go run github.com/air-verse/air@latest -c .air.toml` |
| `pnpm dev:web` | `pnpm --filter @slopstack/web dev` → `vite` |
| `pnpm build` | `pnpm -r build` (web: `tsc -b && vite build`) |
| `pnpm lint` / `pnpm test` | recursed to workspace packages |
| `pnpm generate:api` | `go run ./apps/server/cmd/slopstack openapi export` |
| `pnpm generate:client` | `openapi-ts -i ../../openapi/openapi.json -o src/api/generated` |
| `make dev/build/test/lint/generate/migrate/docker` | thin wrappers over the above |

`apps/server/.air.toml`: builds `./tmp/slopstack` from `./cmd/slopstack`, runs
`SLOPSTACK_DEVELOPMENT=true ./tmp/slopstack serve --http=${SLOPSTACK_HTTP_ADDRESS:-0.0.0.0:8090}`,
watches `go,json,yaml,yml,toml`, excludes `tmp`, `pb_data`, `node_modules`.
Note `air@latest` is fetched on every `pnpm dev` — unpinned and network-dependent.

`docker-compose.yml` builds the root `Dockerfile`, maps `8090:8090`, sets
`SLOPSTACK_STORAGE_PATH=/app/pb_data`, enables backups, and mounts the named
volume `slopstack-data` at `/app/pb_data`.