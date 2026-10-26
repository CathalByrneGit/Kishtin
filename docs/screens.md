# Screens

`kishtind` serves the screens at its address (for example `http://kitchen-pc.local:8080/`). The layout is built for a computer screen: a sidebar on the left, a top bar with search and **Log check**, and tables with a details panel. The theme is light and high contrast by default. Every status shows as text plus colour.

Each device can instead show a full-screen kitchen layout: a **pass screen**, a **station screen** or a **phone** layout. See [Kitchen screens](#kitchen-screens). On the desktop layout below 900 px wide, the sidebar becomes a menu button and pages stack into one column.

## Pairing a device and logging in

1. A manager makes a device key in **Setup → Device keys** (or with `kishtin keys create --kind device --name "Office PC"`). The pairing link is shown once.
2. Open the link once on the computer, tablet or phone. The browser remembers the key. You can also paste the key on the setup screen.
3. Staff pick their name and enter their PIN. A PIN login lasts 12 hours. The exit icon at the bottom of the sidebar switches staff.

To take a device out of use, revoke its key in **Device keys**, or use **Unpair this device** in **Settings**.

## Kitchen screens

Choose what a device shows in **Settings → This device → This device shows**, or from the menu (☰) at the top right of a kitchen screen:

| Layout | For | Shows |
|---|---|---|
| Desktop, with the sidebar | the office computer | the full app, described below |
| Pass screen | a wall-mounted tablet or TV at the pass | every open order in large type, with Fire, Serve, Mark ready, Change time, Hold again and Urgent for the head chef or a manager |
| Station screen | a tablet at one station | that station's all-day counts, its tickets with a button per item, and recently done items to undo |
| Phone | a phone | four tabs: Today (fridge and cleaning checks), Log check, New order (for staff who take orders), and the Pass |

- The choice is kept in that device's browser only. A device with a phone-sized screen starts on the phone layout; anything else starts on the desktop.
- Kitchen screens have no sidebar: a top bar with what the screen is, the restaurant, a large clock (pass and station), the connection, who is logged in, and the menu. The menu switches staff, changes the screen, and sets the theme and high contrast.
- Under the top bar, **Hold to talk** runs voice commands, and the keyboard button opens a field to type one. The reply shows below with what was heard, and a question shows its answers and **Confirm** or **Cancel** as buttons. Holding the space bar works like holding the button. **Hands-free: listen for "Kishtin"** turns on hands-free listening on that screen, with a "Listening for Kishtin" chip while it is on. See [voice.md](voice.md).
- Every button, field and tab on a kitchen screen is at least 56 px tall, and type is larger, so they work with wet or gloved hands and read at arm's length.
- The station screen's columns wrap on a portrait tablet: Start now and In progress side by side, Coming up below.
- Kitchen screens keep the screen awake where the browser allows it, show the last known state with a "Not connected" notice if the server drops, and update after every change on any device.
- A PIN login lasts 12 hours, so a shared pass or station screen asks for a PIN again after a long shift. Records, exports, verify and setup are on the desktop layout only.

## Layout

- **Sidebar**: Service (Pass, Stations, Orders, Voice); HACCP (Today, Records, Record books, Exports, Verify); Setup (Menu, Scheduler, Connections, Fridges and tasks, Limits and rules, HACCP records, Staff, Device keys, Backups); and Settings at the bottom. The Pass is the first page after logging in. The footer shows who is logged in and has the theme and switch-staff buttons. The collapse button shrinks the sidebar to icons, and the browser remembers that choice.
- **Top bar**: where you are, a search box, the connection, **New order** and **Log check**.
- **Search** (Ctrl K, or ⌘K on a Mac): jump to any page, start a new order, or start logging a check.
- **Log check** (Ctrl L, or ⌘L): opens the log form in a panel on the right.

The connection chip says:

- **Synced**: connected. New records from any device appear straight away.
- **Connecting…**: reconnecting.
- **Not saving**: the server cannot be reached, or the last save failed. A "Not connected" notice says when the data on screen was last updated. Records cannot be saved until the connection is back.

Each page has its own address, such as `/records?range=31&id=…`, so you can bookmark it and the back button works.

## Service

How the kitchen side works, and how the scheduler decides, is in [kitchen.md](kitchen.md). The Pass and Stations refresh after every change on any device and every 15 seconds, and their countdowns tick every second.

### Pass

Every open order as a card, for the head chef or expo:

- **Figures**: open orders, and how many fired courses are on track, at risk, late, or ready to serve, and how many courses are held.
- **All day**: every dish not yet served, across the kitchen, with its variations (regular, well done, notes) and how many are to start, cooking, ready and held. Tap a dish to show only the orders with it.
- **Show**: all orders, only those that need action (at risk, late or ready), or those with held courses.
- Each card's heading is the order's name if it has one (with the table or takeaway number under it), otherwise its table or takeaway number. It shows the ticket number, time entered, covers, notes, a **Square** chip for orders from the till, and an **Allergy** chip and red edge when flagged. Each course shows its state and the scheduler's status (on track, at risk, late, ready) with the reason, the target time with a countdown, and its items grouped as "2× Burger (Well done)" with their state.
- Head chefs and managers get the buttons: **Fire** a held course; **Serve** a course once every item is ready; **Change time** (a new target, with a reason for the log); **Hold again** on a fired course while nothing in it has started; **Mark ready** once every item in a course has been started, when the plates are at the pass; **Urgent**; and **Close order** once everything is served or voided. Other staff see the same cards without buttons.
- The history icon opens the order: its courses and items, its full history (who did what, when), **Add items**, **Hold again**, **Add a name** or **Change name**, and on each item its next step (**Start**, **Done cooking** and so on) and **Change**. Change shows the item as an editable line (dish, modifiers, notes) and an optional reason; **Replace item** voids the old one and adds the new one in its place.

### Stations

Pick a station. Its screen has three parts:

- **All day**: how many of each dish the station has in hand, counted once per dish, with its variations in small writing underneath and where they are: "5 Burger · 3 regular · 1 well done · 1 well done, no onion · 2 to start, 3 cooking". An item with a note counts as its own variation. Tap a dish to show only the tickets with it (and again to show them all). When the scheduler says to start several together, for different tables, **Start 3 regular** starts them in one tap.
- **Tickets**: one per order and course with work for this station, the one that needs a hand soonest first. The heading is the order's name, or its table or takeaway number, with the course, the time since it was fired, and its status (on track, at risk, late) as a coloured top edge and a chip. Each item is its own line with its own button: **Start**, **Done cooking**, **Done resting** or **Ready**. A line says **Start now**, **Start 18:42**, or how long is left on what it is doing (red once overdue). The i button shows why the scheduler chose that time. Items already ready are struck through until the whole ticket is done. Allergies show in red at the top.
- **Recently done**: items made ready here in the last 15 minutes, with **Undo** to put one back a step.

Line cooks, head chefs and managers get the buttons.

Each button sends the state the cook saw. If someone else has already moved those items, or the screen was out of date, nothing moves: the screen says who moved them and when, for example "Synthetic burger for Table 4 is already cooking (Liam Cook, 18:41)", and reloads. A double tap cannot move an item two steps.

### Orders

- **New order** (front of house, head chefs and managers): table or takeaway, table number, an optional name (such as the guest's, shown instead of the table), covers, an allergy tick and notes. Tap dishes to add them, grouped by course; dishes that are 86 cannot be added. Each line has a quantity, its course, modifier chips and notes. Head chefs and managers can tick courses to **fire straight away**. **Send order** shows the ticket number.
- **Today's orders**: open orders, or every order today, sortable. Select one to open it, with its history, **Add items** and **Change**.

### Voice

- **Try a command**: **Hold to talk** and a field to type a command, with the reply below.
- **Set up**: whether spoken commands (speech-to-text) and spoken replies (text-to-speech) are set up on the server, whether this device's microphone can be used, and **Speak replies on this device**.
- **What you can say**: every command, with what it does.
- **Recent commands**: the last 50, newest first: when, who, what was heard (and whether it was typed), the outcome (done, answered, asked, not done, not understood, cancelled) and the reply.

## Today

- **Figures**: checks done out of due, out-of-limit readings today, how many of those have no corrective action, cleaning tasks done, and batches in use.
- **Fridge and freezer temperatures**: one row per unit, with its limit and a column for each check time. Each check shows the reading (green pass, amber borderline, red fail), **Due**, or amber **Missed** once the next check has started. **Log** opens the log form with that unit chosen.
- **Daily cleaning**: tick a task to record it as done, with your name and the time. Each task can be done once a day.
- **Needs attention**: today's failed and borderline records, why, and the corrective action. Select a record to open it in Records.
- **Batches in use**: batch labels with a use-by date from three days ago onwards: Expired, Use today, or OK.

Check times are set per unit in **Fridges and tasks**. By default each unit has an AM check (until 12:00) and a PM check.

## Log check

Choose a type: Fridge / freezer, Delivery, Cook / reheat, Cooling, Hot holding, Batch label or Cleaning. As you type, a box shows the result the server will record (pass, borderline or fail) and which limit it used.

- **Delivery**: tick **Packaging intact** and **In date**. Leaving either unticked fails the delivery. **Add docket photo** attaches a photo or PDF.
- **Cooling**: enter when cooking finished and when the food went into the fridge (**Now** fills in the time). A fridge time earlier than the cooking time counts as over midnight. If more than 2 hours passed, enter the core temperature at 2.5 hours.
- **Batch label**: the use-by date is the made-on date plus the shelf life in days (5 by default).

**When was this done?** › **Earlier** logs a check done earlier, with its date and time and, if it was someone else, who did it. When correcting a record, the correction keeps the original's time unless you choose **A different time**.

If the result is not a pass, a corrective action is required. Pick a quick-pick action or write your own. **Save record** stays off until there is a result and, if needed, an action.

## Records

A table of every record, newest first. You can:

- Search by fridge, food, supplier, staff member or action.
- Filter by Today, 7 days, 31 days or All, and by type, or show only borderline and failed records.
- Sort by any column.

Select a record to open its details panel:

- The reading, the result and why, the corrective action, and where the limit came from.
- The docket photo, if there is one.
- The evidence: record ID, hash, previous hash, limit version and rules version, each with a copy button.

**Correct this record** opens the log form filled in from the record, with a required reason. Saving adds a new record that points to the original. The original never changes; the table marks it **Corrected** and the new one **Correction**.

## Record books

Head chefs and managers only. Choose a month, check each booklet's missed checks and preview, then download the PDF. See [record-books.md](record-books.md).

## Catch up

![Catch up](screenshots/catch-up.png)

Every fridge and freezer check and cleaning task that was due and has no record, a card per day, by default for the last 14 days (choose **From** and **To** for up to 62). Type the temperature, or tick **Done** for a cleaning task, and the time it was done; the time starts at the start of the check's window. **Done by** names who did them, if not you. Leave checks that were not done blank: Kishtin never fills them in.

**Check entries** shows each result. A reading out of limit needs the corrective action that was taken. **Save** adds every entry, or none if any cannot be saved. A day already signed off is marked; adding to it needs permission (Setup › Permissions).

## Paper import

Head chefs and managers only. Add a photo of a paper record book page, type its rows (or let a local model read them), check them, then save them all at once. See [record-books.md](record-books.md#bringing-in-paper-records).

## Exports

Head chefs and managers only.

1. Choose a date range (or every record), a record type, and whether to include only borderline and failed records.
2. Check the preview: how many records match, failed, borderline, without an action, and a count per type.
3. Download one of these:
   - **Inspection pack (PDF)**.
   - **Records (CSV)**.
   - **Limit history (CSV)**: every limit version ever saved.

After a download, **Last export** shows how many records the file has, whether the hash chain verified, and the chain head. Keep the chain head with the file.

The preview counts at most 1000 records. The export itself has no limit.

## Verify

Re-checks three things:

- The hash chain from the first record.
- Every attached file against its SHA-256.
- That every limit version a record points to exists.

It runs when you open the page, and again with **Check again**. If something does not match, the page shows the first break and links to that record. It also says what verify cannot catch: records removed from the end of the chain. See [haccp.md](haccp.md#records-cannot-be-changed).

## Setup

Everyone can see **Menu**, **Scheduler**, **Fridges and tasks**, **Limits and rules** and **Permissions**. Changing them, and opening **Staff**, **Device keys** and **Backups** at all, needs a manager logged in with their password, unless a manager has given that job to others in **Permissions**. Use **Password login** on the page. That logs this device in with a password for 30 minutes.

- **Menu**: a card per station with its equipment and how many each holds, and every dish in a sortable table with its course, station, equipment and capacity, cook time, total time, how long it can wait, and status (Available, 86, off the menu). **Add station**, **Add equipment**, **Add dish** and **Edit** need the manager password. The dish form takes times in minutes or minutes:seconds, the food category from the HACCP lists, and modifiers with their extra cook minutes. **86** and **Back on** are open to head chefs, line cooks and managers.
- **Connections**: what Kishtin can connect to (Square, for now), what each sends, and whether it is connected and in use. **Set up** opens the connector's page. **Square** has its setup in three steps: connect the account (paste the access token, checked with Square before it is saved), map till items to dishes, and turn import on (with a review of exactly what is sent to Square); then recent till orders with anything to check. See [square.md](square.md).
- **Scheduler**: the strategy, batching, priorities, how long a ready course can wait at the pass, how often screens re-plan, and a buffer per station. **Review changes** shows what changes before saving with a reason. A card lists what the scheduler does not do yet.
- **Fridges and tasks**: fridges and freezers with their check times, and which fridges are non-food; daily cleaning tasks and their areas; food categories; quick-pick corrective actions per record type. Edit freely, then **Review changes** shows what changes and what it affects, and asks for a reason before saving.
- **Limits and rules**: the limits in use, and the full history with who changed each one and why.
  - **Change** or **New limit** opens the limit form. It covers fridge and freezer maximums, delivery borderlines, hot-holding minimums, cooking time–temperature combinations and cooling times, for a dish, food category, unit or every one.
  - **Preview the change** shows what it replaces, any warning that it is below your guidance or the FSAI defaults, and how recent records would have scored under it.
  - A reason is always needed. A HACCP plan reference is needed when the limit is less strict than your guidance.
  - The food-safety rules are toggles, reviewed the same way.
  - The reference guidance is changed with `kishtin settings set guidance`.
- **Permissions**: who may do what, per action: the roles and named staff allowed, and whether a PIN is enough. Managers may always do everything; staff, keys, permissions and fitness to work stay managers only. See [permissions.md](permissions.md).
- **Staff**: add staff with a role and a PIN (managers also get a password of at least 10 characters), change roles, change a PIN or password, and deactivate or reactivate. Staff are never deleted, because records name who logged them.
- **Device keys**: make device keys and staff keys for the CLI. The key and pairing link are shown once. You can revoke a key.
- **Backups**: the latest backup, the schedule, and every backup on the server with its record count, size, verify result and chain head. **Back up now** makes one straight away.

## Settings

- **Restaurant**: the name, backup schedule and retention, largest upload, and preview length. These change through the same review as Setup. See [settings.md](settings.md#system-settings).
- **This device**: which screen it shows (see [Kitchen screens](#kitchen-screens)), light, dark or system theme, high contrast, and unpairing.

## Screenshots

All taken with synthetic data, at 1440 × 960 unless noted.

| Pass | Stations: the grill |
|---|---|
| ![Pass](screenshots/pass.png) | ![Station](screenshots/station.png) |
| **Station with burgers cooking** | **Order history** |
| ![Station cooking](screenshots/station-cooking.png) | ![Order history](screenshots/order-history.png) |
| **New order** | **Menu** |
| ![New order](screenshots/new-order.png) | ![Menu](screenshots/menu.png) |
| **Changing a dish** | **Scheduler** |
| ![Dish editor](screenshots/dish-editor.png) | ![Scheduler](screenshots/scheduler.png) |
| **Today** | **Records, with a record selected** |
| ![Today](screenshots/today.png) | ![Records](screenshots/records.png) |
| **Log check** | **Correcting a record** |
| ![Log check](screenshots/log-check.png) | ![Correcting a record](screenshots/correct.png) |
| **Search** | **Verify** |
| ![Search](screenshots/palette.png) | ![Verify](screenshots/verify.png) |
| **Exports** | **Limits and rules** |
| ![Exports](screenshots/exports.png) | ![Limits](screenshots/limits.png) |
| **Changing a limit** | **Fridges and tasks** |
| ![Changing a limit](screenshots/limit-preview.png) | ![Fridges and tasks](screenshots/lists.png) |
| **Staff** | **Device keys** |
| ![Staff](screenshots/staff.png) | ![Device keys](screenshots/keys.png) |
| **Backups** | **Settings** |
| ![Backups](screenshots/backups.png) | ![Settings](screenshots/settings.png) |
| **Voice** | **Connections** |
| ![Voice](screenshots/voice.png) | ![Connections](screenshots/connections.png) |
| **Square** | |
| ![Square](screenshots/square.png) | |

### Kitchen screens

| Pass screen on a landscape tablet (1280 × 800) | Station screen on a portrait tablet (800 × 1280) |
|---|---|
| ![Pass screen](screenshots/pass-screen.png) | ![Station screen](screenshots/station-screen.png) |

A voice question waiting for "confirm" on the pass screen:

![Voice confirm](screenshots/voice-confirm.png)

The pass screen on a TV (1920 × 1080):

![Pass screen on a TV](screenshots/pass-tv.png)

| Phone: Today | Phone: Log check | Phone: New order | Phone: Pass |
|---|---|---|---|
| ![Phone Today](screenshots/phone-today.png) | ![Phone Log check](screenshots/phone-log.png) | ![Phone New order](screenshots/phone-order.png) | ![Phone Pass](screenshots/phone-pass.png) |
