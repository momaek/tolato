# ToLaTo

ToLaTo is an AI-assisted multi-node VPS operations console. This repository currently contains the in-progress control-plane backend, frontend mock app, and supporting product/architecture docs.

## Quick Start

1. Install Go dependencies:

```bash
go mod tidy
```

2. Prepare local PostgreSQL:

```bash
./scripts/setup-local-postgres.sh
```

3. Run the backend server:

```bash
go run ./cmd/tolato-server -config configs/server.local.yaml
```

Or use the one-shot helper:

```bash
./scripts/run-local-backend.sh
```

4. Verify the server is up:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/api/v1/nodes
curl http://127.0.0.1:8080/api/v1/history/tasks
curl http://127.0.0.1:8080/api/v1/settings/preferences
```

5. Run the web app:

```bash
cd web
pnpm install
pnpm dev
```

## Current Scope

- `tolato-server` now provides a minimal runnable HTTP service with:
  - `GET /healthz`
  - `GET /api/v1/nodes`
  - `GET /api/v1/nodes/:id`
  - `GET /api/v1/history/tasks`
  - `GET /api/v1/history/tasks/:id`
  - `GET/PUT /api/v1/settings/model-config`
  - `GET/PUT /api/v1/settings/account-security`
  - `POST /api/v1/settings/model-config/test`
  - `POST /api/v1/settings/password/change`
  - `POST /api/v1/settings/sessions/revoke-others`
  - `GET/PUT /api/v1/settings/preferences`
- `tolato-server` now also exposes `GET /ws/ui` and `GET /ws/agent`
- development console flow is wired with a seeded idle session, scripted LLM loop, and fallback local execution when no real agent is connected
- `Nodes` HTTP currently uses a static development node source
- `History` HTTP currently reads from the configured store and still lacks full task detail projection
- `Settings` HTTP currently reads from the configured store; password change and revoke-other-sessions are development-mode placeholders
- frontend now defaults to the real backend routes; set `VITE_USE_MOCK=true` only when you explicitly want the mock app behavior

## Repository Layout

- `cmd/`: binary entrypoints
- `configs/`: local config files
- `internal/server/`: control plane code
- `db/migrations/`: initial database migrations
- `docs/`: product, UI, and architecture documents
- `web/`: frontend app and mock adapters

## Local Development Defaults

- Server config: `configs/server.local.yaml`
- Memory-only fallback config: `configs/server.memory.local.yaml`
- Local PostgreSQL bootstrap: `compose.yaml` + `scripts/setup-local-postgres.sh`
- Default PostgreSQL DSN: `postgres://tolato:tolato@127.0.0.1:5432/tolato_control_plane?sslmode=disable`
- Backend API: `http://localhost:8080`
- Web dev server: `http://localhost:5173`
- Admin credentials: `admin` / `admin`
