# Taskboard MCP

Taskboard exposes one OAuth-protected Streamable HTTP endpoint (default `/mcp`). Configure the public URL and a random signing key of at least 32 characters as described in [AUTH.md](AUTH.md). OAuth uses public clients, S256 PKCE, and resource-bound access tokens. Access tokens last one hour. Refresh tokens rotate on use, expire after 30 days, and are stored as hashes. Users can revoke a connected MCP client from **Settings → Account**.

All tools operate only on the authenticated user's data. A task's list must also belong to that user. Project tools use Taskboard lists with project mode enabled.

## Task tools

- `list_tasks`: search and filter by list, status, priority, scheduled date range, or undated status. `limit` is 1–100.
- `get_task`: read one task.
- `create_task`: create a scheduled or undated task in one of the user's lists.
- `update_task`: edit the title, Markdown description, status, priority, list, completion, or due date. Set `due_date` to an empty string to unschedule a task.
- `delete_task`: delete one task.

Delete tools require `confirm: true`. Checklist items are stored with tasks and remain available through the task record, but dedicated MCP checklist CRUD is still outstanding.

Task statuses use the stored iCalendar values `NEEDS-ACTION`, `IN-PROCESS`, `COMPLETED`, and `CANCELLED`.

## Project tools

- `list_projects`: list or search active projects, optionally filtered by status.
- `get_project`: read project metadata, progress, tasks, undated backlog, upcoming deadlines, and overdue tasks.
- `create_project`: create a project list with a goal/description, optional home URL, and target date.
- `update_project`: update metadata or status, or archive the project.

Project progress is calculated from task completion and is not stored as a percentage.

## Calendar tools

- `list_calendars`: list the user's calendars.
- `list_events`: search events, filter by calendar and start range, or set `upcoming` to list future events.
- `get_event`, `create_event`, `update_event`, `delete_event`: event CRUD. Creation and moves validate calendar ownership. Recurrence rules can be read and written as stored RRULE values; selecting an individual occurrence or applying changes to a series is not yet supported through MCP.

## Contact tools

- `list_contacts`: search contacts and optionally filter by address book.
- `get_contact`, `create_contact`, `update_contact`, `delete_contact`: contact CRUD. Writes validate address-book ownership.

All deletes require an explicit `confirm: true` argument. A missing or false value is rejected.

## Current limits

Recurring-series operations, dedicated checklist CRUD, and shared-resource permissions are not available through MCP. No project membership or shared calendars are implemented yet.
