# Kishtin

Kishtin, from *cistin*, Irish for kitchen.

A self-hosted kitchen operations system for one restaurant. It runs on a small machine in the restaurant, keeps working with the internet down, and stores everything in one SQLite file the restaurant owns.

See [AGENTS.md](AGENTS.md) for the full plan. This README describes what is built today.

## What works now

Roadmap stages 1 to 7: the HACCP log, ported from the browser prototype; the menu with stations and equipment; orders and the pass; the cook scheduler; voice control; cook times learned from real cooks; and orders from Square (and SumUp).

- **Screens** for a computer, served by `kishtind` itself: the Pass, Stations and Orders; HACCP Today, Records, Record books, Exports and Verify; and Setup for the menu, the scheduler, fridges and tasks, limits and rules, HACCP records, staff, device keys, backups and settings. Search with Ctrl K. Each device can instead show a full-screen pass screen, station screen or phone layout, with large type and 56 px buttons. See [docs/screens.md](docs/screens.md).
- **Menu**: stations, equipment with how many units it holds, and dishes with cook, rest, plate and hold times, batch capacity, modifiers such as "well done +2 min", and 86 for sold out. See [docs/kitchen.md](docs/kitchen.md).
- **Orders and the pass**: tables and takeaways with daily ticket numbers and an optional name (such as the guest's) shown as the heading; courses held until fired, and held again if nothing has started; items moved from waiting to cooking, resting, plating, ready and served; items changed after ordering (voided and replaced in one step); a course served together; expo overrides (new target time, urgent) logged with every other change in each order's history.
- **Voice control**: push-to-talk on kitchen screens for firing, holding, starting, readying, voiding, 86ing and asking what is next, and for fridge, freezer, cooking, delivery, cooling, batch label and cleaning records, asking for any detail not said. Optional hands-free listening for "Kishtin", off by default on every device. Speech is recognised by a local whisper.cpp sidecar and replies spoken by a local Piper sidecar; no audio leaves the building. It asks when unsure, needs "confirm" for a HACCP fail, and logs every command. See [docs/voice.md](docs/voice.md).
- **Connections**: Setup → Connections lists what Kishtin can connect to, what each sends, and its state. Square and SumUp: a manager connects each there with a token, checked with the till company and kept outside the database.
- **Orders from Square**: orders taken on the Square till come into the kitchen on their own, deduped on Square's order ID and version. Till items are mapped to dishes once; drinks are marked not for the kitchen. Changes on the till that would remove food are flagged to the pass, never voided automatically. Kishtin only reads from Square, never sees card data, and keeps working when Square is unreachable. See [docs/square.md](docs/square.md).
- **Sales from SumUp**: sales paid on SumUp come into the kitchen as takeaways, mapped to dishes by item name. SumUp reports a sale only once it is paid, so this suits counter service and takeaways. Refunds are flagged to the pass, never voided automatically. See [docs/sumup.md](docs/sumup.md).
- **Station tickets**: each station sees one ticket per order and course, with a button per item, a timer and status colour, the scheduler's reason one tap away, all-day counts per dish (with "start these together"), and recently done items to undo. The pass has all-day counts too.
- **Cook scheduler**: backward scheduling within equipment capacity, with batching across tables, priorities and station buffers. It tells each station what to start and when, says why for every recommendation, and shows each course as on track, at risk or late. Its settings are versioned.
- **HACCP log**: fridge, freezer, delivery, cook, reheat, cooling, hot holding, batch label and cleaning records, checked against FSAI limits. See [docs/haccp.md](docs/haccp.md).
- **Versioned limits**: a manager can set limits per dish, food category or fridge. Every change is a new version, and every record stores the version it was checked against.
- **Customisable guidance and rules**: the reference guidance limits are measured against (starts as FSAI), whether borderline readings need a corrective action, managed lists of fridges, food categories and cleaning tasks, and system settings such as the backup schedule. All versioned, previewed before saving, and audited. See [docs/settings.md](docs/settings.md).
- **Tamper evidence**: records are append-only and hash-chained. Docket photos are stored under their SHA-256. `kishtin haccp verify` re-checks all of it.
- **Today**: every fridge and freezer check (done, due or missed, with check times per unit), the cleaning checklist, checks done out of due, out-of-limit readings and their actions, and batches in use.
- **Live sync**: every open screen shows new records straight away.
- **Exports**: CSV of records and the limit history, and a PDF inspection pack, each with record IDs and hashes.
- **Record books**: official HACCP booklets filled in from the records for a month, with missed checks marked. Bundles per country (Ireland's FSAI Safe Catering booklets SC1 to SC9, and a general set for any country), which a manager can change in Setup › Food safety › HACCP records. A manager signs off each period, which fills the booklets' manager check line. Kishtin compares your copy of a booklet with its template, checks the publisher for new editions when asked, and brings in old paper pages from a photo. An optional local model can suggest columns and read photos. See [docs/record-books.md](docs/record-books.md).
- **Backups** on a schedule. Each copy is verified and gets a `.sha256` file.
- **Checks logged later**: any check can be logged with the time it was done and who did it, and **Catch up** lists the daily checks with no record so they can be filled in together. See [docs/haccp.md](docs/haccp.md#checks-logged-later).
- **Staff login**: PIN on shared kitchen devices, password for manager actions, API keys for the CLI and scripts. A forgotten manager password can be reset with `kishtind reset-password` on the Kishtin computer itself.
- **Starts with the computer**: the installer offers it, and it can be turned on in Settings or with `kishtind startup on`. It uses Task Scheduler on Windows, a LaunchAgent on macOS and a systemd user service on Linux. See [docs/install.md](docs/install.md#start-with-the-computer).
- **Learned cook times**: Kishtin learns each dish's real cook time from the station screens and, when a manager turns it on, new orders use it once a dish has 10 cooks. Setup › Kitchen › Menu shows set and learned times side by side, with a reset per dish. See [docs/kitchen.md](docs/kitchen.md#learned-cook-times).
- **Reports**: pass times (share on time, fire to pass, waiting at the pass, why courses were late, dish times against set times) and HACCP checks (checks and cleaning done and missed, out-of-limit readings, each fridge's temperatures). See [docs/reports.md](docs/reports.md).
- **Recording**: scheduler plans as they change, with why each course ran late; an activity log of sold-out dishes, logins, screens dropping off and how each HACCP record was entered; the device behind each action; every version of each till order; optional voice recordings. Leavers can be anonymised. See [docs/recording.md](docs/recording.md).
- **Permissions**: a manager can give any role or named member of staff jobs such as signing off records, and choose whether a PIN is enough. See [docs/permissions.md](docs/permissions.md).
- **HTTP API** (OpenAPI spec in [api/openapi.json](api/openapi.json)) and the **`kishtin` CLI**, which uses the API.

## What does not exist yet

- Ticket progress written back to Square, and menu sync from the Square catalog.
- Scheduler service profiles, replaying a service under new settings, and a fixed pace between courses.
- Postgres and DuckDB backends. Only SQLite is built.
- Off-site backups. Kishtin writes backups on the server machine. Copying them somewhere else is up to you.
- Saving while the server is unreachable. Screens keep showing the last data with a "Not connected" notice, but new records cannot be saved until the connection is back.
- Printable batch labels.

## Install and run

Setting Kishtin up in a restaurant, and not technical? Start with **[Getting started](docs/getting-started.md)**: a step-by-step guide that also explains how Kishtin differs from cloud systems.

On Windows, in PowerShell:

```powershell
irm https://github.com/cathalbyrnegit/kishtin/releases/latest/download/install.ps1 | iex
```

On macOS or Linux, in Terminal:

```sh
curl -fsSL https://github.com/cathalbyrnegit/kishtin/releases/latest/download/install.sh | sh
```

The installer offers to start Kishtin with the computer, and starts it. Otherwise run `kishtind`. Then open `http://127.0.0.1:8080/` in a browser on the same computer to set it up. The installer checks the download against the release's `SHA256SUMS` first. See [docs/install.md](docs/install.md) for updates, other options, and what it does not do yet.

### Build from source

You need Go 1.27 or later, Bun 1.4.4 and git (the kit-ui component library is a git submodule).

```sh
git submodule update --init       # fetches frontend/vendor/kit-ui
make build                         # builds the screens, then bin/kishtind and bin/kishtin

bin/kishtind                       # serves on 127.0.0.1:8080
```

Open `http://127.0.0.1:8080/` in a browser on the same computer. A new install shows **Set up Kishtin**: name the restaurant and make the manager's account. The last step shows a key for the `kishtin` command line once. See [Setting Kishtin up](docs/screens.md#setting-kishtin-up).

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

The server only listens on the machine itself by default; see Configuration to serve other devices over Wi-Fi.

### Configuration

Data lives in `~/.kishtin/` by default (or `/var/lib/kishtin/`). Change it with `KISHTIN_DATA_DIR` or a `kishtin.toml`. See [kishtin.example.toml](kishtin.example.toml).

Out of the box, Kishtin serves the Kishtin computer over plain HTTP on port 8080, and tablets on the restaurant's network over HTTPS on port 8443 with its own certificate, paired by QR code (see [docs/screens.md](docs/screens.md#connecting-a-tablet-or-phone)). Other devices get nothing over plain HTTP but the page that helps them install that certificate. Set `tablets = false` under `[server]` to keep Kishtin on this computer only. To serve plain HTTP more widely, configure your own TLS certificate, or explicitly mark the network as trusted. Every API request needs an API key.

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

## Licence

Kishtin is licensed under the [Apache License, Version 2.0](LICENSE). See [NOTICE](NOTICE). It includes open-source software under its own licences (MIT, BSD and Apache 2.0); `kishtind licenses` prints them all, and they are kept in [internal/legal/licenses.txt](internal/legal/licenses.txt).
