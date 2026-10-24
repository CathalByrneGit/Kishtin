GO ?= go
BUN ?= bun
OAPI_CODEGEN ?= $(GO) run github.com/doordash-oss/oapi-codegen-dd/v3/cmd/oapi-codegen@v3.76.3
SQLC ?= sqlc

.PHONY: build test test-cgo lint generate migrate dev frontend frontend-deps frontend-test test-pg test-duckdb

# The Go build embeds frontend/dist, so build the screens first.
build: frontend
	$(GO) build -o bin/ ./cmd/...

frontend-deps:
	git submodule update --init frontend/vendor/kit-ui
	cd frontend && $(BUN) install --frozen-lockfile

frontend: frontend-deps
	cd frontend && $(BUN) run build

frontend-test: frontend-deps
	cd frontend && $(BUN) run check && $(BUN) run test && $(BUN) run kit-ui-check

test: frontend
	$(GO) test ./...

# The conformance suite must pass on both SQLite drivers.
test-cgo: frontend
	CGO_ENABLED=1 $(GO) test -tags sqlite_cgo ./...

lint: frontend
	golangci-lint run

# Regenerate sqlc queries, the OpenAPI spec, the CLI's API client and the
# frontend's API types.
generate:
	$(SQLC) generate
	$(GO) run ./cmd/kishtind openapi > api/openapi.json
	$(OAPI_CODEGEN) --config internal/client/cfg.yaml api/openapi.json
	cd frontend && $(BUN) run generate:api

# Migrations run when kishtind starts; this applies them without serving.
migrate:
	$(GO) run ./cmd/kishtind migrate

# Runs the server on :8080 and the Vite dev server with live reload on
# :5173, which proxies /api to the server.
dev: frontend
	$(GO) run ./cmd/kishtind & (cd frontend && $(BUN) run dev); kill %1

test-pg test-duckdb:
	@echo "The Postgres and DuckDB backends are not built yet." >&2; exit 1
