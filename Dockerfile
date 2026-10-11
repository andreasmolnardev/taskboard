FROM node:22.23.2-alpine3.24 AS web-build
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/package.json
RUN corepack enable && pnpm install --frozen-lockfile --ignore-scripts && pnpm rebuild esbuild
COPY slopstack.yaml slopstack.yaml
COPY scripts scripts
COPY apps/web apps/web
RUN pnpm --filter @taskboard/web build

FROM golang:1.27.1-alpine3.24 AS server-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY apps/server apps/server
RUN CGO_ENABLED=0 go build -o /taskboard ./apps/server/cmd/taskboard

FROM alpine:3.24.2
RUN addgroup -S taskboard && adduser -S taskboard -G taskboard
WORKDIR /app
COPY --from=server-build /taskboard /app/taskboard
COPY --from=web-build /src/apps/web/dist /app/public
RUN chown -R taskboard:taskboard /app
USER taskboard
VOLUME ["/app/pb_data"]
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8090/api/health || exit 1
CMD ["/app/taskboard", "serve", "--http=0.0.0.0:8090"]
