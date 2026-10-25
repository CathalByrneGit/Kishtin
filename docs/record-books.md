# Record books

Kishtin fills in official HACCP record books from the records staff log, so nobody has to copy readings onto paper. The first pack is the FSAI Safe Catering record books (Ireland).

Head chefs and managers only.

## What it can fill in

| Booklet | Filled in | How |
|---|---|---|
| SC2 Fridge/Freezer/Chill Display Temperature Records | Yes | A month grid: a row per day, a column per check on each fridge and freezer (AM and PM, or the check times set in **Fridges and tasks**), six units per sheet. |
| SC3 Cooking/Cooling/Reheating Records | Yes | A row per cooking record. A cooling record joins the row of the same food cooked within an hour of its "cooked at" time; otherwise it gets its own row. Reheats get their own rows. |
| SC4 Hot Hold/Display Records | Yes | A row per food per day, with up to three checks. A fourth check starts a new row. |
| SC1, SC5, SC6, SC7, SC8, SC9 | No | Kishtin does not record what they ask for yet. The screen says what is missing for each. |

Columns Kishtin does not record print blank, and the booklet says which ones they are:

- SC3 "Time started cooking" prints N/A when a core temperature is recorded, because the booklet says the start time is not needed then.
- SC4 "Time into hot hold" is always blank.

## What the pages show

- **Signed** is the first name of the person who logged the record.
- **FAIL** and **CHECK** mark failed and borderline readings, using the restaurant's limits saved with each record. The comments give the reason and the corrective action.
- **Corrections** show in place of the record they correct, with "Corrected" and the reason in the comments. Both records stay in the log.
- **missed** marks an SC2 check that was due and has no reading. Kishtin never fills a gap in. A check counts as missed once the next check has started (or the day has ended), and only for fridges and freezers on the managed list, from the first record ever logged. Kishtin does not know which days you were closed, so days you were closed show as missed.
- **Kishtin ref** (SC3, SC4) is the record number. The footer gives the record numbers used, the hash chain result and the chain head. Run `kishtin haccp verify` to check the records.
- The **Manager/Supervisor check** line is left blank to sign by hand.

## On the screen

![Record books](screenshots/record-books.png)

**Record books** under HACCP. Choose a month. Each booklet shows how many checks were missed and how many records it uses. **Preview** shows the filled-in sheets and a list of missed checks. **Download PDF** gives a landscape A4 PDF to print.

## From the command line

```
kishtin haccp books                              # list the booklets and what each needs
kishtin haccp book SC2 --month 2026-10           # missed checks and blank columns
kishtin haccp book SC2 --month 2026-10 -o sc2.pdf
kishtin haccp book SC4 --json                    # the filled-in pages as JSON
```

API: `GET /api/haccp/books`, `GET /api/haccp/books/{book}?month=YYYY-MM`, `GET /api/haccp/books/{book}/pdf?month=YYYY-MM`.

## Templates

Each booklet is a template kept as data in `internal/books/packs/`. A template lists its columns, which record field goes in each, its notes, and the SHA-256 and edition of the official PDF it copies. A new edition with renamed or reordered columns is a change to that data, not to code. A booklet that needs a new kind of row (for example a checklist) needs a new builder.

What Kishtin does not do yet:

- Templates ship with Kishtin. They cannot be added or edited from the screens.
- Kishtin does not check the FSAI site for new editions.
- It does not print onto the official PDF pages. It draws its own pages in the same layout.
- Ask your environmental health officer whether they accept printed or on-screen records in place of the paper booklets.
