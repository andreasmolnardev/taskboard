# Taskboard
A todo and calendar app created using Slopstack as a template
Which means that the React client uses the pocketbase js sdk for auth and crud

Supports iCalendar import/export, lossless vCard 3.0/4.0 contacts, CardDAV discovery/query/multiget/sync, and CalDAV event/task sync with ETags, recurrence-aware queries, and sync tokens.

## Sidebar
In desktop its on the left edge, bottom one on mobile
just a centered 'docked' (to the left screen edge) isle with icons for every of the tabs listed below.
When hovering over one show their title to the right
No expand as of now
create the sidebar component under Taskboard's components

## Home: Upcoming View
The home tab shows heading 'Upcoming' with button to filter that opens a popover to filter by type (task/event), list/calendar, date created, date, etc.
underneath it shows the current and next month in a flexbox. lists/calendars get a calendar, when theres an entry timestamped for that date it shows as a dot under the date number
underneath that it shows each list's dot along with the lists name in a flexbox

then it shows upcoming entries as a list. grouped by date. each entry is a row with time (start - end if its an event), the entry's icon, the title in semibold font, description in less weighty same font tho. and to the right it shows the calendar's/list's name

## List View
Two-col layout (1fr 2fr)
vertical list of lists/calendars, then the selected lists items

## Search
Search box opening from the centre of the screen
Two modes: Local (nox extra api calls only using the local cache) and Deep Search (via API call)

## Notifications
A popover displaying active and past notifications.

## Settings
Account settings, light/dark mode, etc.

## Backups

Backups are disabled unless `TASKBOARD_BACKUP_ENABLED=true` is set. Before enabling them, run `taskboard backup key-init` once and save the recovery key somewhere safe; the key is stored in the server data directory and printed only on creation. On a replacement server, install the saved key with `taskboard backup key-import < recovery-key.txt`. Losing the key means encrypted backups cannot be opened.

Scheduled backups use the configured cron (`TASKBOARD_BACKUP_CRON`, default `0 3 * * *`) and retention (`TASKBOARD_BACKUP_MAX_KEEP`, default `14`). PocketBase archives are encrypted with AES-256-GCM before being written to the configured local or S3-compatible backup store. Set `TASKBOARD_BACKUP_JSON_ENABLED=true` to include a JSON data export in the nightly schedule. Create and restore them with `taskboard backup create`, `taskboard backup list`, and `taskboard backup restore <name> --confirm`.

`taskboard backup json` creates an encrypted, versioned JSON data export for todos, events, lists, calendars, address books, and contacts. It is a portable data export, not a full-instance restore image; it does not include users, settings, or uploaded files.

Taskboard now reads `TASKBOARD_*` settings. Existing deployments may keep using `SLOPSTACK_*` during migration; `TASKBOARD_*` takes precedence when both are set. The PocketBase data directory and Docker volume stay at their existing paths.
