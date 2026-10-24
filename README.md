# Kishtin

Kishtin, from *cistin*, Irish for kitchen.

A self-hosted kitchen operations system for one restaurant. It runs on a small machine in the restaurant, keeps working with the internet down, and stores everything in one SQLite file the restaurant owns.

See [AGENTS.md](AGENTS.md) for the full plan. This README describes what is built today.

## What works now

Roadmap stage 1, backend part:

- **HACCP log**: fridge, freezer, delivery, cook, reheat, cooling, hot holding, batch label and cleaning records, checked against FSAI limits. See [docs/haccp.md](docs/haccp.md).
- **Versioned limits**: a manager can set limits per dish, food category or fridge. Every change is a new version, and every record stores the version it was checked against.
- **Tamper evidence**: records are append-only and hash-chained. Docket photos are stored under their SHA-256. `kishtin haccp verify` re-checks all of it.
- **CSV export** of records and the limit history, for inspections.
- **Backups** on a schedule. Each copy is verified and gets a `.sha256` file.
- **Staff login**: PIN on shared kitchen devices, password for manager actions, API keys for the CLI and scripts.
- **HTTP API** (OpenAPI spec in [api/openapi.json](api/openapi.json)) and the **`kishtin` CLI**, which uses the API.

## What does not exist yet

- The kitchen screens (Svelte UI). Open `http://127.0.0.1:8080/` and you get a page saying so. For now, use the CLI or the API.
- Menu, stations, orders, the pass view, the scheduler and voice control (roadmap stages 2–5).
- Postgres and DuckDB backends. Only SQLite is built.
- Off-site backups. Kishtin writes backups on the server machine. Copying them somewhere else is up to you.

## Install and run

You need Go 1.27 or later.

```sh
make build                         # builds bin/kishtind and bin/kishtin

# First time only, with the server stopped: create the first manager.
bin/kishtind setup --name "Your Name"
# It asks for a password and prints an API key. Keep the key safe.

bin/kishtind                       # serves on 127.0.0.1:8080
```

In another terminal:

```sh
export KISHTIN_API_KEY=kishtin_key_...   # the key from setup
bin/kishtin whoami
bin/kishtin haccp log fridge --unit "Walk-in" --temp 3.4
bin/kishtin haccp list
bin/kishtin haccp verify
bin/kishtin haccp export --from 2026-05-01 --to 2026-05-31 -o may.csv
```

Every command takes `--json` for scripts and agents. Run `bin/kishtin --help` for the rest.

### Configuration

Data lives in `~/.kishtin/` by default (or `/var/lib/kishtin/`). Change it with `KISHTIN_DATA_DIR` or a `kishtin.toml`. See [kishtin.example.toml](kishtin.example.toml).

The server only listens on loopback unless you configure TLS, or explicitly mark the network as trusted. Every API request needs an API key.

## Development

```sh
make test       # go test ./... (pure-Go SQLite)
make test-cgo   # the same suite on the CGO SQLite driver
make lint       # golangci-lint
make generate   # sqlc queries, api/openapi.json, and the CLI's API client
```

Tests, fixtures and docs use synthetic data only.
