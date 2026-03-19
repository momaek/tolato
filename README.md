# ToLaTo

ToLaTo is an AI-assisted multi-node VPS operations console. This repository currently contains the MVP backend skeleton for:

- `tolato-server`: control plane with HTTP and WebSocket endpoints
- `tolato-agent`: node-side agent runtime skeleton

## Quick Start

1. Start local dependencies:

```bash
docker-compose up -d
```

2. Install Go dependencies:

```bash
make tidy
```

3. Build binaries:

```bash
make build
```

4. Run the server:

```bash
make run-server
```

5. Run the agent:

```bash
make run-agent
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
