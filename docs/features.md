---
title: What Kishtin does
description: Everything Kishtin does today, and what is not built yet.
---

# What Kishtin does

Kishtin is a kitchen system for one restaurant. It runs on a small computer in the restaurant, keeps working with the internet down, and keeps everything in one database file the restaurant owns. This page lists what is built today; the [plan](https://github.com/CathalByrneGit/Kishtin/blob/main/AGENTS.md) describes where it is going.

## What is built

Roadmap stages 1 to 7: the HACCP log; the menu with stations and equipment; orders and the pass; the cook scheduler; voice control; cook times learned from real cooks; and orders from Square (and SumUp).

- **Screens** for a computer, served by `kishtind` itself: the Pass, Stations and Orders; HACCP Today, Records, Record books, Exports and Verify; and Setup for the menu, the scheduler, fridges and tasks, limits and rules, HACCP records, staff, device keys, backups and settings. Search with Ctrl K. Each device can instead show a full-screen pass screen, station screen or phone layout, with large type and 56 px buttons. See [Screens](screens.md).
- **Menu**: stations, equipment with how many units it holds, and dishes with cook, rest, plate and hold times, batch capacity, modifiers such as "well done +2 min", and 86 for sold out. See [Menu, orders, the pass and the scheduler](kitchen.md).
- **Orders and the pass**: tables and takeaways with daily ticket numbers and an optional name (such as the guest's) shown as the heading; courses held until fired, and held again if nothing has started; items moved from waiting to cooking, resting, plating, ready and served; items changed after ordering (voided and replaced in one step); a course served together; expo overrides (new target time, urgent) logged with every other change in each order's history.
- **Voice control**: push-to-talk on kitchen screens for firing, holding, starting, readying, voiding, 86ing and asking what is next, and for fridge, freezer, cooking, delivery, cooling, batch label and cleaning records, asking for any detail not said. Optional hands-free listening for "Kishtin", off by default on every device. Speech is recognised by a local whisper.cpp sidecar and replies spoken by a local Piper sidecar; no audio leaves the building. It asks when unsure, needs "confirm" for a HACCP fail, and logs every command. See [Voice control](voice.md).
- **Connections**: Setup → Connections lists what Kishtin can connect to, what each sends, and its state. Square and SumUp: a manager connects each there with a token, checked with the till company and kept outside the database.
- **Orders from Square**: orders taken on the Square till come into the kitchen on their own, deduped on Square's order ID and version. Till items are mapped to dishes once; drinks are marked not for the kitchen. Changes on the till that would remove food are flagged to the pass, never voided automatically. Kishtin only reads from Square, never sees card data, and keeps working when Square is unreachable. See [Orders from Square](square.md).
- **Sales from SumUp**: sales paid on SumUp come into the kitchen as takeaways, mapped to dishes by item name. SumUp reports a sale only once it is paid, so this suits counter service and takeaways. Refunds are flagged to the pass, never voided automatically. See [Sales from SumUp](sumup.md).
- **Station tickets**: each station sees one ticket per order and course, with a button per item, a timer and status colour, the scheduler's reason one tap away, all-day counts per dish (with "start these together"), and recently done items to undo. The pass has all-day counts too.
- **Cook scheduler**: backward scheduling within equipment capacity, with batching across tables, priorities and station buffers. It tells each station what to start and when, says why for every recommendation, and shows each course as on track, at risk or late. Its settings are versioned.
- **HACCP log**: fridge, freezer, delivery, cook, reheat, cooling, hot holding, batch label and cleaning records, checked against FSAI limits. See [HACCP log](haccp.md).
- **Versioned limits**: a manager can set limits per dish, food category or fridge. Every change is a new version, and every record stores the version it was checked against.
- **Customisable guidance and rules**: the reference guidance limits are measured against (starts as FSAI), whether borderline readings need a corrective action, managed lists of fridges, food categories and cleaning tasks, and system settings such as the backup schedule. All versioned, previewed before saving, and audited. See [Settings](settings.md).
- **Tamper evidence**: records are append-only and hash-chained. Docket photos are stored under their SHA-256. `kishtin haccp verify` re-checks all of it.
- **Today**: every fridge and freezer check (done, due or missed, with check times per unit), the cleaning checklist, checks done out of due, out-of-limit readings and their actions, and batches in use.
- **Live sync**: every open screen shows new records straight away.
- **Exports**: CSV of records and the limit history, and a PDF inspection pack, each with record IDs and hashes.
- **Record books**: official HACCP booklets filled in from the records for a month, with missed checks marked. Bundles per country (Ireland's FSAI Safe Catering booklets SC1 to SC9, and a general set for any country), which a manager can change in Setup › Food safety › HACCP records. A manager signs off each period, which fills the booklets' manager check line. Kishtin compares your copy of a booklet with its template, checks the publisher for new editions when asked, and brings in old paper pages from a photo. An optional local model can suggest columns and read photos. See [Record books](record-books.md).
- **Backups** on a schedule, and before a new version changes the database. Each copy is verified and gets a `.sha256` file, and photos are kept with them. `kishtind restore` checks a backup and puts it back, keeping what it replaced. See [Restoring from a backup](security.md#restoring-from-a-backup).
- **Opens like an app**: a tablet or phone can add Kishtin to its home screen and open it full screen from its own icon. See [Add Kishtin to the home screen](screens.md#add-kishtin-to-the-home-screen).
- **Logging without a connection**: when a tablet or phone cannot reach the Kishtin computer, a check can be kept on the device and is sent when the connection is back. See [HACCP log](haccp.md#logging-without-a-connection).
- **Printable batch labels**: a batch label record prints as a label for the tub (product, made on, use by, who made it, record number), for a label printer or on A4. See [HACCP log](haccp.md#printing-batch-labels).
- **Closed days**: weekly closed days and closed dates, so checks are not due, and not missed, when the restaurant is shut. See [HACCP log](haccp.md#closed-days).
- **Checks logged later**: any check can be logged with the time it was done and who did it, and **Catch up** lists the daily checks with no record so they can be filled in together. See [HACCP log](haccp.md#checks-logged-later).
- **Staff login**: PIN on shared kitchen devices, password for manager actions, API keys for the CLI and scripts. A forgotten manager password can be reset with `kishtind reset-password` on the Kishtin computer itself.
- **Starts with the computer**: the installer offers it, and it can be turned on in Settings or with `kishtind startup on`. It uses Task Scheduler on Windows, a LaunchAgent on macOS and a systemd user service on Linux. See [Installing Kishtin](install.md#start-with-the-computer).
- **Weekly and longer checks**: cleaning tasks can be due every week, two weeks, four weeks or any number of days, such as beer line cleaning or extractor filters, and show on Today as due or overdue. See [HACCP log](haccp.md#weekly-and-longer-tasks).
- **Guides for non-technical owners and staff**: a [getting-started guide](getting-started.md), printable one-page staff guides in the app, a getting-started list on Today that ticks itself off, and [a guide to Kishtin's records for your environmental health officer](for-inspectors.md).
- **Allergens**: each dish lists which of the 14 regulated allergens it contains, checked by a manager; an allergen chart to show or print for customers; allergy tickets show what each item contains. See [Menu, orders, the pass and the scheduler](kitchen.md#allergens).
- **Collection screen**: a screen facing customers with takeaway numbers being prepared and ready to collect, with no staff login. See [Screens](screens.md#kitchen-screens).
- **Learned cook times**: Kishtin learns each dish's real cook time from the station screens and, when a manager turns it on, new orders use it once a dish has 10 cooks. Setup › Kitchen › Menu shows set and learned times side by side, with a reset per dish. See [Menu, orders, the pass and the scheduler](kitchen.md#learned-cook-times).
- **Reports**: pass times (share on time, fire to pass, waiting at the pass, why courses were late, dish times against set times) and HACCP checks (checks and cleaning done and missed, out-of-limit readings, each fridge's temperatures). See [Reports](reports.md).
- **Recording**: scheduler plans as they change, with why each course ran late; an activity log of sold-out dishes, logins, screens dropping off and how each HACCP record was entered; the device behind each action; every version of each till order; optional voice recordings. Customers' names and order notes are cleared 30 days after an order closes. Leavers can be anonymised. See [What Kishtin records](recording.md).
- **Permissions**: a manager can give any role or named member of staff jobs such as signing off records, and choose whether a PIN is enough. See [Permissions](permissions.md).
- **HTTP API** (OpenAPI spec in [api/openapi.json](https://github.com/CathalByrneGit/Kishtin/blob/main/api/openapi.json)) and the **`kishtin` CLI**, which uses the API.

## Not built yet

- Ticket progress written back to Square, and menu sync from the Square catalog.
- Scheduler service profiles, replaying a service under new settings, and a fixed pace between courses.
- Postgres and DuckDB backends. Only SQLite is built.
- Off-site backups. Kishtin writes backups, with the photos, on the Kishtin computer. Copying the backup folder somewhere else is up to you.
- Saving anything but HACCP checks while the Kishtin computer is unreachable. Checks can be kept on the device and sent later; orders, the pass and settings cannot be changed until the connection is back, and screens show the last data with a "Not connected" notice.
