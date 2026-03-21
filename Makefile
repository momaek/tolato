APP_NAME := tolato
GO ?= go

.PHONY: build run-server run-agent run-server-local run-agent-local fmt lint tidy infra-up infra-down db-migrate web-install run-web

build:
	$(GO) build -o bin/tolato-server ./cmd/tolato-server
	$(GO) build -o bin/tolato-agent ./cmd/tolato-agent

run-server:
	$(GO) run ./cmd/tolato-server --config configs/server.example.yaml

run-agent:
	$(GO) run ./cmd/tolato-agent --config configs/agent.example.yaml

run-server-local:
	$(GO) run ./cmd/tolato-server --config configs/server.local.yaml

run-agent-local:
	$(GO) run ./cmd/tolato-agent --config configs/agent.local.yaml

fmt:
	gofmt -w $(shell find . -type f -name '*.go' -not -path './vendor/*')

lint:
	@echo "lint target is reserved for future integration"

tidy:
	$(GO) mod tidy

infra-up:
	docker compose up -d
	@echo "waiting for postgres to become healthy..."
	@until [ "$$(docker inspect -f '{{.State.Health.Status}}' $$(docker compose ps -q postgres) 2>/dev/null)" = "healthy" ]; do sleep 2; done
	@echo "waiting for redis to become healthy..."
	@until [ "$$(docker inspect -f '{{.State.Health.Status}}' $$(docker compose ps -q redis) 2>/dev/null)" = "healthy" ]; do sleep 2; done

infra-down:
	docker compose down

db-migrate:
	docker compose exec -T postgres psql -U tolato -d tolato < db/migrations/000001_init.up.sql

web-install:
	cd web && pnpm install

run-web:
	cd web && pnpm dev
