# Kishtin

Kishtin, from *cistin*, Irish for kitchen.

A self-hosted kitchen operations system for one restaurant. It runs on a small machine in the restaurant, keeps working with the internet down, and stores everything in one SQLite file the restaurant owns.

See [AGENTS.md](AGENTS.md) for the full plan. This README describes what is built today.

## What works now

Roadmap stage 1: the HACCP log, ported from the browser prototype.

- **Kitchen screens**: Today, Log check, Records and Setup, for phones and tablets, served by `kishtind` itself. See [docs/screens.md](docs/screens.md).
- **HACCP log**: fridge, freezer, delivery, cook, reheat, cooling, hot holding, batch label and cleaning records, checked against FSAI limits. See [docs/haccp.md](docs/haccp.md).
- **Versioned limits**: a manager can set limits per dish, food category or fridge. Every change is a new version, and every record stores the version it was checked against.
- **Customisable guidance and rules**: the reference guidance limits are measured against (starts as FSAI), whether borderline readings need a corrective action, managed lists of fridges, food categories and cleaning tasks, and system settings such as the backup schedule. All versioned, previewed before saving, and audited. See [docs/settings.md](docs/settings.md).
- **Tamper evidence**: records are append-only and hash-chained. Docket photos are stored under their SHA-256. `kishtin haccp verify` re-checks all of it.
- **Today**: every fridge and freezer check (done, due or missed, with check times per unit), the cleaning checklist, checks done out of due, out-of-limit readings and their actions, and batches in use.
- **Live sync**: every open screen shows new records straight away.
- **Exports**: CSV of records and the limit history, and a PDF inspection pack, each with record IDs and hashes.
- **Backups** on a schedule. Each copy is verified and gets a `.sha256` file.
- **Staff login**: PIN on shared kitchen devices, password for manager actions, API keys for the CLI and scripts.
- **HTTP API** (OpenAPI spec in [api/openapi.json](api/openapi.json)) and the **`kishtin` CLI**, which uses the API.

## What does not exist yet

- Menu, stations, orders, the pass view, the scheduler and voice control (roadmap stages 2–5).
- Postgres and DuckDB backends. Only SQLite is built.
- Off-site backups. Kishtin writes backups on the server machine. Copying them somewhere else is up to you.
- Saving while the server is unreachable. Screens keep showing the last data with a "Not connected" notice, but new records cannot be saved until the connection is back.
- Printable batch labels.

## Install and run

You need Go 1.27 or later, Bun 1.4.4 and git (the kit-ui component library is a git submodule).

```sh
git submodule update --init       # fetches frontend/vendor/kit-ui
make build                         # builds the screens, then bin/kishtind and bin/kishtin

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
bin/kishtin settings show guidance
```

Every command takes `--json` for scripts and agents. Run `bin/kishtin --help` for the rest.

To use the screens on a kitchen tablet, add staff and make a device key, then open the pairing link it prints on the tablet:

```sh
bin/kishtin staff add --name "Liam Cook" --pin 1234
bin/kishtin keys create --kind device --name "Pass tablet"
```

The server only listens on the machine itself by default; see Configuration to serve tablets over Wi-Fi.

### Configuration

Data lives in `~/.kishtin/` by default (or `/var/lib/kishtin/`). Change it with `KISHTIN_DATA_DIR` or a `kishtin.toml`. See [kishtin.example.toml](kishtin.example.toml).

The server only listens on loopback unless you configure TLS, or explicitly mark the network as trusted. Every API request needs an API key.

## Development

```sh
make dev        # server on :8080 plus the Vite dev server with live reload on :5173
make frontend-test # svelte-check, frontend unit tests and kit-ui-check
make test       # go test ./... (pure-Go SQLite)
make test-cgo   # the same suite on the CGO SQLite driver
make lint       # golangci-lint
make generate   # sqlc queries, api/openapi.json, the CLI's API client and the screens' API types
```

Tests, fixtures and docs use synthetic data only.
