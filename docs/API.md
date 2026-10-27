# API

## Runtime

- Backend: Go and PocketBase.
- Default address: `http://localhost:8090`.
- Frontend proxy: Vite forwards `/api` to the backend.
- OpenAPI JSON: `/openapi.json`.
- OpenAPI YAML: `/openapi.yaml`.
- ReDoc: `/docs`.

## Current example endpoint

`GET /api/todos` is protected by PocketBase auth and currently returns an empty example response. It is intentionally minimal starter infrastructure.

```http
GET /api/todos
Authorization: Bearer <pocketbase-token>
```

Unauthenticated requests return `401`.

## Calendar import and export

`POST /api/calendars/import` accepts multipart form data with `calendar`, optional `list`, and `file` fields. The target calendar and list must belong to the signed-in user. VEVENT components use `calendar`; VTODO components use `list`. The import accepts files up to 10 MiB and updates existing components with the same owner, type, and UID instead of creating duplicates.

`GET /api/calendars/{id}/export.ics` downloads the signed-in user's calendar as an RFC 5545 iCalendar file.

Both endpoints require a PocketBase bearer token.

## Account data

`GET /api/account/export` downloads a JSON export containing the signed-in user's profile and owned app data, including tasks, events, lists, calendars, contacts, reminders, notifications, sync state, and app-password metadata. Secret hashes and auth credentials are not exported. The response is private and marked `no-store`.

`POST /api/account/delete` permanently deletes the signed-in account and its owned records in one transaction. It requires a PocketBase bearer token and a JSON body with `confirmEmail` matching the current account email:

```json
{"confirmEmail":"you@example.com"}
```

It returns `204 No Content` on success. Export important data before deletion; deletion cannot be undone except by restoring a server backup.

## Extending a module

1. Define request and response types in the Go module.
2. Register the operation with Huma and add security metadata.
3. Enforce auth and permission checks in the route handler.
4. Read and write PocketBase records using the current user as owner.
5. Export OpenAPI with `pnpm generate:api`.
6. Generate the client with `pnpm generate:client`.
7. Use generated client calls from the React feature.

Never treat frontend visibility as authorization. Backend `Authorize` is the enforcement point.
