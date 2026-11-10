# Offline use and sync

Taskboard saves tasks, events, lists, and calendars on the device in IndexedDB. After an online visit finishes syncing and the service worker is installed, the production app can reopen offline. Changes save locally before the UI reports success. The open app syncs on reconnect, focus, realtime hints, or **Sync now**.

Edits to separate fields merge. Changes to the same field, linked date/time fields, or delete versus edit stay in conflict until the user chooses a version. This is revision-based merging, not a text CRDT. Sync retries use durable operation IDs so a lost response does not duplicate a change.

Recurring event views cache the fetched date range, from the start of the current month last year to the start of that month two years ahead. Pending occurrence overrides and cancellations are shown over that saved range. Changes to a series require the server to expand the new series; the UI marks saved ranges and falls back to base events with a message if no range is cached. Contacts and notifications use saved read-only snapshots offline. Reminders, sharing, account changes, imports, exports, and deep search need a connection.

Caches are split by server and account. Logout clears the active account's local data, including pending edits. Signing in again after sync auth expires preserves pending edits. Sync across closed or suspended apps is not promised.

## Deploy

Serve the production build over HTTPS (localhost also works). The service worker caches only versioned public app assets, never authenticated API responses. New app shells wait for old tabs to close before activation. Keep `/sw.js` available at the origin root and allow browser update checks to reach it.

Offline sync is enabled by default. Build with `VITE_OFFLINE_ENABLED=false` to use direct PocketBase reads and writes during a staged rollout. This flag leaves stored queues intact; sync pending changes before disabling offline mode for users.

The server migration adds private sync journal and receipt collections. Existing data is seeded into the journal on the first pull. Journal entries and receipts are retained until account deletion; cursors do not expire. Monitor storage growth before adding compaction. The API contract is in [OFFLINE_SYNC_API.md](OFFLINE_SYNC_API.md).

## Checks

- `pnpm --filter @slopstack/web test`
- `go test ./apps/server/...`
- `pnpm --filter @slopstack/web test:browser`

The browser check uses a production build and a simulated sync transport to test offline launch, task create/edit/complete/delete, reload persistence, and reconnect. Server tests cover the real sync handlers separately. Install Chromium with `pnpm --filter @slopstack/web exec playwright install chromium` before the browser check. Physical iOS installation, suspension, and update checks still need device validation.
