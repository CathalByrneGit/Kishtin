# HACCP log

What Kishtin's HACCP log does today, and what it does not.

## Records

| Type | What you record | Checked against |
|---|---|---|
| `fridge` | unit, temperature | fridge limit (FSAI default ≤ 5°C) |
| `freezer` | unit, temperature | freezer limit (FSAI default ≤ −18°C) |
| `delivery` | supplier, product, chilled/frozen/ambient, temperature, packaging intact, in date, optional docket photo | chilled ≤ 5°C (borderline to 8°C), frozen ≤ −18°C (borderline to −15°C), ambient: no temperature limit. Packaging not intact or not in date fails. |
| `cook` | food, core temperature, optional hold time | core ≥ 75°C, or any time–temperature combination the limit allows |
| `reheat` | food, core temperature, optional hold time | core ≥ 70°C |
| `cooling` | food, cooked at, into fridge at, temperature at 2h30 if late | into the fridge within 2 h; otherwise above 10°C at 2h30 means serve now or discard |
| `hot_hold` | food, temperature | ≥ 63°C |
| `batch_label` | product, made on, use by (or shelf life in days), quantity | no limit; use-by cannot be before made-on |
| `cleaning` | task, area, done or not | not done is a fail; a task can be done once a day |

Delivery results also say what to do: a borderline delivery is accepted only if used or stored straight away; a failed one is rejected.

Temperatures are compared to 0.1°C. A reading exactly on the limit passes. Fridge, freezer, food category and cleaning task names can be fixed to managed lists; see [settings.md](settings.md#lists).

## Results

- **pass**: within the limit.
- **borderline**: outside the target but inside the borderline range (deliveries), or late into the fridge but at or below 10°C at the cutoff (cooling).
- **fail**: outside the limit.
- **recorded**: no limit applies.

A **fail** always needs a corrective action before it can be saved. A **borderline** record needs one too, unless the food-safety rules say otherwise (see [settings.md](settings.md#food-safety-rules)). Each record stores the rules version that applied. Check a reading first, without saving, with `kishtin haccp log … --dry-run` or `POST /api/haccp/check`.

## Records cannot be changed

There is no way to edit or delete a record, in the API, the CLI or the store. To fix a mistake, save a new record of the same type with `--corrects <id>` and `--reason`. The original stays as it was.

Each record stores a SHA-256 hash of its content and of the record before it. `kishtin haccp verify` re-checks the chain from the first record and reports the first break. It also re-hashes every attached file and checks that every record's limit version exists.

What verify cannot catch: if someone deletes the newest records from the end of the chain, what is left still verifies. Compare the chain head (`X-Kishtin-Chain-Head` on exports, `head_hash` from verify and backups) with an older export or backup to catch that.

## Limits

The FSAI defaults are constants in `internal/haccp/limits.go`. The restaurant's own limits are data, changed through `kishtin limits set` or `POST /api/haccp/limits`:

- A limit can apply to a **dish**, a **food category**, a **fridge or freezer unit**, or a whole check type. The most specific one that exists is used, and each record says which level it came from.
- Changing a limit needs a manager logged in with a password (or a manager API key). A PIN is not enough.
- Every change needs a reason. A limit **less strict than your reference guidance** also needs a HACCP plan reference. The guidance starts as the FSAI defaults and can be changed; see [settings.md](settings.md#reference-guidance). The exception is a unit marked non-food, such as a drinks fridge, which only needs a reason.
- A cooking combination counts as less strict unless the guidance has one that is no hotter and held no longer. The FSAI guidance only has 75°C, so 70°C for 2 minutes needs a plan reference until you add it to your guidance.
- Each limit stores the guidance version it was checked against. Previews also say when a limit is below the FSAI defaults, whatever the guidance says.
- A change creates a new version. Old versions are kept, and records already saved keep their result.
- `kishtin limits set` first shows how recent records would have scored under the new limit, then asks before saving.

## Today

`kishtin haccp today` (or `GET /api/haccp/today`, or the Today screen) shows each fridge and freezer check for today as done, due or missed, the cleaning checklist, how many of the day's checks are done, today's out-of-limit records and whether each has an action, and batches in use.

Each unit has daily check times (see [settings.md](settings.md#lists)); by default AM until 12:00 and PM. A reading counts for the check whose time it falls in; the latest reading in a check counts. A check not logged by the time the next one starts is missed. A correction fills the check of the record it corrects.

## Exports

`kishtin haccp export --from DATE --to DATE` writes a CSV of the records in that range (both dates inclusive, Dublin time). Each row has the record ID, sequence number, UTC and Dublin times, who logged it, the reading, the result and reason, the corrective action, the limit version, and both hashes.

Leave out `--from` and `--to` for every record; `--type` and `--fails` narrow it further.

`kishtin haccp export --limits` writes the full limit history.

`kishtin haccp export --pdf -o pack.pdf` writes the **inspection pack**: a summary (records, failures, hash chain head, attached file checks, the guidance in use), failed and borderline records with their actions, every record by day, and the limit versions those records were checked against. It takes the same filters.

Text that starts with `=`, `+`, `-` or `@` is prefixed with `'` so spreadsheets do not run it as a formula.

## Docket photos

Upload a JPEG, PNG, WebP, HEIC photo or a PDF, up to `max_upload_mb` (15 MB by default) (`--docket file.jpg` on a delivery, or `POST /api/files`). It is stored under its SHA-256 in the data folder's `files/` folder, marked read-only, and never deleted by Kishtin. Kishtin does not read the docket's contents.

## Backups

On the schedule in the system settings (every 6 hours by default; see [settings.md](settings.md#system-settings)), and on `kishtin backup now`, Kishtin:

1. writes a consistent copy of the database with SQLite's `VACUUM INTO`,
2. opens the copy read-only and verifies its HACCP chain,
3. writes `<backup>.sha256` next to it,
4. removes backups beyond `backup_keep` (28 by default).

Backups stay on the server machine. Copy them somewhere else yourself. Attached files are not in the database backup: copy the `files/` folder too.
