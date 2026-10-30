# Kishtin's records, for your environmental health officer

This page is for a restaurant owner to read before talking to their environmental health officer (EHO), and to show them. It explains, in an inspector's terms, what Kishtin records, how it stops records being changed after the fact, how to show records during an inspection, and what it does not do.

**Kishtin has not yet been reviewed by the FSAI or by any EHO.** Whether digital records are accepted in place of the paper Safe Catering booklets is the EHO's decision. The questions at the end are the ones to ask.

## In one paragraph

Kishtin is a kitchen system that runs on one computer in the restaurant. Staff log their food safety checks on a tablet or phone: fridge and freezer temperatures, deliveries, cooking, cooling, reheating, hot holding, cleaning, hygiene inspections, staff training and fitness to work. Each reading is checked against the restaurant's limits, which start as the FSAI's. A failed reading cannot be saved without a corrective action. Records can be printed in the layout of the FSAI's Safe Catering booklets.

## What is recorded

| Check | What is kept | Checked against |
|---|---|---|
| Fridges and freezers | unit, temperature, time, who | 5°C or below for fridges, −18°C or colder for freezers (FSAI), unless the restaurant set its own |
| Deliveries | supplier, product, chilled, frozen or ambient, temperature, packaging, in date, optional batch code, use-by, vehicle check and a photo of the docket | chilled 5°C or below (borderline to 8°C), frozen −18°C or colder (borderline to −15°C) |
| Cooking and reheating | food, core temperature, optional hold time | 75°C core for cooking, 70°C for reheating, or a time and temperature combination in the restaurant's HACCP plan |
| Cooling | food, when cooked, when into the fridge, temperature at 2½ hours if late | into the fridge within 2 hours; otherwise above 10°C at 2½ hours means serve now or discard |
| Hot holding | food, temperature | 63°C or above |
| Cleaning | task, area, done or not; daily, weekly or longer | not done is a fail |
| Hygiene inspection | the FSAI checklist, each question answered, the action for each "no" | any "no" is a fail |
| Training | staff member, topics or course, trainer, date | none |
| Fitness to work | staff member, date, reason, fit or not, action | none; only the outcome is kept, never symptoms |
| Manager sign-off | the period, a note, and what the period held when signed | none |

Every record keeps who logged it, when the check was done, when it was saved, the reading, the result (pass, borderline or fail), the reason, the corrective action, and which version of the limits it was checked against.

Each dish also lists which of the 14 regulated allergens it contains, with a chart for customers. See [kitchen.md](kitchen.md#allergens).

## Records cannot be changed or deleted

- **Nothing can be edited or deleted**, not on the screens, the command line or the API. A mistake is fixed by adding a correcting record, with a reason, which points to the original. Both are kept and both are printed.
- **Every record is chained to the one before it** with a SHA-256 hash: a digital fingerprint of its content and of the previous record's fingerprint. Changing any saved record, even one character in the database file, breaks the chain from that point on.
- **Verify** (Exports › Verify, or `kishtin haccp verify`) re-checks the whole chain, every attached photo, and that every limit a record was checked against still exists. If anything does not match, it shows the first record where it breaks.
- **What verify cannot catch**: if someone removed the newest records from the end, what remains would still check out. To catch that, every export, printout and backup shows the **chain head**, the fingerprint of the last record. Compare it with an older printout or backup: the older head must still be in the chain.
- **Limits are versioned.** A change of limit creates a new version with who, when and why. A limit less strict than the FSAI's needs a reference to the HACCP plan. Records already saved keep the result they had: changing a limit never re-grades the past.

## Checks logged later

Kitchens do not always log a check the moment it is done. Kishtin lets staff say when a check was done and by whom.

- Such a record shows at the time the check was done, with the name given. The printed booklets do not mark it as late.
- The record also keeps when it was saved and by whom. The record's details show this, and the CSV export has both times, so an inspector can see the difference.
- A time in the future is refused. Whether staff may log earlier checks at all, and whether anything may be added to a period a manager has already signed off, are the restaurant's permissions to set.
- **Missed checks are never filled in.** A fridge check that was due and has no reading prints as "missed".

## Showing records during an inspection

| What | Where | Gives |
|---|---|---|
| **The FSAI booklets** (SC1 to SC9) | HACCP › Record books, choose the month | The filled-in Safe Catering sheets as an A4 PDF, with missed checks marked, the manager's sign-offs, and the chain check on each page |
| **Inspection pack** | Exports › Downloads, or `kishtin haccp export --pdf` | A summary (records, failures, chain head, photo checks, limits in use), every failed and borderline record with its action, every record by day, and the limit versions used |
| **Records** | HACCP › Records | Every record, searchable by date, type and result, each with its evidence |
| **CSV** | Exports › Downloads | Every record in a spreadsheet, with record numbers, both times, the limit version and both hashes |
| **Verify** | Exports › Verify | That nothing has been changed, as above |
| **Allergen chart** | Setup › Kitchen › Allergens | Every dish against the 14 allergens, printable |

## What Kishtin does not do

- It does not know which days the restaurant was closed, so a fridge check on a closed day shows as missed. Note closed days in the manager's sign-off.
- It does not record the health questions or declaration for fitness to work: keep the signed form on paper.
- Some booklet columns it does not record, such as SC4 "Time into hot hold", print blank, and the booklet says which.
- It does not print onto the official PDF pages. It draws its own pages in the same layout.
- It records allergens as a manager enters them. It does not know recipes or suppliers' ingredients, or cross-contact in the kitchen.
- It has not been tested in a working kitchen or reviewed by the FSAI.

## Questions to ask your EHO

1. Would you accept these records in place of the paper Safe Catering booklets?
2. Do you want them printed each month, or is showing them on screen during a visit enough?
3. How often should a manager sign off the records: weekly, or monthly?
4. Is it acceptable that a check logged later prints at the time it was done, with the time it was saved kept in the record? Or should late entries be marked on the printout?
5. Are photos of supplier dockets on delivery records useful to you?
6. How long should we keep records?
7. Is the allergen chart enough for customers, alongside staff telling them?

## Before you go

1. Print last month's booklets from **Record books**, and an **inspection pack** for the same month.
2. Run **Verify** and note that it passes.
3. Print the **allergen chart**.
4. Bring this page.
