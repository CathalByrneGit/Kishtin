// Package integrations connects Kishtin to point-of-sale systems. A POS
// adapter fetches the till's catalog and orders and returns them as the
// types here; core packages never see a POS's own API types. The
// translation from a POS order to a kitchen ticket is pure and lives here
// too, so it can be tested without a network.
package integrations

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// POSAdapter is a point-of-sale system Kishtin reads from.
type POSAdapter interface {
	// Name is the POS's name, such as "square".
	Name() string
	// Catalog returns the items and modifiers on the till.
	Catalog(ctx context.Context, locationID string) ([]CatalogEntry, error)
	// OrdersSince returns orders created or updated at or after since,
	// oldest change first. The same order can come back more than once,
	// with the same or a newer version.
	OrdersSince(ctx context.Context, locationID string, since time.Time) ([]TillOrder, error)
	// Locations lists the account's locations. It is also how a new
	// connection is checked.
	Locations(ctx context.Context) ([]Location, error)
}

// Location is one of the account's places of business.
type Location struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Active is false for a location closed on the till.
	Active bool `json:"active"`
}

// EntryKind says what a catalog entry is.
type EntryKind string

const (
	// EntryItem is something sold: a Square item variation, such as
	// "Burger (Regular)".
	EntryItem EntryKind = "item"
	// EntryModifier is a modifier, such as "Well done".
	EntryModifier EntryKind = "modifier"
)

// CatalogEntry is one item or modifier on the till.
type CatalogEntry struct {
	ID   string    `json:"id"`
	Kind EntryKind `json:"kind"`
	Name string    `json:"name"`
	// Group is the item a variation belongs to, or the modifier list a
	// modifier belongs to.
	Group string `json:"group,omitempty"`
}

// Label names an entry the way staff would: "Burger (Regular)".
func (e CatalogEntry) Label() string {
	if e.Group == "" || strings.EqualFold(e.Group, e.Name) {
		return e.Name
	}
	if e.Kind == EntryItem {
		if strings.EqualFold(e.Name, "regular") {
			return e.Group
		}
		return e.Group + " (" + e.Name + ")"
	}
	return e.Name
}

// OrderState is where a POS order is.
type OrderState string

const (
	OrderOpen      OrderState = "open"
	OrderCompleted OrderState = "completed"
	OrderCanceled  OrderState = "canceled"
)

// Order is an order from the till.
type TillOrder struct {
	ID string `json:"id"`
	// Version goes up every time the order changes on the till.
	Version int64      `json:"version"`
	State   OrderState `json:"state"`
	// Ticket is the ticket name the till gave the order, such as "Table 4".
	Ticket string `json:"ticket,omitempty"`
	// Takeaway is set for pickup and delivery orders.
	Takeaway bool `json:"takeaway,omitempty"`
	// Customer is the pickup or delivery customer's name.
	Customer  string     `json:"customer,omitempty"`
	Lines     []TillLine `json:"lines"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Line is one line of a till order: "2 × Burger, Well done".
type TillLine struct {
	// UID identifies the line within the order, across versions.
	UID string `json:"uid"`
	// CatalogID is the catalog item (variation) sold. Empty for a custom
	// amount.
	CatalogID string `json:"catalog_id,omitempty"`
	Name      string `json:"name"`
	// Quantity is how many; fractional quantities (sold by weight) are
	// rounded up.
	Quantity  int            `json:"quantity"`
	Modifiers []TillModifier `json:"modifiers,omitempty"`
	Note      string         `json:"note,omitempty"`
}

// Modifier is a modifier on a line.
type TillModifier struct {
	CatalogID string `json:"catalog_id,omitempty"`
	Name      string `json:"name"`
}

// Mapping links a till catalog entry to the kitchen: an item to a dish, a
// modifier to one of that dish's modifiers, or either to nothing ("not
// for the kitchen", such as drinks from the bar).
type Mapping struct {
	POS        string    `json:"pos"`
	ExternalID string    `json:"external_id"`
	Kind       EntryKind `json:"kind"`
	// Name is the till's name for it when the mapping was saved.
	Name string `json:"name"`
	// DishID is the dish an item makes.
	DishID string `json:"dish_id,omitempty"`
	// Modifier is the dish modifier a till modifier means, by name.
	Modifier string `json:"modifier,omitempty"`
	// Ignore marks an entry that is not for the kitchen.
	Ignore    bool      `json:"ignore,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

// Validate checks a mapping's own fields.
func (m Mapping) Validate() error {
	switch {
	case m.ExternalID == "":
		return errors.New("a mapping needs the till's catalog ID")
	case m.Kind != EntryItem && m.Kind != EntryModifier:
		return fmt.Errorf("kind must be item or modifier, not %q", m.Kind)
	case m.Ignore && (m.DishID != "" || m.Modifier != ""):
		return errors.New("a mapping is either not for the kitchen or names what it makes, not both")
	case !m.Ignore && m.Kind == EntryItem && m.DishID == "":
		return errors.New("choose the dish this item makes, or mark it not for the kitchen")
	case !m.Ignore && m.Kind == EntryModifier && m.Modifier == "":
		return errors.New("choose the modifier this means, or mark it not for the kitchen")
	}
	return nil
}

// POSSettings is how Kishtin uses the POS. Turning sync on is a versioned,
// audited change made by a manager after seeing what is sent.
type POSSettings struct {
	// Enabled turns order import on.
	Enabled bool `json:"enabled"`
	// LocationID is the till location whose orders to import.
	LocationID string `json:"location_id"`
	// PollSeconds is how often to ask for new and changed orders.
	PollSeconds int `json:"poll_seconds"`
}

// DefaultPOSSettings is off.
func DefaultPOSSettings() POSSettings { return POSSettings{PollSeconds: 5} }

// Normalize trims the location.
func (s POSSettings) Normalize() POSSettings {
	s.LocationID = strings.TrimSpace(s.LocationID)
	return s
}

// Validate checks the settings.
func (s POSSettings) Validate() error {
	if s.PollSeconds < 2 || s.PollSeconds > 300 {
		return errors.New("poll_seconds must be between 2 and 300")
	}
	if s.Enabled && s.LocationID == "" {
		return errors.New("set the location ID before turning order import on")
	}
	return nil
}

// Ticket is a till order turned into what the kitchen cooks.
type Ticket struct {
	Takeaway bool
	// Table is set for a table order.
	Table string
	// Name is the takeaway customer's name.
	Name  string
	Lines []TicketLine
	// Unmapped lists till lines the kitchen does not know yet, by name,
	// such as "2 × Lamb special". They are not cooked until mapped.
	Unmapped []string
}

// TicketLine is a dish to add for one till line.
type TicketLine struct {
	LineUID   string
	DishID    string
	Modifiers []string
	Notes     string
	Quantity  int
}

// Translate turns a till order into a ticket, using the mappings by till
// catalog ID. already is how many of each line were added before (by line
// UID), so only new quantity is returned. reduced lists lines with less
// than before, which the pass must deal with by hand: Kishtin never voids
// food on its own.
func Translate(o TillOrder, mappings map[string]Mapping, already map[string]int) (t Ticket, reduced []string) {
	t.Takeaway = o.Takeaway
	if o.Takeaway {
		t.Name = o.Customer
		if t.Name == "" {
			t.Name = o.Ticket
		}
	} else if table, ok := TableName(o.Ticket); ok {
		t.Table = table
	} else {
		// No table to be found: treat it as a takeaway named by its ticket.
		t.Takeaway, t.Name = true, o.Ticket
	}
	seen := map[string]bool{}
	for _, l := range o.Lines {
		seen[l.UID] = true
		before := already[l.UID]
		if l.Quantity < before {
			reduced = append(reduced, fmt.Sprintf("%s now %d, was %d", l.Name, l.Quantity, before))
			continue
		}
		m, ok := mappings[l.CatalogID]
		switch {
		case ok && m.Ignore:
			continue
		case !ok || m.DishID == "":
			if l.Quantity > before {
				t.Unmapped = append(t.Unmapped, fmt.Sprintf("%d × %s", l.Quantity-before, l.Name))
			}
			continue
		}
		if l.Quantity == before {
			continue
		}
		tl := TicketLine{LineUID: l.UID, DishID: m.DishID, Notes: strings.TrimSpace(l.Note), Quantity: l.Quantity - before}
		var unknownMods []string
		for _, mod := range l.Modifiers {
			mm, ok := mappings[mod.CatalogID]
			switch {
			case ok && mm.Ignore:
			case ok && mm.Modifier != "":
				if !slices.Contains(tl.Modifiers, mm.Modifier) {
					tl.Modifiers = append(tl.Modifiers, mm.Modifier)
				}
			default:
				unknownMods = append(unknownMods, mod.Name)
			}
		}
		// A modifier the kitchen has no timing for still reaches the
		// cook, as a note.
		if len(unknownMods) > 0 {
			extra := strings.Join(unknownMods, ", ")
			if tl.Notes != "" {
				tl.Notes = extra + "; " + tl.Notes
			} else {
				tl.Notes = extra
			}
		}
		t.Lines = append(t.Lines, tl)
	}
	for uid, n := range already {
		if !seen[uid] && n > 0 {
			reduced = append(reduced, fmt.Sprintf("a line removed on the till (%d added before)", n))
		}
	}
	slices.Sort(reduced)
	return t, reduced
}

// TableName reads a table from a till ticket name: "Table 4", "T4",
// "Tbl 12", or a bare "4".
func TableName(ticket string) (string, bool) {
	s := strings.TrimSpace(ticket)
	low := strings.ToLower(s)
	for _, p := range []string{"table", "tbl", "t"} {
		if strings.HasPrefix(low, p) {
			rest := strings.TrimSpace(strings.TrimLeft(s[len(p):], " .#:-"))
			if rest != "" && rest[0] >= '0' && rest[0] <= '9' && len(rest) <= 20 && !strings.Contains(rest, " ") {
				return rest, true
			}
		}
	}
	if s != "" && len(s) <= 6 && s[0] >= '0' && s[0] <= '9' && !strings.Contains(s, " ") {
		return s, true
	}
	return "", false
}

// State is where order import has got to.
type SyncState struct {
	POS string `json:"pos"`
	// Since is the update time to ask for orders from next.
	Since      time.Time  `json:"since"`
	LastPollAt *time.Time `json:"last_poll_at,omitempty"`
	LastOKAt   *time.Time `json:"last_ok_at,omitempty"`
	// LastError is the last poll's problem, empty when it worked.
	LastError string `json:"last_error,omitempty"`
}

// Imported is a till order Kishtin has seen, and what it did with it.
// Kishtin dedupes on the till's order ID and version, so an order is never
// entered twice.
type Imported struct {
	POS        string     `json:"pos"`
	ExternalID string     `json:"external_id"`
	Version    int64      `json:"version"`
	State      OrderState `json:"state"`
	// OrderID is the kitchen order, empty while nothing on it is for the
	// kitchen.
	OrderID string `json:"order_id,omitempty"`
	// Label is how the kitchen names it: "Table 4".
	Label string `json:"label"`
	// Added is how many of each till line went to the kitchen, by line UID.
	Added map[string]int `json:"added"`
	// Unmapped lists till lines not yet mapped to a dish.
	Unmapped []string `json:"unmapped"`
	// Notices lists changes on the till the pass must deal with by hand,
	// such as a line removed after it was sent to the kitchen.
	Notices []string `json:"notices"`
	// Payload is the till order as last seen, so it can be imported again
	// when an item is mapped, without asking the till.
	Payload   TillOrder `json:"payload"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}
