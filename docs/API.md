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

## Extending a module

1. Define request and response types in the Go module.
2. Register the operation with Huma and add security metadata.
3. Enforce auth and permission checks in the route handler.
4. Read and write PocketBase records using the current user as owner.
5. Export OpenAPI with `pnpm generate:api`.
6. Generate the client with `pnpm generate:client`.
7. Use generated client calls from the React feature.

Never treat frontend visibility as authorization. Backend `Authorize` is the enforcement point.
