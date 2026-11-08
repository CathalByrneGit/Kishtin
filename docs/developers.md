---
title: Building and developing Kishtin
description: Build Kishtin from source, use the command line, configure the server, and work on the code and these docs.
---

# Building and developing Kishtin

For developers and anyone running Kishtin from source. Restaurant owners installing a release want [Installing Kishtin](install.md) instead. The design rules for contributors and coding agents are in [AGENTS.md](https://github.com/CathalByrneGit/Kishtin/blob/main/AGENTS.md).

## Build from source

You need Go 1.27 or later, Bun 1.4.4 and git (the kit-ui component library is a git submodule).

```sh
git submodule update --init       # fetches frontend/vendor/kit-ui
make build                         # builds the screens, then bin/kishtind and bin/kishtin

bin/kishtind                       # serves on 127.0.0.1:8080
```

Open `http://127.0.0.1:8080/` in a browser on the same computer. A new install shows **Set up Kishtin**: name the restaurant and make the manager's account. The last step shows a key for the `kishtin` command line once. See [Setting Kishtin up](screens.md#setting-kishtin-up).

To do it from the command line instead, run `bin/kishtind setup --name "Your Name"` with the server stopped. It asks for a password and prints an API key.

To use the command line, in another terminal:

```sh
export KISHTIN_API_KEY=kishtin_key_...   # the key shown at setup
bin/kishtin whoami
bin/kishtin haccp log fridge --unit "Walk-in" --temp 3.4
bin/kishtin haccp list
bin/kishtin haccp verify
bin/kishtin haccp export --from 2026-05-01 --to 2026-05-31 -o may.csv
bin/kishtin settings show guidance
bin/kishtin menu
bin/kishtin order new --table 4 --item "2x Burger: well done" --fire main
bin/kishtin pass --why
```

Every command takes `--json` for scripts and agents. Run `bin/kishtin --help` for the rest.

To use the screens, add staff and make a device key, then open the pairing link it prints in the browser you will use. After that, staff, keys and limits can also be managed from the screens.

```sh
bin/kishtin staff add --name "Liam Cook" --pin 1234
bin/kishtin keys create --kind device --name "Office PC"
```

The server only listens on the machine itself by default; see [Configuration](#configuration) to serve other devices over Wi-Fi.

## Configuration

Data lives in `~/.kishtin/` by default (or `/var/lib/kishtin/`). Change it with `KISHTIN_DATA_DIR` or a `kishtin.toml`. See [kishtin.example.toml](https://github.com/CathalByrneGit/Kishtin/blob/main/kishtin.example.toml).

Out of the box, Kishtin serves the Kishtin computer over plain HTTP on port 8080, and tablets on the restaurant's network over HTTPS on port 8443 with its own certificate, paired by QR code (see [Connecting a tablet or phone](screens.md#connecting-a-tablet-or-phone)). Other devices get nothing over plain HTTP but the page that helps them install that certificate. Set `tablets = false` under `[server]` to keep Kishtin on this computer only. To serve plain HTTP more widely, configure your own TLS certificate, or explicitly mark the network as trusted. Every API request needs an API key.

## Development

```sh
make dev        # server on :8080 plus the Vite dev server with live reload on :5173
make frontend-test # svelte-check, frontend unit tests and kit-ui-check
make test       # go test ./... (pure-Go SQLite)
make test-cgo   # the same suite on the CGO SQLite driver
make lint       # golangci-lint
make generate   # sqlc queries, api/openapi.json, the CLI's API client and the screens' API types
make docs       # the documentation site from docs/, into website/site/ (needs uv)
make docs-serve # the same, served with live reload on :8000
```

The documentation is published at **https://cathalbyrnegit.github.io/Kishtin/**, built from `docs/` with [Zensical](https://zensical.org) by `.github/workflows/docs.yml` on every push to `main`. Add a new page to `nav` in `zensical.toml`. Links to files outside `docs/` go to their GitHub address, so they work on the site too.

Tests, fixtures and docs use synthetic data only.
