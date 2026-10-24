# HACCP log

What Kishtin's HACCP log does today, and what it does not.

## Records

| Type | What you record | Checked against |
|---|---|---|
| `fridge` | unit, temperature | fridge limit (FSAI default ≤ 5°C) |
| `freezer` | unit, temperature | freezer limit (FSAI default ≤ −18°C) |
| `delivery` | supplier, product, chilled/frozen/ambient, temperature, optional docket photo | chilled ≤ 5°C (borderline to 8°C), frozen ≤ −18°C (borderline to −15°C), ambient: no limit |
| `cook` | food, core temperature, optional hold time | core ≥ 75°C, or any time–temperature combination the limit allows |
| `reheat` | food, core temperature, optional hold time | core ≥ 70°C |
| `cooling` | food, cooked at, into fridge at, temperature at 2h30 if late | into the fridge within 2 h; otherwise above 10°C at 2h30 means serve now or discard |
| `hot_hold` | food, temperature | ≥ 63°C |
| `batch_label` | product, made on, use by, quantity | no limit; use-by cannot be before made-on |
| `cleaning` | task, area, done or not | not done is a fail |

Temperatures are compared to 0.1°C. A reading exactly on the limit passes.

## Results

- **pass**: within the limit.
- **borderline**: outside the target but inside the borderline range (deliveries), or late into the fridge but at or below 10°C at the cutoff (cooling).
- **fail**: outside the limit.
- **recorded**: no limit applies.

A **fail** or **borderline** record cannot be saved without a corrective action. Check a reading first, without saving, with `kishtin haccp log … --dry-run` or `POST /api/haccp/check`.

## Records cannot be changed

There is no way to edit or delete a record, in the API, the CLI or the store. To fix a mistake, save a new record of the same type with `--corrects <id>` and `--reason`. The original stays as it was.

Each record stores a SHA-256 hash of its content and of the record before it. `kishtin haccp verify` re-checks the chain from the first record and reports the first break. It also re-hashes every attached file and checks that every record's limit version exists.

What verify cannot catch: if someone deletes the newest records from the end of the chain, what is left still verifies. Compare the chain head (`X-Kishtin-Chain-Head` on exports, `head_hash` from verify and backups) with an older export or backup to catch that.

## Limits

The FSAI defaults are constants in `internal/haccp/limits.go`. The restaurant's own limits are data, changed through `kishtin limits set` or `POST /api/haccp/limits`:

- A limit can apply to a **dish**, a **food category**, a **fridge or freezer unit**, or a whole check type. The most specific one that exists is used, and each record says which level it came from.
- Changing a limit needs a manager logged in with a password (or a manager API key). A PIN is not enough.
- Every change needs a reason. A limit **less strict than FSAI guidance** also needs a HACCP plan reference. The exception is a unit marked non-food, such as a drinks fridge, which only needs a reason.
- A cooking combination below 75°C counts as less strict than FSAI guidance, even a common equivalent like 70°C for 2 minutes, so it needs a plan reference.
- A change creates a new version. Old versions are kept, and records already saved keep their result.
- `kishtin limits set` first shows how recent records would have scored under the new limit, then asks before saving.

## Exports

`kishtin haccp export --from DATE --to DATE` writes a CSV of the records in that range (both dates inclusive, Dublin time). Each row has the record ID, sequence number, UTC and Dublin times, who logged it, the reading, the result and reason, the corrective action, the limit version, and both hashes.

`kishtin haccp export --limits` writes the full limit history. Hand both files to an inspector.

Text that starts with `=`, `+`, `-` or `@` is prefixed with `'` so spreadsheets do not run it as a formula.

## Docket photos

Upload a JPEG, PNG, WebP, HEIC photo or a PDF (`--docket file.jpg` on a delivery, or `POST /api/files`). It is stored under its SHA-256 in the data folder's `files/` folder, marked read-only, and never deleted by Kishtin. Kishtin does not read the docket's contents.

## Backups

Every `backup.interval` (6 hours by default), and on `kishtin backup now`, Kishtin:

1. writes a consistent copy of the database with SQLite's `VACUUM INTO`,
2. opens the copy read-only and verifies its HACCP chain,
3. writes `<backup>.sha256` next to it,
4. removes backups beyond `backup.keep` (28 by default).

Backups stay on the server machine. Copy them somewhere else yourself. Attached files are not in the database backup: copy the `files/` folder too.
