# ToLaTo

ToLaTo is an AI-assisted multi-node VPS operations console. This repository currently contains the MVP backend skeleton for:

- `tolato-server`: control plane with HTTP and WebSocket endpoints
- `tolato-agent`: node-side agent runtime skeleton

## Quick Start

1. Start local dependencies:

```bash
make infra-up
```

2. Initialize the database schema:

```bash
make db-migrate
```

If you already started Postgres before pulling schema changes, run `make db-migrate` again to apply them to the existing local database.

3. Install Go dependencies:

```bash
make tidy
```

4. Build binaries:

```bash
make build
```

5. Run the server:

```bash
make run-server-local
```

6. Run the agent:

```bash
make run-agent-local
```

7. Run the web app:

```bash
make web-install
make run-web
```

## Repository Layout

- `cmd/`: binary entrypoints
- `internal/server/`: control plane code
- `internal/agent/`: node agent code
- `internal/shared/`: shared protocol, config, types, and action metadata
- `api/`: OpenAPI and JSON schema contracts
- `db/migrations/`: initial database migrations
- `deployments/`: Dockerfiles and systemd units
- `docs/`: product and architecture documents

## Local Development Defaults

- Server config: `configs/server.local.yaml`
- Agent config: `configs/agent.local.yaml`
- Web env: `web/.env.local`
- Backend API: `http://localhost:8080`
- Web dev server: `http://localhost:5173`
- Admin credentials: `admin` / `admin`
