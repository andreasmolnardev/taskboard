# Taskboard Roadmap

## Current foundation

- PocketBase auth and persistent tasks, events, lists, calendars, and contacts.
- Day, week, month, upcoming, list, people, settings, and search views.
- List/calendar assignment, container management, task completion, and deletion.
- ICS event and task import/export with UID-based updates.
- Bounded recurrence engine and occurrence API.
- Deep server search.
- In-app reminder and notification backend.
- Basic CardDAV and beta CalDAV protocol support.
- Scheduled local or S3-compatible backups plus manual backup/restore CLI.
- Initial server and web test suites.

## Near term: make core calendar behavior reliable

1. Finish time-zone storage and display.
   - Preserve IANA `TZID` values and floating times.
   - Store all-day values as dates, not UTC instants.
   - Test DST changes and exclusive all-day end dates.
2. Finish recurring-series editing.
   - Edit one occurrence, future occurrences, or the whole series.
   - Persist detached overrides and cancelled occurrences.
   - Add a friendly repeat editor instead of raw RRULE fields.
3. Finish reminders in the UI.
   - Create structured reminders from the composer.
   - Add per-item read, snooze, and dismiss actions.
   - Add optional email and Web Push delivery later.
4. Harden calendar and list ownership.
   - Use validated relations or server hooks for container assignment.
   - Add default-container selection and safe archive behavior.

## CalDAV readiness

1. Persist stable client-selected resource hrefs.
2. Make ETag checks and writes transactional.
3. Use recurrence-aware time-range queries.
4. Add deterministic query limits and deletion tombstones.
5. Add sync tokens and `sync-collection` support.
6. Preserve complete calendar objects, including recurrence overrides and unknown properties.
7. Test with Apple Calendar, Thunderbird, and DAVx5.

CalDAV remains beta until these items pass interoperability tests.

## Product work

- Add drag-to-create, drag-to-move, and resize interactions to time grids.
- Add multi-day layout, keyboard navigation, and stronger mobile views.
- Open deep-search results in the correct editor or detail view.
- Add shared calendars, roles, invitations, attendee replies, and free/busy lookup.
- Add offline/PWA caching, queued edits, realtime updates, and conflict handling.
- Add account export and deletion flows.

## Operations and quality

- Add a subprocess-based automated restore drill.
- Monitor backup age and report failed schedules.
- Publish import/export and DAV endpoints in OpenAPI.
- Add browser end-to-end tests for CRUD, recurrence, import/export, reminders, and sync.
- Verify static frontend serving through the Go runtime.
- Add rate limits and structured audit logs for auth, import, search, and DAV routes.
