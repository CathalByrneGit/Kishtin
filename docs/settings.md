# Settings

Everything Kishtin checks HACCP records against can be changed by a manager. There are five parts:

| What | Where | Changed with |
|---|---|---|
| Critical limits, per dish, food category, fridge or check type | [haccp.md](haccp.md#limits) | `kishtin limits set` |
| Reference guidance | below | `kishtin settings set guidance` |
| Food-safety rules | below | `kishtin settings set rules` |
| Lists of fridges, freezers, food categories and cleaning tasks | below | `kishtin settings set lists` |
| System settings | below | `kishtin settings set system` |

Every change works the same way:

- **Show, then save.** `kishtin settings set` lists each field that would change, warns about anything less strict, and describes the effect on current limits and recent records. Then it asks before saving.
- **A reason every time.** A change that makes food safety less strict also needs `--plan-ref`: the HACCP plan section or validation that supports it.
- **Manager password.** Changes need a manager logged in with a password, or a manager API key. Any staff member can view settings.
- **Versioned.** A change saves a new version and keeps the old ones. `kishtin settings history GROUP` lists them. Records already saved keep their result.

To change a group, write out its current value, edit it, and set it:

```sh
kishtin settings show lists --value > lists.json
# edit lists.json
kishtin settings set lists -f lists.json --reason "Add the fish fridge"
```

The same actions are in the API under `/api/settings/{guidance,rules,lists,system}` (`GET`, `GET …/history`, `POST …/preview`, `PUT`).

## Reference guidance

The reference guidance is what your limits are measured against. It starts as the FSAI defaults. Each check type (fridge, freezer, cook, reheat, hot hold, cooling, chilled and frozen delivery) has an entry, with the same fields as a limit.

A **limit** looser than the guidance needs a plan reference. **Guidance** looser than the FSAI defaults needs a plan reference when you save it. So every loosening is tied to a plan reference at one level or the other.

Example: to treat 70°C for 2 minutes as equal to 75°C, add it to the cook guidance once, with your plan reference:

```json
{ "check": "cook", "combos": [ { "min_c": 75, "hold_seconds": 0 }, { "min_c": 70, "hold_seconds": 120 } ] }
```

After that, a cook limit with 70°C for 2 minutes (or hotter, or held longer) needs no plan reference. Previews still say when a limit is below the FSAI defaults, so nothing is hidden.

Each limit stores the guidance version it was checked against. Changing the guidance never changes existing limits or records. The preview lists limits that would become, or stop being, below the guidance.

The FSAI defaults themselves are constants in `internal/haccp/limits.go` and do not change through settings.

## Food-safety rules

| Rule | Default | Meaning |
|---|---|---|
| `borderline_needs_action` | `true` | A borderline reading (for example a chilled delivery at 6–8°C) needs a corrective action to be saved. Turning this off makes food safety less strict, so it needs a plan reference. |
| `listed_names_only` | `true` | When a list has entries, readings must use a name from it. |

One rule cannot be changed: **a failed reading always needs a corrective action.**

Each record stores the rules version in force when it was saved.

## Lists

```json
{
  "units": [
    { "name": "Walk-in", "kind": "fridge" },
    { "name": "Drinks fridge", "kind": "fridge", "non_food": true },
    { "name": "Chest freezer", "kind": "freezer" },
    { "name": "Bar fridge", "kind": "fridge", "checks": [
      { "label": "Opening", "from": "08:00" },
      { "label": "Mid-service", "from": "14:00" },
      { "label": "Close", "from": "21:00" }
    ] }
  ],
  "food_categories": ["poultry", "pork", "minced meat", "fish", "rice"],
  "cleaning_tasks": [ { "task": "Deep clean fryer", "area": "Fryer station" } ],
  "corrective_actions": [ { "type": "fridge", "text": "Moved food to a working unit" } ]
}
```

- The lists start with the prototype's: Walk-in fridge, Prep fridge and Chest freezer, its six daily cleaning tasks, and its quick-pick corrective actions.
- `checks` sets a unit's daily check times. Each runs from its `from` time (HH:MM) until the next one starts; the first also covers the start of the day. Leave it out for AM (until 12:00) and PM.
- `corrective_actions` are the quick-pick actions offered for each record type when a reading is not a pass. Staff can always write their own.
- An empty list accepts any name.
- Once a list has entries, a name not on it is refused (with `listed_names_only` on), and the error lists the names to choose from. Names match ignoring case, and the listed spelling is saved. This stops "Walkin" and "Walk-in" becoming two fridges with different limits.
- Fridge readings must name a fridge, and freezer readings a freezer.
- Limits for a fridge, freezer or food category must use a listed name too.
- A cleaning record gets its area from the list when none is given.
- The preview warns when recent records or current limits use names the new lists do not have.

Dishes are not listed yet; they come with the menu (roadmap stage 2).

## System settings

| Setting | Default | Meaning |
|---|---|---|
| `restaurant_name` | empty | Shown at the top of every screen and on the inspection pack. |
| `backup_interval_minutes` | 360 | How often to back up. 0 turns scheduled backups off; otherwise at least 15. |
| `backup_keep` | 28 | How many backups to keep. 0 keeps every backup. |
| `max_upload_mb` | 15 | Largest docket or invoice file accepted (1–100). |
| `preview_records` | 50 | How many recent records a limit preview re-scores (1–500). |

Until you save a change, these come from `kishtin.toml` (`[backup] interval` and `keep`, `[files] max_size_mb`). Once saved, the saved version wins. Changes take effect without a restart.

The backup folder, listen address and TLS stay in `kishtin.toml` only. They decide where files are written and who can connect, so they cannot be changed over the API.
