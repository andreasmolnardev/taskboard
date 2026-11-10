# Offline sync API

These routes require a PocketBase user auth token. Cursors, records, and operation receipts are scoped to that user.

## Pull

`GET /api/sync/pull?cursor=0`

Returns up to 500 changes after the supplied cursor:

```json
{
  "cursor": 12,
  "changes": [
    { "collection": "todos", "id": "record-id", "revision": 12, "deleted": false, "record": { "id": "record-id", "title": "Plan" } }
  ]
}
```

`record` is `null` for tombstones. Continue pulling while a response has 500 changes; use the returned cursor on the next request. Cursor `0` seeds a full snapshot from the current `todos`, `events`, `lists`, and `calendars` records into the journal, so later pages use a stable server revision order. Revisions are monotonically assigned per owner. Cursors do not expire in this first version.

## Push

`POST /api/sync/push` accepts up to 100 independent operations. The server handles each operation in its own transaction and returns a result for each, so one invalid operation does not discard successful siblings.

```json
{
  "operations": [{
    "operationId": "stable-client-uuid",
    "collection": "todos",
    "id": "stable-record-id",
    "kind": "update",
    "baseRevision": 8,
    "base": { "title": "Plan", "description": "" },
    "patch": { "title": "Plan review", "description": "Monday" }
  }]
}
```

`kind` is `create`, `update`, or `delete`. Create uses a client-chosen stable `id` and a patch of record fields. Update sends the complete canonical `record` returned by pull as `base`, plus its `baseRevision`; `patch` includes the fields changed locally. The server checks that the base exactly matches that journal revision before merging. Delete sends an empty patch and the base revision. The server ignores client attempts to set `owner`, `id`, and PocketBase timestamps. Supported collections are `todos`, `events`, `lists`, and `calendars`.

If the base revision is stale, the server merges disjoint fields. Event time fields, task completion and due/start fields, recurrence fields, and list/calendar assignment are grouped. Different edits in the same group return `conflict`; an edit whose record was deleted also returns `conflict`. A stale delete returns `conflict`. Conflicts include the latest canonical `record` and `revision`, or `deleted: true` and the tombstone revision. Conflict and permanent error results are saved as receipts too: reusing an operation ID returns the same result. Use a new operation ID after changing an operation.

Recurrence occurrence edits use `kind: "override"` or `kind: "cancel"`, `collection: "events"`, and `id` set to the master event ID. `patch` contains `recurrenceId` (RFC3339 timestamp or `YYYYMMDD`) and, for `override`, a `values` object of event fields. `baseRevision` must match the current master revision. The result record is the generated occurrence override. Cancellation uses `kind: "cancel"`; it leaves a cancelled override tombstone-like event record so future pulls preserve the cancellation.

Each response is `{ "results": [...], "cursor": 14 }`. A result has `operationId`, `status` (`applied`, `conflict`, or `error`), and on success the canonical `revision` and `record`, or `deleted: true`. Recurrence successes also return `masterRevision` and `masterRecord` so queued actions for another occurrence can use the new master revision. `message` gives a short conflict or error reason. The cursor is the latest owner revision after the batch.

## Current limits

Writes made through PocketBase record CRUD, including DAV and imports that use `app.Save`, append journal entries inside the same transaction as the record write. The sync route stores each operation receipt, mutation, and journal entry transactionally. Legacy rows are made visible by the first cursor-zero snapshot. This initial API has no cursor compaction/expiry, no atomic multi-operation dependency batches, and no conflict-resolution endpoint. The server validates owner and container ownership for sync writes; callers should send regular PocketBase-valid field values.
