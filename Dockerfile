FROM node:24-alpine AS web-build
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/package.json
RUN corepack enable && pnpm install --frozen-lockfile --ignore-scripts && pnpm rebuild esbuild
COPY apps/web apps/web
RUN pnpm --filter @slopstack/web build

FROM golang:1.26-alpine AS server-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY apps/server apps/server
RUN CGO_ENABLED=0 go build -o /slopstack ./apps/server/cmd/slopstack

FROM alpine:3.22
RUN addgroup -S slopstack && adduser -S slopstack -G slopstack
WORKDIR /app
COPY --from=server-build /slopstack /app/slopstack
COPY --from=web-build /src/apps/web/dist /app/public
RUN chown -R slopstack:slopstack /app
USER slopstack
VOLUME ["/app/pb_data"]
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8090/api/health || exit 1
CMD ["/app/slopstack", "serve", "--http=0.0.0.0:8090"]
