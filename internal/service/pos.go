package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v7"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// Square is how Kishtin imports orders from Square: whether it is on, the
// location, and how often it asks.
func (s *Service) Square() *Group[integrations.POSSettings] { return s.tillGroup(tills[0]) }

// SumUp is how Kishtin imports paid sales from SumUp: whether it is on,
// the merchant account, and how often it asks.
func (s *Service) SumUp() *Group[integrations.POSSettings] { return s.tillGroup(tills[1]) }

// tillGroup is a till's import settings. Turning import on is a manager's
// audited consent to the requests described in the preview.
func (s *Service) tillGroup(t till) *Group[integrations.POSSettings] {
	return &Group[integrations.POSSettings]{s: s, spec: groupSpec[integrations.POSSettings]{
		kind:       t.kind,
		builtinID:  t.builtin,
		seeded:     true,
		builtin:    func(*Service) integrations.POSSettings { return integrations.DefaultPOSSettings() },
		normalize:  integrations.POSSettings.Normalize,
		validate:   integrations.POSSettings.Validate,
		lessStrict: func(_, _ integrations.POSSettings) []string { return nil },
		notes:      tillNotes(t),
		edit:       auth.ActManageSettings,
	}}
}

// tillNotes says exactly what turning import on sends, before it is saved.
func tillNotes(t till) func(context.Context, *Service, store.Store, integrations.POSSettings, integrations.POSSettings) ([]string, error) {
	return func(_ context.Context, s *Service, _ store.Store, cur, next integrations.POSSettings) ([]string, error) {
		var out []string
		switch {
		case next.Enabled && !cur.Enabled:
			out = append(out, t.sends(next), t.returns, t.startsAt,
				"Imported courses are held until the pass fires them. Items not mapped to a dish are listed, not cooked.")
		case !next.Enabled && cur.Enabled:
			out = append(out, fmt.Sprintf("Kishtin stops asking %s for orders. Orders already imported stay as they are.", t.name))
		case next.Enabled:
			out = append(out, fmt.Sprintf("Kishtin keeps asking %s for orders every %d seconds.", t.name, next.PollSeconds))
		}
		if s.adapter(t.id) == nil {
			out = append(out, fmt.Sprintf("%s is not connected yet, so nothing will be imported until a manager connects it.", t.name))
		}
		return out, nil
	}
}

// TillStatus is how order import from a till stands.
type TillStatus struct {
	// Till is the till's connector ID: square or sumup.
	Till string `json:"till"`
	// Connected is set when the server has a token for the till.
	Connected bool                                `json:"connected"`
	Settings  Versioned[integrations.POSSettings] `json:"settings"`
	State     *integrations.SyncState             `json:"state,omitempty"`
	// Recent is the most recently seen till orders.
	Recent   []integrations.Imported `json:"recent"`
	Mappings []integrations.Mapping  `json:"mappings"`
}

// TillStatus shows whether import is on, when it last worked, and the
// most recent till orders with anything the pass must check.
func (s *Service) TillStatus(ctx context.Context, p *auth.Principal, id string) (TillStatus, error) {
	if err := s.allow(p, auth.ActViewKitchen); err != nil {
		return TillStatus{}, err
	}
	t, err := tillByID(id)
	if err != nil {
		return TillStatus{}, err
	}
	set, err := s.tillGroup(t).current(ctx, s.st)
	if err != nil {
		return TillStatus{}, err
	}
	out := TillStatus{Till: t.id, Connected: s.adapter(t.id) != nil, Settings: set}
	if st, err := s.st.POS().GetPOSState(ctx, t.id); err == nil {
		out.State = &st
	} else if !errors.Is(err, store.ErrNotFound) {
		return TillStatus{}, err
	}
	if out.Recent, err = s.st.POS().ListImported(ctx, t.id, 50); err != nil {
		return TillStatus{}, err
	}
	if out.Mappings, err = s.st.POS().ListMappings(ctx, t.id); err != nil {
		return TillStatus{}, err
	}
	return out, nil
}

// CatalogRow is one entry on the till with how the kitchen maps it.
type CatalogRow struct {
	Entry integrations.CatalogEntry `json:"entry"`
	Label string                    `json:"label"`
	// Mapping is the saved mapping, if any.
	Mapping *integrations.Mapping `json:"mapping,omitempty"`
	// Suggested is a dish (for an item) or modifier (for a modifier) with
	// the same name, to save typing. It is never saved on its own.
	Suggested string `json:"suggested,omitempty"`
}

// TillCatalog reads the catalog from a till, with the saved mappings and
// suggestions by name. It sends the till only its token and location. A
// manager asks for it from the till's page, after reading what is sent.
// SumUp has no product list in its API, so its catalog is the items sold
// in recent sales.
func (s *Service) TillCatalog(ctx context.Context, p *auth.Principal, id string) ([]CatalogRow, error) {
	if err := s.allow(p, auth.ActManageMenu); err != nil {
		return nil, err
	}
	t, err := tillByID(id)
	if err != nil {
		return nil, err
	}
	pos := s.adapter(t.id)
	if pos == nil {
		return nil, notConnected(t)
	}
	set, err := s.tillGroup(t).current(ctx, s.st)
	if err != nil {
		return nil, err
	}
	entries, err := pos.Catalog(ctx, set.Value.LocationID)
	if err != nil {
		return nil, newErr(KindConflict, "Could not read the catalog: %s", err)
	}
	maps, err := s.st.POS().ListMappings(ctx, t.id)
	if err != nil {
		return nil, err
	}
	byID := map[string]integrations.Mapping{}
	for _, m := range maps {
		byID[m.ExternalID] = m
	}
	setup, err := menuSetup(ctx, s.st)
	if err != nil {
		return nil, err
	}
	out := make([]CatalogRow, 0, len(entries))
	for _, e := range entries {
		row := CatalogRow{Entry: e, Label: e.Label()}
		if m, ok := byID[e.ID]; ok {
			row.Mapping = &m
		}
		row.Suggested = suggest(setup, e)
		out = append(out, row)
	}
	slices.SortStableFunc(out, func(a, b CatalogRow) int {
		if a.Entry.Kind != b.Entry.Kind {
			return strings.Compare(string(a.Entry.Kind), string(b.Entry.Kind))
		}
		return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label))
	})
	return out, nil
}

// suggest finds a dish or modifier with the same name as a catalog entry.
func suggest(setup menu.Setup, e integrations.CatalogEntry) string {
	if e.Kind == integrations.EntryItem {
		for _, name := range []string{e.Label(), e.Group, e.Name} {
			for _, d := range setup.Dishes {
				if d.Active && strings.EqualFold(d.Name, name) {
					return d.ID
				}
			}
		}
		return ""
	}
	for _, d := range setup.Dishes {
		for _, m := range d.Modifiers {
			if strings.EqualFold(m.Name, e.Name) {
				return m.Name
			}
		}
	}
	return ""
}

// MappingInput maps one till catalog entry.
type MappingInput struct {
	ExternalID string                 `json:"external_id"`
	Kind       integrations.EntryKind `json:"kind" enum:"item,modifier"`
	// Name is the till's name, for the record.
	Name     string `json:"name"`
	DishID   string `json:"dish_id,omitempty"`
	Modifier string `json:"modifier,omitempty"`
	// Ignore marks it not for the kitchen, such as drinks from the bar.
	Ignore bool `json:"ignore,omitempty"`
}

// SaveTillMappings saves how till items and modifiers map to the kitchen.
// Open till orders with items that are now mapped get those items added.
func (s *Service) SaveTillMappings(ctx context.Context, p *auth.Principal, id string, in []MappingInput) ([]integrations.Mapping, error) {
	if err := s.allow(p, auth.ActManageMenu); err != nil {
		return nil, err
	}
	t, err := tillByID(id)
	if err != nil {
		return nil, err
	}
	if len(in) == 0 {
		return nil, newErr(KindInvalid, "send at least one mapping")
	}
	err = s.st.WithTx(ctx, func(tx store.Store) error {
		setup, err := menuSetup(ctx, tx)
		if err != nil {
			return err
		}
		for _, mi := range in {
			m := integrations.Mapping{
				POS: t.id, ExternalID: strings.TrimSpace(mi.ExternalID), Kind: mi.Kind, Name: strings.TrimSpace(mi.Name),
				DishID: mi.DishID, Modifier: strings.TrimSpace(mi.Modifier), Ignore: mi.Ignore,
				UpdatedAt: s.clk.Now(), UpdatedBy: p.User.ID,
			}
			if err := m.Validate(); err != nil {
				return newErr(KindInvalid, "%s: %s", m.Name, err)
			}
			if m.DishID != "" {
				if d, ok := setup.Dish(m.DishID); !ok || !d.Active {
					return newErr(KindInvalid, "%s: dish %s is not on the menu", m.Name, m.DishID)
				}
			}
			if m.Modifier != "" {
				found := false
				for _, d := range setup.Dishes {
					for _, dm := range d.Modifiers {
						if strings.EqualFold(dm.Name, m.Modifier) {
							m.Modifier, found = dm.Name, true
						}
					}
				}
				if !found {
					return newErr(KindInvalid, "%s: no dish has a modifier called %q", m.Name, m.Modifier)
				}
			}
			if err := tx.POS().SaveMapping(ctx, m); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Items waiting for a mapping go to the kitchen now.
	imported, err := s.st.POS().ListImported(ctx, t.id, 200)
	if err != nil {
		return nil, err
	}
	for _, o := range imported {
		if len(o.Unmapped) > 0 && o.State == integrations.OrderOpen {
			if _, err := s.importOrder(ctx, t, o.Payload, true); err != nil {
				return nil, err
			}
		}
	}
	s.publish(events.KindOrder, "")
	return s.st.POS().ListMappings(ctx, t.id)
}

// SyncResult says what one poll did.
type SyncResult struct {
	// Seen is how many changed till orders came back.
	Seen int `json:"seen"`
	// Created and Updated count kitchen orders entered and added to.
	Created int `json:"created"`
	Updated int `json:"updated"`
	// Skipped counts orders already up to date.
	Skipped int `json:"skipped"`
}

// SyncTillNow polls a till once, for the Sync now button.
func (s *Service) SyncTillNow(ctx context.Context, p *auth.Principal, id string) (SyncResult, error) {
	if err := s.allow(p, auth.ActRunPass); err != nil {
		return SyncResult{}, err
	}
	return s.SyncTill(ctx, id)
}

// SyncTill asks a till for orders changed since the last poll and enters
// them. If the till cannot be reached it records why and changes nothing;
// the next poll asks from the same point, so no order is missed. Orders
// are deduped on the till's order ID and version.
func (s *Service) SyncTill(ctx context.Context, id string) (SyncResult, error) {
	var res SyncResult
	t, err := tillByID(id)
	if err != nil {
		return res, err
	}
	set, err := s.tillGroup(t).current(ctx, s.st)
	if err != nil {
		return res, err
	}
	if !set.Value.Enabled {
		return res, newErr(KindConflict, "Order import from %s is off. A manager turns it on on the %s page.", t.name, t.name)
	}
	pos := s.adapter(t.id)
	if pos == nil {
		return res, notConnected(t)
	}
	st, err := s.st.POS().GetPOSState(ctx, t.id)
	if errors.Is(err, store.ErrNotFound) {
		// Import starts from when it was turned on.
		st = integrations.SyncState{POS: t.id, Since: set.Version.CreatedAt}
	} else if err != nil {
		return res, err
	}
	now := s.clk.Now()
	st.LastPollAt = &now
	list, err := pos.OrdersSince(ctx, set.Value.LocationID, st.Since)
	if err != nil {
		st.LastError = err.Error()
		if serr := s.st.POS().SavePOSState(ctx, st); serr != nil {
			return res, serr
		}
		s.publish(events.KindOrder, "")
		return res, newErr(KindConflict, "Could not reach %s: %s. Orders can still be entered by hand; import carries on from the same point when %s answers.", t.name, err, t.name)
	}
	for _, o := range list {
		res.Seen++
		outcome, err := s.importOrder(ctx, t, o, false)
		if err != nil {
			st.LastError = fmt.Sprintf("order %s: %s", o.ID, err)
			_ = s.st.POS().SavePOSState(ctx, st)
			return res, err
		}
		switch outcome {
		case importCreated:
			res.Created++
		case importUpdated:
			res.Updated++
		default:
			res.Skipped++
		}
		// The next poll asks again from this change, inclusive; versions
		// dedupe what comes back twice.
		if o.UpdatedAt.After(st.Since) {
			st.Since = o.UpdatedAt
		}
	}
	st.LastError, st.LastOKAt = "", &now
	if err := s.st.POS().SavePOSState(ctx, st); err != nil {
		return res, err
	}
	if res.Seen > 0 {
		s.publish(events.KindOrder, "")
	}
	return res, nil
}

type importOutcome int

const (
	importSkipped importOutcome = iota
	importCreated
	importUpdated
)

// importOrder enters a till order, or the part of it that is new since it
// was last seen. force imports again at the same version, after a mapping
// change.
func (s *Service) importOrder(ctx context.Context, t till, o integrations.TillOrder, force bool) (importOutcome, error) {
	prev, err := s.st.POS().GetImported(ctx, t.id, o.ID)
	found := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return importSkipped, err
	}
	if found && o.Version <= prev.Version && !force {
		return importSkipped, nil
	}
	now := s.clk.Now()
	rec := prev
	if !found {
		rec = integrations.Imported{POS: t.id, ExternalID: o.ID, Added: map[string]int{}, FirstSeen: now}
	}
	maps, err := s.st.POS().ListMappings(ctx, t.id)
	if err != nil {
		return importSkipped, err
	}
	byID := map[string]integrations.Mapping{}
	for _, m := range maps {
		byID[m.ExternalID] = m
	}
	ticket, reduced := integrations.Translate(o, byID, rec.Added)
	setup, err := menuSetup(ctx, s.st)
	if err != nil {
		return importSkipped, err
	}
	var notices []string
	for _, r := range reduced {
		notices = append(notices, "Changed on the till: "+r+". Void on the pass what is not wanted.")
	}
	if o.State == integrations.OrderCanceled && prev.State != integrations.OrderCanceled && rec.OrderID != "" {
		notices = append(notices, "Cancelled on the till. Void on the pass what is not wanted.")
	}
	// Dishes that are 86 or off the menu are not refused: the till took
	// the order, so the pass is told instead.
	var items []NewItem
	for _, l := range ticket.Lines {
		rec.Added[l.LineUID] += l.Quantity
		d, ok := setup.Dish(l.DishID)
		switch {
		case !ok || !d.Active:
			notices = append(notices, fmt.Sprintf("%d × %s was ordered on the till but is off the menu.", l.Quantity, dishName(setup, l.DishID)))
			continue
		case !d.Available:
			notices = append(notices, fmt.Sprintf("%d × %s was ordered on the till while 86.", l.Quantity, d.Name))
			continue
		}
		mods := slices.DeleteFunc(slices.Clone(l.Modifiers), func(m string) bool {
			return !slices.ContainsFunc(d.Modifiers, func(dm menu.Modifier) bool { return strings.EqualFold(dm.Name, m) })
		})
		notes := l.Notes
		for _, m := range l.Modifiers {
			if !slices.Contains(mods, m) {
				notes = strings.TrimPrefix(notes+"; "+m, "; ")
			}
		}
		if len([]rune(notes)) > 200 {
			notes = string([]rune(notes)[:200])
		}
		items = append(items, NewItem{DishID: d.ID, Modifiers: mods, Notes: notes, Quantity: l.Quantity})
	}

	outcome := importSkipped
	p := &auth.Principal{User: t.user(), Method: auth.MethodKey}
	if len(items) > 0 {
		var kitchen *orders.Order
		if rec.OrderID != "" {
			if k, err := s.st.Orders().GetOrder(ctx, rec.OrderID); err == nil {
				kitchen = &k
			}
		}
		switch {
		case kitchen == nil:
			no := NewOrder{Kind: orders.KindTable, Table: ticket.Table, Items: items}
			if ticket.Takeaway {
				no.Kind, no.Table, no.Name = orders.KindTakeaway, "", truncate(ticket.Name, 40)
			}
			no.Notes = truncate(tillNote(t, ticket.Unmapped), 500)
			created, err := s.CreateOrder(ctx, p, no)
			if err != nil {
				return importSkipped, fmt.Errorf("enter till order %s: %w", o.ID, err)
			}
			rec.OrderID, rec.Label, outcome = created.ID, created.Label(), importCreated
		case kitchen.Status == orders.StatusClosed:
			for _, it := range items {
				notices = append(notices, fmt.Sprintf("%d × %s was added on the till after the order was closed in the kitchen.", max(it.Quantity, 1), dishName(setup, it.DishID)))
			}
		default:
			if _, err := s.AddItems(ctx, p, kitchen.ID, items); err != nil {
				return importSkipped, fmt.Errorf("add to till order %s: %w", o.ID, err)
			}
			outcome = importUpdated
		}
	}
	if rec.Label == "" {
		rec.Label = ticket.Name
		if !ticket.Takeaway {
			rec.Label = "Table " + ticket.Table
		}
	}
	// Tell the pass: the notices go on the kitchen order's notes and
	// history, where the pass sees them.
	if len(notices) > 0 && rec.OrderID != "" {
		if err := s.noteOrder(ctx, t, p, rec.OrderID, notices); err != nil {
			return importSkipped, err
		}
		if outcome == importSkipped {
			outcome = importUpdated
		}
	}
	rec.Version, rec.State, rec.Payload, rec.LastSeen = o.Version, o.State, o, now
	rec.Unmapped = ticket.Unmapped
	rec.Notices = append(rec.Notices, notices...)
	if err := s.st.POS().SaveImported(ctx, rec); err != nil {
		return importSkipped, err
	}
	return outcome, nil
}

// noteOrder adds till notices to a kitchen order's notes and history.
func (s *Service) noteOrder(ctx context.Context, t till, p *auth.Principal, orderID string, notices []string) error {
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		o, err := tx.Orders().GetOrder(ctx, orderID)
		if err != nil {
			return err
		}
		o.Notes = truncate(strings.TrimSpace(o.Notes+"\n"+t.name+": "+strings.Join(notices, " ")), 500)
		if err := tx.Orders().UpdateOrder(ctx, o); err != nil {
			return err
		}
		for _, n := range notices {
			ev := s.event(p, o, orders.EventPOS)
			ev.Detail = n
			if err := tx.Orders().AppendEvent(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		s.orderChanged(orderID)
	}
	return err
}

// tillNote tells the kitchen about till items it does not know yet.
func tillNote(t till, unmapped []string) string {
	if len(unmapped) == 0 {
		return ""
	}
	return "From " + t.name + ", not on the kitchen menu yet: " + strings.Join(unmapped, ", ") + "."
}

func dishName(setup menu.Setup, id string) string {
	if d, ok := setup.Dish(id); ok {
		return d.Name
	}
	return "a dish no longer on the menu"
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// RunTills polls each till for orders while its import is on, until ctx
// ends.
func (s *Service) RunTills(ctx context.Context, log *slog.Logger) {
	var wg sync.WaitGroup
	for _, t := range tills {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runTill(ctx, t, log)
		}()
	}
	wg.Wait()
}

// runTill polls one till. Failures back off up to two minutes and are
// shown on the till's page.
func (s *Service) runTill(ctx context.Context, t till, log *slog.Logger) {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval, b.MaxInterval = 5*time.Second, 2*time.Minute
	for {
		wait := 15 * time.Second
		set, err := s.tillGroup(t).current(ctx, s.st)
		if err == nil && set.Value.Enabled && s.adapter(t.id) != nil {
			wait = time.Duration(set.Value.PollSeconds) * time.Second
			if res, err := s.SyncTill(ctx, t.id); err != nil {
				wait = b.NextBackOff()
				log.Warn(t.id+" sync", "err", err, "retry_in", wait)
			} else {
				b.Reset()
				if res.Created+res.Updated > 0 {
					log.Info(t.id+" sync", "created", res.Created, "updated", res.Updated)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// TillOrderVersions lists every version of a till order Kishtin saw,
// oldest first, as Kishtin read it: its lines, modifiers and state.
func (s *Service) TillOrderVersions(ctx context.Context, p *auth.Principal, id, externalID string) ([]store.ImportedVersion, error) {
	if err := s.allow(p, auth.ActManageSettings); err != nil {
		return nil, err
	}
	t, err := tillByID(id)
	if err != nil {
		return nil, err
	}
	vs, err := s.st.POS().ListImportedVersions(ctx, t.id, externalID)
	if err != nil {
		return nil, err
	}
	if len(vs) == 0 {
		return nil, newErr(KindNotFound, "no till order %s has been seen", externalID)
	}
	return vs, nil
}
