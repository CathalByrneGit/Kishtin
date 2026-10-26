# AGENTS.md

Guidance for AI coding agents and human contributors working on this repository.

## What this project is

**Kishtin** (from *cistin*, Irish for kitchen; said "KISH-tin") is a self-hosted kitchen operations system for a single restaurant in Ireland, written in Go. It has four parts:

1. **HACCP compliance log**: temperature checks, deliveries, cooking, cooling, hot holding, batch labels, cleaning, and corrective actions, checked against FSAI limits. This replaces an earlier browser prototype and keeps its features.
2. **Pass management (kitchen display)**: every open order and table in one live view. Each dish has a typical cook time, and the head chef or expo can see what is cooking, what is waiting, and what is ready at the pass.
3. **Cook scheduler**: decides what each station should start cooking and when. The goal is that every dish for a table or course reaches the pass at the same time, without going over the capacity of any station (for example "the grill takes 3 burgers at once").

4. **Voice control**: hands-free commands for the pass, stations and HACCP logging, because chefs' hands are usually full or dirty. Speech recognition runs locally.

**Later, not in v1:** orders arriving from the restaurant's **Square** POS, and small local AI models for smarter voice and suggestions (see AI, later).

### Goals

- Runs on a small machine inside the restaurant and keeps working if the internet drops.
- Works on phones and tablets in a hot, busy kitchen: large tap targets, high contrast, readable at arm's length.
- One Go binary plus one database file. Easy to install, back up and update.
- Scheduling decisions are explainable. A chef can always see why the system says "fire 2 burgers now".

### Non-goals for v1

- Multi-site or chain management.
- Handling card numbers or card data directly. Payments stay in Square (see Integrations).
- Stock control, rota or payroll.
- Generative or large AI models. v1 uses deterministic rules, then learns typical times from real cook logs. Voice in v1 uses local speech-to-text plus a fixed command grammar.
- Anything that needs a cloud service to run a service. Kitchen features must work with the internet down.

### Naming

- Product name in UI and docs: **Kishtin**. Tagline: "Kishtin, from *cistin*, Irish for kitchen."
- Binaries: `kishtind` (server daemon) and `kishtin` (CLI). Config file: `kishtin.toml`. Data directory: `~/.kishtin/` or `/var/lib/kishtin/`.
- Go module path: decide once the domain is registered (for example `kishtin.ie/kishtin`); until then use `github.com/<org>/kishtin`.
- Environment variables use the `KISHTIN_` prefix (for example `KISHTIN_SQUARE_TOKEN`).
- Spelled with a capital K in prose and lower case in code and commands.

## Design principles

Inspired by the local-first tools from Kenn Software (kenn.io), especially docbank (docbank.ai). Apply these to every feature.

- **Local first, owned by the restaurant.** Data lives in one SQLite file on the restaurant's own machine. Nothing needs a cloud account to work, and the owner can copy, back up or inspect the file at any time.
- **Nothing leaves without consent.** Anything that would send data outside the building (Square sync, an accounting export, a future optional AI provider) shows what will be sent first and only runs once a person agrees. Local processing is always the default.
- **Show the plan, then act.** Exports, bulk changes, imports and scheduler overrides show what will happen before they happen. The scheduler explains every recommendation.
- **Verifiable records.** HACCP records and stored files can be checked for tampering (see HACCP). Reports and exports carry enough evidence (record IDs, timestamps, checksums) that an inspector or accountant can check the numbers.
- **People and agents use the same records.** Every action available in the UI is also available through a CLI with `--json` output and a local HTTP API, so scripts and AI agents work with exactly the same data and rules as staff. A local MCP server can come later.
- **Honest limits.** The UI and docs state plainly what the system does not do. Never imply a check was done, an order was sent or a record was saved when it wasn't.
- **Plain, direct words.** Short, declarative copy with no marketing language. Errors say what went wrong and what to do next.
- **Bright, clear interface.** A light theme by default: clean, bright and high contrast, so screens feel at home in a bright kitchen and read clearly at arm's length. Minimal chrome, strong type, and status shown with clear colour plus text (never colour alone). No dark default.

## Users and roles

| Role | Main tasks |
|---|---|
| Head chef / expo | Runs the pass, fires courses, overrides the scheduler, closes tickets |
| Line cook | Sees their station's queue, marks items started and done |
| Front of house | Enters orders (until a POS is integrated), sees when food is ready |
| Manager | Menu and station setup, HACCP review and export, users |

Authentication: short staff PIN login on shared kitchen devices, with a role per user. Manager actions need a full password by default.

Permissions are customisable: a manager can give an action to other roles or named staff and choose whether a PIN is enough (versioned `permissions` settings, `internal/auth/permissions.go`). Managers always keep every action. Staff, API keys, permissions and fitness to work stay managers with a password. Service code checks actions with `s.allow`, never by role; screens use the `can` map from `/api/auth/me`.

## Glossary

Use these terms consistently in code, UI text and docs.

- **Order**: what a table or takeaway customer asked for. Contains items, grouped by course.
- **Item**: one dish on an order (for example "1x burger, medium, no onion").
- **Course**: starters, mains, desserts. A course is **held** until the expo **fires** it.
- **Ticket**: the kitchen's view of one order or course.
- **Station**: a cooking area with its own queue (grill, fryer, sauté, cold section, pastry).
- **Equipment**: a capacity-limited resource inside a station (for example "grill: 3 burger slots", "fryer: 2 baskets").
- **Batch capacity**: how many units of a dish one piece of equipment can cook at once.
- **Cook time / rest time / plate time**: the three timed phases of a dish. Rest covers resting meat and similar.
- **Target time**: when every item of a fired course should be at the pass together.
- **Latest start**: target time minus (cook + rest + plate). Starting later makes the course late.
- **Pass**: the counter where finished plates wait for service. **Ready at pass** is the end state of an item.

## Architecture

- **Language:** Go 1.27 or later (matching docbank). Standard library first, then the libraries below.
- **HTTP API:** Huma v2 on top of `net/http`, so every route has a typed request and response and the OpenAPI spec is generated from code. The CLI client is generated from that spec.
- **UI:** a Svelte 5 web app built on Kenn's shared component library, `kit-ui`, the same stack as docbank and the other Kenn apps. It is compiled ahead of time and embedded into the Go binary, so deployment stays one file. See Frontend below.
- **Database:** pluggable storage behind one Go interface. SQLite is the default and the reference backend; Postgres and DuckDB are supported options. See Storage backends below.
- **Deployment:** one binary that serves the UI, the API and the SSE streams, running on a mini-PC or similar on the restaurant's network. Staff devices connect over local Wi-Fi.
- **Time:** store all timestamps in UTC. Display them in `Europe/Dublin`. Inject the clock through a `Clock` interface so tests control time.

### Go libraries

Follow docbank's dependency choices (github.com/kenn-io/docbank) so the stack stays familiar and well-tested. Use these rather than alternatives:

| Need | Library |
|---|---|
| HTTP API with OpenAPI | `github.com/danielgtaylor/huma/v2` |
| Client code from the OpenAPI spec | `github.com/doordash-oss/oapi-codegen-dd/v3` |
| CLI commands and flags | `github.com/spf13/cobra`, `github.com/spf13/pflag` |
| Config file | `github.com/BurntSushi/toml` (`kishtin.toml`) |
| SQLite | `modernc.org/sqlite` (pure Go, default) and `github.com/mattn/go-sqlite3` (CGO), selectable at build time; both must pass the test suite |
| Single-instance lock on the data directory | `github.com/gofrs/flock` |
| Retries (Square polling, sidecars) | `github.com/cenkalti/backoff/v7` |
| IDs | `github.com/google/uuid` |
| File type detection for uploads (invoices, photos) | `github.com/gabriel-vasile/mimetype` |
| PDF reading and processing (supplier invoices) | `github.com/pdfcpu/pdfcpu`; `github.com/klippa-app/go-pdfium` with `github.com/tetratelabs/wazero` for text and page images without CGO |
| PDF output (inspection reports) | `github.com/go-pdf/fpdf` |
| JSON Schema validation (API input, AI extraction output) | `github.com/santhosh-tekuri/jsonschema/v5` |
| Local MCP server for agents (later) | `github.com/modelcontextprotocol/go-sdk` |
| Optional terminal UI for managers | `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2` |
| Test assertions | `github.com/stretchr/testify` |
| Concurrency helpers, crypto, text | `golang.org/x/sync`, `golang.org/x/crypto`, `golang.org/x/text` |

Live screen updates use Server-Sent Events from the standard library. Add `github.com/coder/websocket` only if two-way streaming is needed (for example streaming voice audio). Docbank's S3 and Markdown libraries are not needed in v1.

### Frontend

Follow the Kenn apps (docbank, agentsview, msgvault), which share one frontend stack.

| Need | Choice |
|---|---|
| Framework | Svelte 5 in runes mode, TypeScript in strict mode |
| Components and design tokens | `@kenn-io/kit-ui` (Apache-2.0), consumed as source. Use its components (Button, Chip, Modal, Table, TextInput, Notice, CommandPalette and so on) instead of hand-rolling them; run its `kit-ui-check` in CI |
| Icons | `@lucide/svelte` (a kit-ui peer dependency) |
| Styling | Plain CSS with kit-ui's CSS custom properties. No Tailwind |
| Build | Vite, producing a static single-page app (no SvelteKit server). Package manager and test runner: Bun, as kit-ui uses |
| API client | TypeScript types generated from the server's OpenAPI spec, so the UI, CLI and agents share one contract |
| Live updates | `EventSource` (Server-Sent Events) from the Go server |
| Voice capture | Browser `MediaRecorder` for push-to-talk audio, posted to the server; spoken replies played from the server's local text-to-speech |

**Embedding.** `make frontend` builds into `frontend/dist/`, and the Go server includes it with `//go:embed`. Commit nothing from `dist/`. A Go-only build without Node fails clearly and says to run `make frontend` first.

**Kitchen-specific UI rules** (on top of kit-ui):

- Light theme by default. Use kit-ui's high-contrast mode on kitchen screens; dark mode stays available as a per-device choice, never the default.
- Large tap targets (at least 56 px on kitchen screens), big numbers for timers and temperatures, readable at arm's length.
- Status always shown as text plus colour (on track / at risk / late, pass / fail).
- Separate layouts for the pass (wall-mounted tablet or TV), station screens, phones (HACCP logging), and the manager's desk view, all from one app.
- Screens keep showing the last known state and a clear "Not connected" notice if the server drops; they never show stale data as current.
- Kitchen screens use the Screen Wake Lock API so tablets don't sleep mid-service.

### Storage backends

Core packages never import a database driver. They depend on interfaces in `internal/store` (for example `HACCPStore`, `OrderStore`, `MenuStore`), grouped into a `Store` with a `WithTx(ctx, func(tx Store) error)` method. Each backend implements them, and one shared conformance suite in `internal/store/storetest` runs against every backend.

| Backend | Role | Notes |
|---|---|---|
| **SQLite** (default) | System of record for a single restaurant | One file, nothing to install. Pure-Go `modernc.org/sqlite` by default. WAL mode, `foreign_keys=ON`, `busy_timeout`, one writer connection plus readers. Full-text search with FTS5. Backups with `VACUUM INTO` on a schedule, then copied off the machine; never copy the live file. |
| **Postgres** | Optional system of record | For sites that already run Postgres, or a future multi-site setup. Full-text search with `tsvector`. Same schema and rules as SQLite. |
| **DuckDB** | Analytics and reporting | Sales vs prep, cook-time trends, supplier price history, HACCP patterns. Reads the operational data; it is not the HACCP system of record in v1. |

Rules:

- Choose the backend in config (`store.driver = sqlite | postgres | duckdb` plus a DSN). Never by build flag alone.
- Write SQL per backend in `internal/store/<backend>/queries/`, generated with `sqlc` where the backend is supported. Keep each backend's schema and queries in its own folder; do not share one SQL file across dialects.
- Store timestamps as UTC, money as integer cents, temperatures as numeric. Do not encode closed value lists (record types, item states) as database enums or `CHECK` lists; store text and validate in Go, so all backends behave the same.
- Use `CHECK` and composite `UNIQUE` constraints only for permanent invariants (for example one record per hash).
- The HACCP hash chain and append-only rule are enforced in Go, so they hold on every backend. The conformance suite proves it.
- A `kishtin store export` / `import` pair moves all data between backends through deterministic JSONL, with record counts and hashes checked after import.

**DuckDB and Quack.** Quack is DuckDB's new HTTP client-server protocol (announced May 2026). It lets several processes or machines read and write one DuckDB database, which suits a reporting server that dashboards and the kitchen app share.

- Treat Quack as experimental until DuckDB marks it production-ready (planned alongside DuckDB 2.0). Check its status before relying on it.
- Quack has no TLS and allows every query by default. Bind it to localhost or put it behind a TLS reverse proxy, and always set an auth token.
- DuckDB is built for analytics. Its small-write throughput is lower than Postgres, so keep live order and pass traffic on SQLite or Postgres.
- The Go DuckDB driver needs CGO. Build it only into a separate `duckdb` build or a `kishtin-analytics` binary, so the default server stays pure Go.
- Simplest path first: DuckDB can read the SQLite file directly for reports. Add Quack when reports need to run on another machine or alongside live writes.

### Repository layout

```
cmd/kishtind/          server daemon: config, wiring, startup
cmd/kishtin/           CLI for people and agents (JSON output with --json)
internal/haccp/        HACCP records, critical limits, pass/fail rules
internal/menu/         dishes, timings, stations, equipment, capacities
internal/orders/       orders, items, courses, ticket lifecycle
internal/scheduler/    pure planning logic (no I/O)
internal/kds/          pass and station views, SSE broadcasting
internal/auth/         users, roles, PIN sessions
internal/store/        storage interfaces and transactions
internal/store/sqlite/   default backend: schema, queries, migrations
internal/store/postgres/ optional backend
internal/store/duckdb/   analytics backend (CGO, separate build)
internal/store/storetest/ conformance suite run against every backend
internal/web/          HTTP handlers, API routes, embedded frontend assets
internal/voice/        command grammar, parsing, confirmations (no audio I/O)
internal/speech/       client for the local speech-to-text / text-to-speech sidecar
internal/ai/           client for an optional local model server (drafts only)
internal/pdftext/      PDF text extraction without CGO (go-pdfium on wazero)
internal/books/        record book bundles, templates and page builders
internal/integrations/ POS adapters, starting with Square (later)
frontend/              Svelte 5 app (kit-ui), built with Vite into frontend/dist/
```

Dependencies point inward. `scheduler` and the rules in `haccp` must not import `store`, `web` or anything that does I/O.

## Module notes

### HACCP (`internal/haccp`)

Record types: fridge/freezer temperature, delivery, cook/reheat, cooling, hot holding, batch label, cleaning task, hygiene inspection, staff training, fitness to work, delivery to a customer, and a manager's sign-off. Each record stores who logged it, when, the reading, the pass/fail result, and a corrective action when it failed.

Default critical limits, from FSAI Safe Catering guidance. Keep these defaults as named constants in one file, each with a comment citing its source. They are the starting point; the restaurant can customise them (see Customisable limits below):

| Check | Limit |
|---|---|
| Fridge | ≤ 5°C |
| Freezer | ≤ −18°C |
| Cooking, core | ≥ 75°C |
| Reheating, core | ≥ 70°C |
| Hot holding | ≥ 63°C |
| Cooling | Into fridge or blast chiller within 2 h of cooking. If not, and still above 10°C at 2.5 h, serve immediately or discard. |
| Chilled delivery | ≤ 5°C (borderline up to 8°C) |
| Frozen delivery | ≤ −18°C (borderline up to −15°C) |

#### Customisable limits

Limits are data, not code. A manager can set limits that match the restaurant's own HACCP plan, for example a different core temperature for chicken, pork, burgers or sous-vide dishes.

- **Limit profiles.** A limit applies at the most specific level that has one: dish → food category (poultry, pork, minced meat, whole-muscle beef, fish, rice, ready-to-eat) → check type default → the FSAI default constant. The UI and every record show which level the limit came from.
- **Time–temperature equivalents.** A cooking limit can be a list of combinations instead of one number, for example 75°C instantly, 70°C for 2 minutes, or a validated sous-vide time at a lower temperature. The cooking check records the temperature and the hold time, and passes if any combination is met.
- **Per-unit limits.** Fridges and freezers can have their own limits (a fish fridge at 0–2°C, a drinks fridge with a looser limit and marked non-food).
- **Stricter is easy, looser needs a reason.** Making a limit stricter than the FSAI default only needs a manager. Making it less strict (for example a rare-beef or pink-burger exception) needs a manager, a written reason, and a reference to the HACCP plan section or validation that supports it. The UI shows a clear warning that the limit is below FSAI guidance.
- **Versioned and audited.** Every change creates a new limit version with who, when and why. Limits are never edited in place. Each HACCP record stores the ID of the limit version it was checked against, so an inspector can see exactly which rule applied on that day, even after limits change.
- **No silent re-grading.** Changing a limit never changes the pass/fail result of records already saved.
- **Preview before saving.** A limit change shows how recent records would have scored under the new limit, then asks to confirm (show the plan, then act).
- Export the full limit history alongside HACCP records, so the inspection pack is complete.

#### Customisable guidance and rules

Every guideline Kishtin checks against is customisable, not only the limits, with the same rules: versioned, previewed before saving, a reason every time, a HACCP plan reference for anything less strict, manager password, and no re-grading of saved records.

- **Reference guidance.** The baseline limits are compared with when deciding "looser". It starts as the FSAI defaults and can be replaced by the restaurant's own HACCP plan, for example to recognise 70°C for 2 minutes as equal to 75°C. Guidance looser than the FSAI constants needs a plan reference; limits looser than the guidance need one. Each limit stores the guidance version it was checked against.
- **Food-safety rules.** Whether borderline readings need a corrective action, and whether names must come from the managed lists. A failed reading always needs a corrective action; that rule is not customisable. Each record stores the rules version in force.
- **Managed lists.** Fridges and freezers, food categories and cleaning tasks, so names stay consistent and unit or category limits apply reliably.
- **System settings.** Backup schedule and retention, upload size, preview length. Paths, listen address and TLS stay in `kishtin.toml`.

Rules:

- A failed reading cannot be saved without a corrective action.
- Records are append-only. A mistake is fixed by adding a correcting record that points to the original, never by editing or deleting. Inspectors need to trust the history.
- Records are hash-chained: each record stores a SHA-256 of its own content plus the previous record's hash. `kishtin haccp verify` re-checks the chain and reports the first break, if any. Attached files are stored under their SHA-256 and re-checked the same way.
- Provide a CSV export by date range for inspections.
- Batch labels track made-on and use-by dates (for example the house grape soda syrup, 5–7 days chilled).
- Delivery records can attach a photo of the supplier's docket or invoice, taken on a phone. Store files on local disk under a content hash, referenced from the record. This works without AI and gives traceability back to the supplier; AI extraction comes later (see AI).

#### Record books (`internal/books`)

Official record books (for example Ireland's FSAI Safe Catering booklets) are filled in from HACCP records, never typed in twice.

- A **bundle** is one country's or issuer's set of booklets, kept as a data file in `internal/books/bundles/`. Developers add a bundle when a country needs one. A new edition with renamed or reordered columns is a data change.
- Each booklet is a template: its columns, the record field in each, its notes, and the edition and SHA-256 of the official PDF it copies. Rows come from a small set of pure builders (fridge grid, cooking log, hot holding log, deliveries, and a daily record made of other booklets' sections). Only a booklet that needs a new kind of row needs code.
- The restaurant picks its bundle and changes what is relevant (booklets in use, titles, columns, notes, sheet layout, manager check, its own added booklets, its uploaded copy) through the versioned `record_books` settings. The same rules as limits apply: preview, reason, manager password, history.
- These settings change only how records print. They never change a record or its result.
- A manager's sign-off of a period is a `sign_off` HACCP record in the hash chain, saved only through the sign-off action (manager password), never through ordinary logging. It stores what the period held when signed. A period with missed checks or readings without an action needs a note.
- Any record can say when its check was done (`taken_at`) and who did it (`signed_by`). Records are found and shown by that time (`done_at` in the store); the time saved stays in the record and the hash chain. Kishtin shows no late marker: how late is acceptable is the restaurant's decision, controlled through permissions (`haccp.backdate`, `haccp.signed_off`).
- Never fill in a missed check. Never invent a bundle or edition: a bundle copies a real published booklet and records its source.
- Old paper pages come in through paper import: each record keeps the photo (`paper_file`), when the check was done (`taken_at`) and the name on the paper (`signed_by`), and is graded against the limits in force at import. Saved all or none.
- An optional local model (`internal/ai`, `[ai]` in `kishtin.toml`, local addresses only) may suggest booklet columns from a copy's text (`internal/pdftext`) or read a photo of a paper page. Its output is always a draft a person reviews, checked by the same rules as typed input.

### Menu and stations (`internal/menu`)

Each dish defines:

- station and the equipment it uses
- batch capacity on that equipment (how many units at once)
- cook, rest and plate times, in seconds
- whether it can be held at the pass, and for how long, before quality drops
- modifiers that change timing (for example "well done" adds 120 s)

Each piece of equipment defines how many slots it has. Capacity is shared by every dish that uses that equipment.

### Orders and pass (`internal/orders`, `internal/kds`)

- Item states: `queued → held → fired → cooking → resting → plating → ready → served`, plus `voided`.
- The expo fires a course. Firing sets the course's target time.
- The pass view shows every open table with a timer per course and colour by state: on track, at risk, late.
- Station views show only that station's queue, in the order the scheduler recommends.
- Every state change is broadcast over SSE to all screens.
- Record actual start and finish times per item. These feed the learned cook times.

### Scheduler (`internal/scheduler`)

The core function is pure and deterministic:

```go
func Plan(state KitchenState, cfg Config, now time.Time) Plan
```

`KitchenState` holds fired items, current equipment use, and dish timings. `Config` holds the chosen strategy and its settings. `Plan` returns ordered recommendations per station, each with a reason.

#### Customisable scheduling

The v1 algorithm below is the default, but the kitchen can tune it or swap it.

- **Strategies.** A `Strategy` interface with one method, `Plan(state, cfg, now) Plan`. v1 ships `backward` (the default, below). Other strategies can be added later (for example `fifo`, for a quiet lunch service, or a priority-first strategy) without changing callers. Each strategy is pure, deterministic and has its own golden tests.
- **Settings**, all editable by a manager, with sensible defaults:
  - batching window (default 2 minutes) and whether batching is allowed per dish
  - target time policy: as soon as possible, or a fixed pace (for example mains 20 minutes after starters are served)
  - course gaps per service (lunch vs dinner)
  - priorities: takeaway, allergy orders, large tables, or a table the expo flags as urgent
  - maximum time a dish can wait at the pass before it counts as late
  - buffer time added per station (for example 30 seconds for a slow plating station)
  - re-plan interval (default 15 seconds)
- **Service profiles.** Save settings as named profiles ("Weekday lunch", "Saturday dinner", "Christmas set menu") and switch between them from the pass.
- **Replay before switching.** Changing a strategy or setting shows a replay of a recent service under the new settings next to what actually happened (how many courses would have been late, average wait at the pass), then asks to confirm.
- Settings changes are versioned and logged like limit changes, and every plan records which profile and version produced it.
- The expo's manual override always beats any strategy.

v1 default algorithm, `backward` (backward scheduling with capacity):

1. For each fired course, compute the target time. Default: now plus the longest total time among its items.
2. For each item, compute its latest start: target minus cook, rest and plate time.
3. Sort items by latest start, earliest first.
4. Assign items to equipment slots in that order. When equipment is full, the item starts when the earliest slot frees up. If that pushes it past its latest start, move the whole course's target later so the course still finishes together, and mark it **at risk**.
5. Batch identical items from different tables into one cook when their start times fall within a configurable window (default 2 minutes) and capacity allows. For example, start 3 burgers together for tables 4 and 7.
6. Return actions such as "Grill: start 3 burgers now (tables 4, 7)" and "Fryer: start fries for table 4 at 18:42".

Requirements:

- Every recommendation carries a human-readable reason.
- The expo can override any recommendation. Overrides are logged and the plan is recomputed.
- Re-plan on every event (new order, fire, start, finish, void) and on a timer every 15 seconds.
- Learned timings: after enough records, replace configured cook times with a smoothed average of actual times (for example an exponentially weighted moving average). Show both values in the menu screen so a manager can see and reset them.

### Voice (`internal/voice`, `internal/speech`)

Voice is a core feature, not an extra. A chef holding a pan must be able to run the pass and log checks without touching a screen.

**How it works:**

- **Capture:** push-to-talk on a large on-screen button, a foot pedal or a Bluetooth headset button. Hands-free listening for a wake word ("Kishtin") is opt-in per device and off by default, because it causes false triggers in a loud kitchen and raises privacy concerns: it shows a clear indicator while on, stops when the screen is hidden or reloaded, and the server ignores and never logs speech that does not start with the wake word. Answers and confirmations in hands-free mode need the wake word too.
- **Microphones:** close-talk headsets or directional mics. Kitchen noise (extractors, fryers, tickets, music) is the main accuracy problem, so test with recordings from the real kitchen.
- **Recognition:** the browser records audio and sends it over the local network to the server. The Go server passes it to a local speech-to-text sidecar process (for example whisper.cpp or a similar small model) over HTTP or stdin. Running it as a sidecar keeps the Go binary free of CGO. No audio leaves the building.
- **Understanding:** v1 matches the transcript against a fixed command grammar with fuzzy matching on menu item names, table numbers and staff vocabulary. Bias recognition toward the current menu and open tables.
- **Feedback:** every command is confirmed by a short spoken reply (local text-to-speech, for example Piper) and on screen. If the match is uncertain, ask ("Table 4 or table 14?") rather than guess.
- **Safety:** voids, bin/discard actions and HACCP fails need a spoken confirmation ("Confirm void"). Every voice action is logged with its transcript, so mistakes can be traced.
- **Fallback:** everything voice can do must also be possible by touch.

**v1 command examples:**

| Say | Does |
|---|---|
| "Fire table 4 mains" | Fires the course and sets its target time |
| "What's next on grill?" | Reads the next scheduler recommendations for that station |
| "Burgers on, tables 4 and 7" | Marks those items as cooking |
| "Table 7 ready" | Marks the course ready at the pass |
| "86 salmon" | Marks salmon as sold out (kitchen slang for unavailable) |
| "Walk-in fridge 3.4 degrees" | Logs a HACCP temperature reading, with pass/fail spoken back |
| "Core temp chicken 78" | Logs a cooking check |

`internal/voice` holds the grammar and parser as pure functions, tested with transcript fixtures, including misheard variants ("for" vs "four", "fire" vs "fryer").

## Integrations (later)

### Square

The restaurant uses **Square** for its till and payments.

- Define a `POSAdapter` interface in `internal/integrations`. Square is the first adapter. Keep the Square SDK and API types inside `internal/integrations/square`; core packages only see our own order types.
- **Menu sync:** pull items and modifiers from the Square Catalog API and map each Square item to a local dish, which adds the kitchen data Square doesn't have (station, equipment, capacity, timings).
- **Orders in:** fetch new and updated orders from the Square Orders API. The server sits inside the restaurant's network, so prefer polling the Orders API every few seconds over exposing a public webhook URL. If webhooks are added later, verify Square's signature header and handle every event idempotently.
- **Status out:** optionally write ticket progress back to Square so front of house sees it on the till.
- **Payments:** stay entirely in Square POS and Square Terminal. This system never receives, logs or stores card data, which keeps it out of PCI DSS card-data scope.
- **Offline:** if Square or the internet is unreachable, manual order entry keeps working, and orders sync when the connection returns. Never drop or duplicate an order: dedupe on Square's order ID and version.
- Store the Square access token in config, the environment, or a credentials file the server writes in the data folder when a manager connects Square on the screens (readable by the server only). Never in the repo, the database, backups or exports.

## AI (later, after the core system works)

Do not start this until roadmap stages 1–5 are done. AI must stay **local and small**: no cloud APIs, everything runs on the restaurant's own hardware.

- Prefer several small specialised models over one large general one, each with a narrow job. Candidates to evaluate include Liquid AI's LFM2 family (including its audio and on-device variants) and similar small open models. Choose by measured accuracy on our own kitchen recordings and data, speed on the target hardware, and licence terms for commercial use.
- Possible jobs:
  - **Speech:** better recognition in kitchen noise, or speech-to-intent directly, replacing the fixed grammar.
  - **Command understanding:** looser phrasing ("get table 4's mains going").
  - **Assistant answers:** "What's left on 12?", "How long on the lamb?", read from live system state.
  - **Suggestions:** prep quantities from past sales, or flagging HACCP patterns (a fridge drifting warmer each afternoon).
  - **Supplier invoices and receipts:** read a photo or PDF with a small local vision or document model (for example LFM2-VL or a similar model, with plain OCR as a fallback) and extract the supplier, invoice number, date, line items, quantities, unit prices, VAT and totals.

### Invoice processing rules

- Extraction only fills in a draft. A person reviews and confirms it before it is saved, with the fields shown next to the original image and uncertain values highlighted.
- Check the arithmetic: line totals, VAT and the invoice total must add up, or the draft is flagged.
- Match the supplier and products to known suppliers and items, and flag price changes since the last invoice.
- Link each invoice to the HACCP delivery record for the same delivery, so a product can be traced from supplier to dish.
- Keep the original file unchanged alongside the extracted data. Irish Revenue rules require business records to be kept for six years, so invoices are never deleted by the app.
- Duplicate check on supplier plus invoice number before saving.
- Accounting export (CSV first; software such as Xero or Sage later) is a separate, later step.
- Run models as sidecar processes behind a small internal interface, so a model can be swapped without touching core code.
- AI only ever **suggests or interprets**. The deterministic scheduler and HACCP rules stay the source of truth. An AI output never saves a HACCP record or voids an order without the same confirmation a human action needs.

## Working on this codebase

### Commands

```
make dev        # run the server and the Vite dev server with live reload on :8080
make frontend   # build the Svelte app into frontend/dist/ for embedding
make frontend-test # frontend unit tests and kit-ui-check
make test       # go test ./...
make lint       # golangci-lint run
make generate   # sqlc generate
make migrate    # goose up for the configured backend
make test-pg    # conformance suite against Postgres (needs a local Postgres)
make test-duckdb # DuckDB backend tests (CGO build)
```

### Conventions

- Format with `gofmt`. Lint must pass before committing.
- Return errors with context: `fmt.Errorf("fire course %d: %w", id, err)`. Do not panic in request paths.
- Pass `context.Context` as the first argument to anything that does I/O.
- Temperatures in °C as `float64`, rounded to 0.1 for display. Durations as `time.Duration`. Money as integer cents in EUR.
- Table-driven tests. Every HACCP limit needs tests on both sides of its boundary, and on the boundary itself.
- Every store change passes the conformance suite on SQLite. CI also runs it on Postgres, and on DuckDB for the analytics queries.
- Tests, fixtures and docs use synthetic data only: invented staff, suppliers and orders, never real ones.
- The scheduler has golden tests: fixed kitchen states in `testdata/` with expected plans. Any change to plan output must update the golden files on purpose and explain why in the commit.
- UI text is plain English, written for kitchen staff. Buttons say what they do ("Fire mains", "Mark ready").

### Rules for agents

- Do not change a default HACCP critical limit constant unless the task asks for it. If you do, update the constant, its source comment, its tests, and this file in the same change. Restaurant-specific limits are data and are changed through the app, never hard-coded.
- Pass/fail logic always takes the limit version as an input. Never read a limit from a global or the database inside the rule functions.
- Keep `internal/scheduler` free of I/O, global state and wall-clock reads.
- Never add a code path that edits or deletes a HACCP record.
- Never handle raw card data.
- Never send audio, transcripts or kitchen data to a cloud service. Speech and AI run locally.
- Any new action must work by touch as well as by voice.
- Keep personal data to a minimum (staff names and PINs only) to stay within GDPR obligations. Fitness to work records keep only the outcome (who, when, why, fit or not, action), never symptoms or answers, and only managers with a password can log or see them.
- Prefer the standard library, then the libraries in the Go libraries table. Ask before adding anything else, and explain what it replaces.
- Write a migration for every schema change, for every supported backend in the same change. Never edit a migration that has already shipped.
- The CLI is a client of the server's HTTP API; it never opens the database directly while the server is running. The API always requires a key, binds to loopback by default, and needs a key plus TLS or a trusted network to listen more widely. (Pattern taken from docbank.)
- Work on feature branches. Never commit to or push `main`. Open pull requests and leave merging to a person. PR descriptions lead with what changed for the kitchen, with screenshots of real UI using synthetic data.
- Don't document unbuilt features as if they exist. This file describes intent; feature docs describe what ships.
- Every new feature gets a CLI command and API route alongside its UI, using the same internal function.
- Keep the design principles above. If a change would send data off the machine, it must show what is sent and ask first.

## Roadmap

1. **HACCP in Go**: port the prototype, staff PIN login, CSV export, backups.
2. **Menu, stations and equipment setup**, including batch capacities, timings, and food categories with their own HACCP limits.
3. **Manual orders and the pass view**: FOH enters orders, expo fires courses, live screens.
4. **Scheduler v1**: backward scheduling, capacity, batching, explanations, overrides.
5. **Voice control v1**: push-to-talk, local speech-to-text, fixed command grammar, spoken confirmations. Start building alongside stages 3–4 so commands match the real pass workflow.
6. **Learned cook times** from recorded actuals.
7. **Square integration**: menu sync, orders in, optional status out.
8. **Local AI**: small specialised models, evaluated against real kitchen data. Supplier invoice extraction is a good first candidate, since every result is reviewed by a person anyway.

## Open questions

- Which Square products are in use (Square POS, Square for Restaurants, Terminal), and does Square currently print kitchen tickets?
- Do suppliers send paper dockets, emailed PDFs, or both? Emailed PDFs could be collected from a dedicated inbox later.
- What mic setup suits the kitchen: headsets per chef, or one mic at the pass?
- How many stations, and how many screens: one per station, or tablets shared?
- Should takeaway and delivery orders share the same queue as tables?
- What hardware will run the server, and how will backups leave the building?
