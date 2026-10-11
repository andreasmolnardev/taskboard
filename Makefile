.PHONY: dev build test lint docker migrate generate config-check compatibility deploy release

dev:
	pnpm dev

build:
	pnpm build
	go build ./apps/server/cmd/taskboard

test:
	pnpm test
	go test ./...

lint:
	pnpm lint
	gofmt -d apps/server

generate:
	pnpm generate:api
	pnpm generate:client

config-check:
	node scripts/check-taskboard-config.mjs

deploy:
	pnpm deploy

release:
	pnpm release

compatibility:
	sh scripts/taskboard-compatibility-check.sh

docker:
	docker build -t taskboard .

migrate:
	go run ./apps/server/cmd/taskboard migrate up
