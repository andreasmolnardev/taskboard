# Development

## Setup

```sh
pnpm install
cp .env.example .env
pnpm generate:api
pnpm generate:client
pnpm dev
pnpm config:check
```

The Go module and web workspace package use Taskboard identities. The server
entrypoint is `apps/server/cmd/taskboard`; Go builds use module
`github.com/andreasmolnardev/taskboard`.

Taskboard's backup archive and JSON formats remain app-owned. The shared
Slopstack dependency supplies auth, configuration, authorization, HTTP, and
backup primitives. The local `go.mod` replace points to
`../slopstack/slopstack` for coordinated development. Hosted CI and release
workflows do not provision that sibling checkout. Before standalone hosted
builds, pin a released Slopstack module version and remove the local replace. No
released version is available in this coordinated change.

Vite serves the web app on `5173`. Go and PocketBase serve the API on `8090`.

## Checks

```sh
pnpm --filter @taskboard/web lint
pnpm --filter @taskboard/web build
pnpm --filter @taskboard/web test
go test ./...
pnpm compatibility:fixture
```

## Feature workflow

1. Add or update the backend schema and migration.
2. Add protected API operations.
3. Export OpenAPI and regenerate the client.
4. Build feature UI from shadcn primitives.
5. Add behavior tests.
6. Run client and server checks.

Generated output under `apps/web/src/api/generated` must not be edited manually.
