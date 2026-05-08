# syntax=docker/dockerfile:1.7
#
# Multi-stage build for the tolato server image. Build context is the repo
# root so we can reach both `web/` and `server/` from here.

# ---------- stage: build web ----------
# Pin to BUILDPLATFORM: the SPA bundle is arch-independent, so building once
# on the native runner and copying into both target arches avoids QEMU.
FROM --platform=$BUILDPLATFORM node:22-alpine AS web-builder
WORKDIR /web

# corepack is bundled with node; enable it so `pnpm` resolves without a
# separate global install step. Pinned to node:22 because corepack's default
# pnpm (currently 11.x) requires Node ≥22.13 — node:20 broke the build with
# `ERR_UNKNOWN_BUILTIN_MODULE: node:sqlite`.
RUN corepack enable

COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/ ./

# Forward the VERSION arg to vite so the SPA can display it (sidebar footer
# links to the matching GitHub release).
ARG VERSION=dev
ENV VERSION=${VERSION}
RUN pnpm build

# ---------- stage: build server ----------
# Pin to BUILDPLATFORM and cross-compile via GOARCH=$TARGETARCH. Without this,
# buildx runs the Go compiler under QEMU on the foreign arch (e.g. arm64),
# which is 5-15x slower than native cross-compilation.
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS server-builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./

# Drop the freshly-built SPA into the webui package so //go:embed captures
# the real assets at compile time.
COPY --from=web-builder /web/dist ./internal/webui/dist

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/tolato-server \
    ./cmd/server

# ---------- stage: runtime ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata su-exec \
 && adduser -D -u 1000 tolato

WORKDIR /app
COPY --from=server-builder /out/tolato-server /usr/local/bin/tolato-server
COPY server/config.yaml /app/config.yaml
COPY server/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

# Pre-create the runtime data dir (geoip mmdb cache lives here by default)
# and hand it to the unprivileged user. The entrypoint will additionally
# re-chown on every start so bind-mounted host dirs work without manual setup.
RUN mkdir -p /app/data/geoip \
 && chown -R tolato:tolato /app/data \
 && chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 8080

# Start as root so the entrypoint can chown the data dir, then drop to
# tolato (UID 1000) via su-exec.
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["-config", "/app/config.yaml"]
