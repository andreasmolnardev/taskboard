.PHONY: dev build test lint docker migrate generate

dev:
	pnpm dev

build:
	pnpm build
	go build ./apps/server/cmd/slopstack

test:
	pnpm test
	go test ./...

lint:
	pnpm lint
	gofmt -d apps/server

generate:
	pnpm generate:api
	pnpm generate:client

docker:
	docker build -t slopstack .

migrate:
	go run ./apps/server/cmd/slopstack migrate up
