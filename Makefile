GO ?= go
OAPI_CODEGEN ?= $(GO) run github.com/doordash-oss/oapi-codegen-dd/v3/cmd/oapi-codegen@v3.76.3
SQLC ?= sqlc

.PHONY: build test test-cgo lint generate migrate dev frontend frontend-test test-pg test-duckdb

build:
	$(GO) build -o bin/ ./cmd/...

test:
	$(GO) test ./...

# The conformance suite must pass on both SQLite drivers.
test-cgo:
	CGO_ENABLED=1 $(GO) test -tags sqlite_cgo ./...

lint:
	golangci-lint run

# Regenerate sqlc queries, the OpenAPI spec and the CLI's API client.
generate:
	$(SQLC) generate
	$(GO) run ./cmd/kishtind openapi > api/openapi.json
	$(OAPI_CODEGEN) --config internal/client/cfg.yaml api/openapi.json

# Migrations run when kishtind starts; this applies them without serving.
migrate:
	$(GO) run ./cmd/kishtind migrate

dev:
	$(GO) run ./cmd/kishtind

frontend frontend-test:
	@echo "The Svelte frontend is not built yet (roadmap stage 1, next step)." >&2; exit 1

test-pg test-duckdb:
	@echo "The Postgres and DuckDB backends are not built yet." >&2; exit 1
