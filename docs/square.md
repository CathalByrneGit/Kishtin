# Orders from Square

Orders taken on the Square till come into the kitchen on their own: no re-typing at the pass.

Kishtin only **reads** from Square. It never changes or creates orders there, never touches payments, and never receives card details, so it stays out of card-data (PCI DSS) scope. Payments stay entirely in Square POS and Square Terminal.

## What happens to a till order

1. Every few seconds (5 by default) Kishtin asks Square for orders created or changed since it last asked.
2. A new order becomes a kitchen order:
   - A pickup or delivery order is a **takeaway**, named for the customer.
   - Otherwise the ticket name gives the **table**: "Table 4", "T4", "Tbl 4" or just "4". An order with no table in its ticket name ("Bar tab") becomes a takeaway with that name.
   - Each till item becomes the dish it is mapped to (see below), with its modifiers and notes. Drinks and anything else marked **not for the kitchen** are left out. An order with nothing for the kitchen does not become a kitchen order.
   - Its courses are **held** until the pass fires them, like any other order. The pass card shows a **Square** chip, and the order's history shows "Square" as who entered it.
3. When the order changes on the till:
   - More of an item, or a new item, is added to the kitchen order. If its course is already fired, it goes straight to its station.
   - Fewer of an item, a removed line, or a cancelled order is **never voided automatically**: the food may already be cooking. Kishtin adds a note to the order on the pass ("Changed on the till: Burger now 1, was 2. Void on the pass what is not wanted.") and to its history, and the pass decides.
4. Orders are deduped on Square's order ID and version, so an order is never entered twice, even when Square sends it again.

Some things the till allows are flagged rather than refused, because the order is already taken:

- An item that is not mapped yet is not cooked. It is listed on the order's notes ("From Square, not on the kitchen menu yet: 1 × Lamb special") and on the Square page. Once a manager maps it, it is added to the open order.
- A dish that is 86 or off the menu is not added; the order gets a note saying it was ordered on the till.
- A till modifier with no kitchen mapping reaches the cook as a note on the item.

## If Square or the internet is down

Manual orders keep working as normal. The Square page shows **Square did not answer** with the reason. Kishtin keeps asking (backing off up to every two minutes), and when Square answers it carries on from where it left off, so no order is missed.

## Setting it up

Everything is done on **Setup → Connections → Square**, by a manager logged in with the password. The page has the steps in order.

1. **Connect your Square account.** In the Square Developer Dashboard, create an application and copy an access token for the restaurant's account with permission to read orders and the item catalog (`ORDERS_READ`, `ITEMS_READ`). Kishtin needs nothing else. Choose **Production** (the real till) or **Sandbox** (Square's test account), paste the token and **Check and connect**. Kishtin sends the token to Square to check it and asks for your locations. If Square refuses it, nothing is saved. If Square accepts it, the token is kept in a file in the data folder that only the server can read (`connectors/square.json`), outside the database, so it is never in a backup or export.
2. **Map till items to dishes.** **Load from Square** (Kishtin asks first, and sends Square the token only), then map each till item to the dish it makes and each till modifier to a dish modifier (such as "Well done"). **Use suggestions** fills in dishes and modifiers with the same names. Mark drinks and anything else not cooked **Not for the kitchen**. **Save changes**.
3. **Turn import on.** Switch on **Import orders from Square**, choose the location from the list, and **Review changes**. The review says exactly what Kishtin will send to Square and what comes back. Save it with a reason. Turning import on is a versioned setting like any other, so it is recorded who turned it on, when and why.

Only orders changed on the till after import is turned on are imported. Earlier orders are not.

**Disconnect** deletes the saved token. Turn import off first. Mappings and orders already imported stay.

Instead of connecting on the screens, the token can be set on the server, in the environment (`KISHTIN_SQUARE_TOKEN`) or as `square.token` in `kishtin.toml`, with `square.environment = "sandbox"` for a sandbox account. A token set there wins, and the screens show it as set on the server and cannot change or disconnect it.

Connect from a device that reaches the server over HTTPS, or on the server itself, so the token does not cross the kitchen Wi-Fi in the clear.

## The Connections page

**Setup → Connections** lists what Kishtin can connect to (Square, for now): what each does, exactly what it sends, whether it is connected and in use, when it last worked and the last problem. **Set up** or **Open** goes to its page.

## The Square page

- **Connect your Square account**: connected or not, production or sandbox, where the token is kept, and **Check and connect** or **Disconnect**.
- **Import status**: whether the server has a Square token, whether import is on, the location, how often it asks, when it last worked, and the last problem.
- **Turn import on or off**: the switch, the location (chosen from your Square locations) and how often to ask, with **Review changes**.
- **Till items and the kitchen**: every item and modifier on the till, how it maps, and a suggestion by name.
- **Recent till orders**: the last 50 till orders seen, their state on the till and version, and anything that needs checking: items not mapped, and changes the pass must deal with.
- **Sync now** asks Square straight away (head chef or manager).

## Commands

```sh
kishtin connectors                        # what Kishtin can connect to, and how each stands
kishtin connectors connect square --environment sandbox --token-file token.txt
                                          # or --token-file - to read it from standard input
kishtin connectors disconnect square
kishtin square locations                  # the account's locations and their IDs
kishtin square status                     # on or off, last worked, recent till orders
kishtin square catalog                    # till items and modifiers, and their mapping
kishtin square map VAR_ID --dish Burger   # or --modifier "Well done", or --ignore
kishtin square sync                       # ask Square now
kishtin settings show square              # the import setting in force
kishtin settings set square -f square.json --reason "Go live"
```

`square.json` holds `{"enabled": true, "location_id": "L1234ABCD", "poll_seconds": 5}`. The command prints what will be sent to Square before saving.

The token is read from a file or standard input, never from the command line, so it does not end up in shell history.

API: `GET /api/connectors`, `PUT` and `DELETE /api/connectors/square`, `GET /api/square/locations`, `GET /api/square`, `GET /api/square/catalog`, `PUT /api/square/mappings`, `POST /api/square/sync`, and `/api/settings/square` for the setting.

## What it does not do yet

- Write ticket progress back to Square, so front of house sees it on the till.
- Webhooks. Kishtin asks Square instead, so the server never needs a public address.
- Sync the menu: dishes are set up in Kishtin and mapped to till items by hand.
- More than one Square location.
