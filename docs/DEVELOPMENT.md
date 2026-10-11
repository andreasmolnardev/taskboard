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

Taskboard uses Slopstack's shared backup command, encrypted archive creation,
PocketBase store adapter, and restore-archive cleanup. Taskboard configures that
command to keep encrypted-only `backup create`, stdin key import, its
`TASKBOARD_*` and legacy restore guards, and its existing `slopstack_restore_`
temporary archive prefix. The JSON export format, scheduler, and retention
policy remain app-owned.

The shared API changes are being validated against the sibling Slopstack
checkout with a temporary local `replace` in `go.mod`. After runtime `v0.2.0`
is released, remove the replace and pin that release before committing for
hosted CI:

```sh
go mod edit -dropreplace=github.com/andreasmolnardev/slopstack
go get github.com/andreasmolnardev/slopstack@v0.2.0
go mod tidy
```

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
