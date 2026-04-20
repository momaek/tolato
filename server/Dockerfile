# syntax=docker/dockerfile:1.7
#
# Multi-stage build for the tolato server image. Build context is the repo
# root so we can reach both `web/` and `server/` from here.

# ---------- stage: build web ----------
FROM node:20-alpine AS web-builder
WORKDIR /web

# corepack is bundled with node 20; enable it so `pnpm` resolves without a
# separate global install step.
RUN corepack enable

COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

# ---------- stage: build server ----------
FROM golang:1.23-alpine AS server-builder
WORKDIR /src

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./

# Drop the freshly-built SPA into the webui package so //go:embed captures
# the real assets at compile time.
COPY --from=web-builder /web/dist ./internal/webui/dist

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/tolato-server \
    ./cmd/server

# ---------- stage: runtime ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 1000 tolato

WORKDIR /app
COPY --from=server-builder /out/tolato-server /usr/local/bin/tolato-server
COPY server/config.yaml /app/config.yaml

USER tolato
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/tolato-server"]
CMD ["-config", "/app/config.yaml"]
