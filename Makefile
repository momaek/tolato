APP_NAME := tolato
GO ?= go

.PHONY: build run-server run-agent fmt lint tidy

build:
	$(GO) build -o bin/tolato-server ./cmd/tolato-server
	$(GO) build -o bin/tolato-agent ./cmd/tolato-agent

run-server:
	$(GO) run ./cmd/tolato-server --config configs/server.example.yaml

run-agent:
	$(GO) run ./cmd/tolato-agent --config configs/agent.example.yaml

fmt:
	gofmt -w $(shell find . -type f -name '*.go' -not -path './vendor/*')

lint:
	@echo "lint target is reserved for future integration"

tidy:
	$(GO) mod tidy
