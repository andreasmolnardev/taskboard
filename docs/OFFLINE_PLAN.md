# Offline and sync build plan

## Goal

After one online visit, users can reopen Taskboard offline, read saved tasks and events, and create, edit, complete, or delete them. Changes survive reloads and sync when the app is open and a connection returns. Concurrent edits must not silently erase data.

Start with tasks, events, lists, calendars, and recurrence edits. Keep account changes, sharing, imports, exports, and deep search online. Show cached contacts and notifications as read-only until their sync behavior is defined.

## Current state

- Vite has no PWA plugin or service worker.
- `app.tsx` loads records directly from PocketBase into memory.
- Entry rows, the composer, and container settings write directly to PocketBase or custom API routes.
- Shared React Query helpers have optimistic updates but no durable cache or outbox.
- CalDAV has a change journal; check its scope before reusing any journal code for web sync.

## Merge rules

Use revision-based, three-way merging first. This gives safe offline edits without claiming full CRDT behavior.

- Store the base revision and base values with each local edit.
- Merge changes to independent fields automatically.
- Treat linked values as one group: event start/end/time zone, task completion/status/time, recurrence rules/overrides, and container assignment.
- If both sides change the same field group differently, keep both versions and ask the user to resolve it.
- Treat delete versus edit as a conflict; do not silently resurrect or discard records.
- Make retries idempotent with a unique operation ID and a server receipt.
- Keep text fields whole in the first release. Character-level collaborative merging would need a separate CRDT design and storage format.

## 1. Define the sync contract

- Inventory all read/write paths, including DAV, ICS imports, occurrence endpoints, and container deletion.
- Define record revisions, change cursors, tombstones, operation receipts, conflict payloads, and cursor expiry.
- Define atomic batches for dependent changes and recurrence operations.
- Use server-assigned revisions and ordering; do not trust client clocks for conflict decisions.
- Scope cursors and receipts to the authenticated account. Recheck access on every push and pull.

Done when the contract covers retries, concurrent writes, deletes, and all existing server write paths.

## 2. Add server sync support

- Add new PocketBase migrations for sync metadata; preserve applied migrations.
- Record changes and deletions transactionally for every supported write path, including DAV and imports.
- Add paged snapshot/pull and push endpoints. A snapshot must have a consistent cursor so writes during initial download are not missed.
- Apply each operation with revision checks, validation, and its receipt in one transaction.
- Return canonical records, conflicts, and explicit permanent errors.
- Set retention rules for tombstones and receipts. Expired cursors require a fresh snapshot while preserving pending local edits.
- Update OpenAPI and run API/client generation. Never edit generated client files by hand.

Done when duplicate pushes have one effect and pull includes changes and deletions from every supported writer.

## 3. Build durable local storage

- Add IndexedDB stores for records, server base snapshots, pending operations, conflicts, and sync cursors.
- Partition data by account and server. Clear account data on logout and prevent late requests from writing into another session.
- Commit each local edit and its outbox operation in one transaction before showing it as saved.
- Assign stable IDs for offline creates and preserve references to new containers.
- Handle storage failure explicitly so the UI never claims an unsaved edit is saved.
- Support local schema upgrades without dropping pending edits.

Done when offline edits survive reloads and account changes cannot expose another account's cache.

## 4. Route the app through local data

- Add one repository layer for reads and writes; replace direct calls in the main loader, composer, entry rows, and container controls.
- Render from local records and subscribe views to local changes.
- Run local search against stored data. Mark deep search as requiring a connection.
- Make cached recurrence views work offline using a shared, tested expansion rule or an explicit cached date range. Never show missing occurrences as an empty calendar without explanation.

Done when supported screens and actions work with the network disabled, including after reload.

## 5. Add the sync worker

- Pull changes, rebase pending edits, push eligible operations, then pull canonical results.
- Preserve operation order and dependencies. Block dependent operations when a parent operation fails or conflicts.
- Retry transient failures with backoff; pause on expired auth and resume after sign-in.
- Sync on startup, reconnect, focus, and an explicit retry action. Use realtime events to trigger pulls, with cursors covering missed events.
- Coordinate tabs with a single sync leader and server idempotency as the final guard.
- Keep edits made during an in-flight request separate from the acknowledged operation.

Done when reconnects, crashes, retries, and multiple tabs converge without losing queued work.

## 6. Add the PWA shell and sync UI

- Add a manifest, install icons, and a service worker that caches versioned app assets and provides an offline navigation fallback.
- Keep authenticated API data in IndexedDB; do not cache auth or private API responses in the service worker.
- Show offline state, pending edit count, failed changes, conflicts, and last successful sync.
- Add conflict resolution that preserves both values until a resolution is saved and acknowledged.
- Make app updates safe with pending edits and local schema versions.
- Treat background sync as optional. Promise sync when the app is open; browsers may suspend closed apps.

Done when the installed app launches offline after an online visit and users can tell whether edits are local, synced, or blocked.

## 7. Test and ship

- Unit tests: merge rules, linked field groups, operation ordering, retries, and schema upgrades.
- Server tests: concurrent writes, duplicate operation IDs, transactional journal writes, authorization, cursor expiry, and snapshot consistency.
- Browser tests: offline launch, CRUD plus reload, reconnect, two clients changing different/same fields, delete/edit conflicts, recurrence edits, multiple tabs, expired auth, and storage failure.
- Test PWA install and offline behavior on desktop Chrome and mobile Safari, including a service worker update with queued edits.
- Roll out behind a flag. Track failed syncs, conflict counts, and outbox age without logging private record contents.

Release gate: no silent edit loss in the scenarios above, all supported writers participate in sync, and pending edits remain recoverable after failures.

## Build order

1. Sync contract and merge rules.
2. Server journal, revisions, and idempotent endpoints.
3. IndexedDB repository and durable outbox.
4. UI migration and sync worker.
5. PWA shell, status, and conflict resolution.
6. End-to-end validation and staged release.

## Implementation notes

The first implementation is described in [OFFLINE.md](OFFLINE.md). Dependent creates sync in order and block when a parent fails; the server commits each operation independently. Occurrence changes and their journal entries commit atomically. A multi-operation atomic batch endpoint is not included.

Journal entries and receipts are retained until account deletion, so cursors do not expire. The UI exposes queue counts, conflict counts, errors, and last sync time; external telemetry and journal compaction remain operational follow-ups. Automated checks cover the server, local repository, and production app in desktop/mobile Chromium. Physical mobile Safari installation, suspension, and update validation remain before broad rollout.
