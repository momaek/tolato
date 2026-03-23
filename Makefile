SHELL := /bin/bash

GO ?= go
PNPM ?= pnpm
DOCKER_COMPOSE ?= docker compose

.PHONY: help tidy postgres postgres-down db-psql backend backend-memory backend-pg dev test web

help:
	@echo "Available targets:"
	@echo "  make postgres        # Start local PostgreSQL and apply migrations"
	@echo "  make postgres-down   # Stop local PostgreSQL container"
	@echo "  make db-psql         # Open a psql shell to the local Tolato database"
	@echo "  make backend         # Run backend with the default local config (PostgreSQL)"
	@echo "  make backend-pg      # Same as backend"
	@echo "  make backend-memory  # Run backend with the memory config"
	@echo "  make dev             # Bootstrap PostgreSQL and run backend"
	@echo "  make tidy            # Run go mod tidy"
	@echo "  make test            # Run go test ./..."
	@echo "  make web             # Start the web dev server"

tidy:
	$(GO) mod tidy

postgres:
	./scripts/setup-local-postgres.sh

postgres-down:
	$(DOCKER_COMPOSE) -f compose.yaml stop postgres

db-psql:
	docker exec -it tolato-postgres-1 psql -U tolato -d tolato_control_plane

backend:
	$(GO) run ./cmd/tolato-server -config configs/server.local.yaml

backend-pg: backend

backend-memory:
	$(GO) run ./cmd/tolato-server -config configs/server.memory.local.yaml

dev:
	./scripts/run-local-backend.sh

test:
	$(GO) test ./...

web:
	cd web && $(PNPM) dev
