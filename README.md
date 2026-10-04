# Taskboard
A todo and calendar app created using Slopstack as a template
Which means that the React client uses the pocketbase js sdk for auth and crud

Supports iCalendar import/export and basic CardDAV contact sync. Full CalDAV calendar sync is planned.

## Sidebar
In desktop its on the left edge, bottom one on mobile
just a centered 'docked' (to the left screen edge) isle with icons for every of the tabs listed below.
When hovering over one show their title to the right
No expand as of now
create sidebar component under slopstack components

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

Backups are disabled unless `SLOPSTACK_BACKUP_ENABLED=true` is set. Before enabling them, run `slopstack backup key-init` once and save the recovery key somewhere safe; the key is stored in the server data directory and printed only on creation. On a replacement server, install the saved key with `slopstack backup key-import < recovery-key.txt`. Losing the key means encrypted backups cannot be opened.

Scheduled backups use the configured cron (`SLOPSTACK_BACKUP_CRON`, default `0 3 * * *`) and retention (`SLOPSTACK_BACKUP_MAX_KEEP`, default `14`). PocketBase archives are encrypted with AES-256-GCM before being written to the configured local or S3-compatible backup store. Set `SLOPSTACK_BACKUP_JSON_ENABLED=true` to include a JSON data export in the nightly schedule. Create and restore them with `slopstack backup create`, `slopstack backup list`, and `slopstack backup restore <name> --confirm`.

`slopstack backup json` creates an encrypted, versioned JSON data export for todos, events, lists, calendars, address books, and contacts. It is a portable data export, not a full-instance restore image; it does not include users, settings, or uploaded files.
