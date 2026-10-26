package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
)

// fakeTill is an in-memory POS.
type fakeTill struct {
	catalog []integrations.CatalogEntry
	orders  []integrations.TillOrder
	err     error
	since   []time.Time
}

func (f *fakeTill) Name() string { return "square" }
func (f *fakeTill) Locations(context.Context) ([]integrations.Location, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []integrations.Location{{ID: "loc-synthetic", Name: "The Synthetic Kitchen", Active: true}}, nil
}
func (f *fakeTill) Catalog(context.Context, string) ([]integrations.CatalogEntry, error) {
	return f.catalog, f.err
}
func (f *fakeTill) OrdersSince(_ context.Context, loc string, since time.Time) ([]integrations.TillOrder, error) {
	f.since = append(f.since, since)
	if f.err != nil {
		return nil, f.err
	}
	var out []integrations.TillOrder
	for _, o := range f.orders {
		if !o.UpdatedAt.Before(since) {
			out = append(out, o)
		}
	}
	return out, nil
}

func TestSquareImport(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	till := &fakeTill{catalog: []integrations.CatalogEntry{
		{ID: "var-burger", Kind: integrations.EntryItem, Name: "Regular", Group: "Synthetic burger"},
		{ID: "var-cola", Kind: integrations.EntryItem, Name: "Regular", Group: "Synthetic cola"},
		{ID: "var-lamb", Kind: integrations.EntryItem, Name: "Regular", Group: "Lamb special"},
		{ID: "mod-welldone", Kind: integrations.EntryModifier, Name: "Well done", Group: "Cooking"},
	}}

	_, err := k.svc.SyncSquare(ctx)
	assert.ErrorContains(t, err, "Order import from Square is off")
	st, err := k.svc.SquareStatus(ctx, k.cook)
	require.NoError(t, err)
	assert.False(t, st.Connected)
	_, err = k.svc.SquareCatalog(ctx, k.manager)
	assert.ErrorContains(t, err, "Square is not connected")

	k.svc.pos = till

	// The catalog, with suggestions by name.
	_, err = k.svc.SquareCatalog(ctx, k.cook)
	assert.Equal(t, KindForbidden, KindOf(err))
	rows, err := k.svc.SquareCatalog(ctx, k.manager)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	assert.Equal(t, "Lamb special", rows[0].Label)
	assert.Equal(t, "Synthetic burger", rows[1].Label)
	assert.Equal(t, k.burger.ID, rows[1].Suggested)
	assert.Equal(t, "Well done", rows[3].Suggested)

	// Turning import on shows what is sent first.
	on := integrations.POSSettings{Enabled: true, LocationID: "loc-synthetic", PollSeconds: 5}
	pv, err := k.svc.Square().Preview(ctx, k.manager, SettingsChange[integrations.POSSettings]{Value: on, Reason: "Go live"})
	require.NoError(t, err)
	assert.Empty(t, pv.Problem)
	assert.Contains(t, pv.Notes[0], "send Square the access token, the location ID (loc-synthetic)")
	assert.Contains(t, pv.Notes[1], "Nothing about the kitchen, staff or HACCP records is sent")
	_, err = k.svc.Square().Save(ctx, k.chef, SettingsChange[integrations.POSSettings]{Value: on, Reason: "Go live"})
	assert.Equal(t, KindForbidden, KindOf(err), "a manager turns it on")
	enabledAt := k.clk.Now()
	_, err = k.svc.Square().Save(ctx, k.manager, SettingsChange[integrations.POSSettings]{Value: on, Reason: "Go live"})
	require.NoError(t, err)

	_, err = k.svc.SaveSquareMappings(ctx, k.manager, []MappingInput{
		{ExternalID: "var-burger", Kind: integrations.EntryItem, Name: "Synthetic burger", DishID: k.burger.ID},
		{ExternalID: "var-cola", Kind: integrations.EntryItem, Name: "Synthetic cola", Ignore: true},
		{ExternalID: "mod-welldone", Kind: integrations.EntryModifier, Name: "Well done", Modifier: "well done"},
	})
	require.NoError(t, err)
	_, err = k.svc.SaveSquareMappings(ctx, k.manager, []MappingInput{{ExternalID: "mod-x", Kind: integrations.EntryModifier, Name: "Extra", Modifier: "Extra cheese"}})
	assert.ErrorContains(t, err, `no dish has a modifier called "Extra cheese"`)

	k.clk.Advance(time.Minute)
	t1 := k.clk.Now()
	till.orders = []integrations.TillOrder{{
		ID: "sq-1", Version: 1, State: integrations.OrderOpen, Ticket: "Table 4", UpdatedAt: t1,
		Lines: []integrations.TillLine{
			{UID: "l1", CatalogID: "var-burger", Name: "Synthetic burger", Quantity: 2, Note: "no onion", Modifiers: []integrations.TillModifier{{CatalogID: "mod-welldone", Name: "Well done"}}},
			{UID: "l2", CatalogID: "var-cola", Name: "Synthetic cola", Quantity: 2},
			{UID: "l3", CatalogID: "var-lamb", Name: "Lamb special", Quantity: 1},
		},
	}, {
		ID: "sq-2", Version: 1, State: integrations.OrderOpen, Takeaway: true, Customer: "Synthetic customer", UpdatedAt: t1,
		Lines: []integrations.TillLine{{UID: "m1", CatalogID: "var-burger", Name: "Synthetic burger", Quantity: 1}},
	}, {
		ID: "sq-3", Version: 1, State: integrations.OrderOpen, Ticket: "Bar", UpdatedAt: t1,
		Lines: []integrations.TillLine{{UID: "b1", CatalogID: "var-cola", Name: "Synthetic cola", Quantity: 3}},
	}}
	res, err := k.svc.SyncSquare(ctx)
	require.NoError(t, err)
	assert.Equal(t, SyncResult{Seen: 3, Created: 2, Skipped: 1}, res, "drinks only: nothing for the kitchen")
	assert.True(t, enabledAt.Equal(till.since[0]), "import starts from when it was turned on")

	open, err := k.svc.ListOrders(ctx, k.chef, OrderFilter{})
	require.NoError(t, err)
	require.Len(t, open, 2)
	t4 := open[0]
	assert.Equal(t, "Table 4", t4.Label())
	assert.Equal(t, "Square", eventsOf(t, k, t4.ID)[0].ByName)
	require.Len(t, t4.Items, 2)
	for _, it := range t4.Items {
		assert.Equal(t, orders.StateHeld, it.State, "the pass fires imported courses")
		assert.Equal(t, []string{"Well done"}, it.Modifiers)
		assert.Equal(t, "no onion", it.Notes)
	}
	assert.Equal(t, "From Square, not on the kitchen menu yet: 1 × Lamb special.", t4.Notes)
	assert.Equal(t, "Takeaway #2 (Synthetic customer)", open[1].Label())

	// The same orders again change nothing.
	res, err = k.svc.SyncSquare(ctx)
	require.NoError(t, err)
	assert.Equal(t, SyncResult{Seen: 3, Skipped: 3}, res)
	assert.True(t, t1.Equal(till.since[1]), "asks again from the last change")

	// Mapping the lamb adds it to the open order.
	_, err = k.svc.SaveSquareMappings(ctx, k.manager, []MappingInput{{ExternalID: "var-lamb", Kind: integrations.EntryItem, Name: "Lamb special", DishID: k.salad.ID}})
	require.NoError(t, err)
	t4, err = k.svc.GetOrder(ctx, k.chef, t4.ID)
	require.NoError(t, err)
	require.Len(t, t4.Items, 3)
	assert.Equal(t, "Synthetic salad", t4.Items[2].DishName)
	assert.Len(t, t4.Courses, 2, "a starter course for the new item")

	// Version 2: one more burger, and fries while they are 86.
	fries, err := k.svc.CreateDish(ctx, k.manager, DishInput{Name: "Synthetic fries", Course: menu.CourseMain, StationID: k.grill.ID, CookSeconds: 240})
	require.NoError(t, err)
	_, err = k.svc.SaveSquareMappings(ctx, k.manager, []MappingInput{{ExternalID: "var-fries", Kind: integrations.EntryItem, Name: "Fries", DishID: fries.ID}})
	require.NoError(t, err)
	_, err = k.svc.SetAvailability(ctx, k.chef, fries.ID, false)
	require.NoError(t, err)
	_, err = k.svc.FireCourse(ctx, k.chef, t4.ID, menu.CourseMain)
	require.NoError(t, err)
	k.clk.Advance(time.Minute)
	o := till.orders[0]
	o.Version, o.UpdatedAt = 2, k.clk.Now()
	o.Lines = append([]integrations.TillLine{}, o.Lines...)
	o.Lines[0].Quantity = 3
	o.Lines = append(o.Lines, integrations.TillLine{UID: "l4", CatalogID: "var-fries", Name: "Fries", Quantity: 1})
	till.orders = []integrations.TillOrder{o}
	res, err = k.svc.SyncSquare(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Updated)
	t4, err = k.svc.GetOrder(ctx, k.chef, t4.ID)
	require.NoError(t, err)
	require.Len(t, t4.Items, 4)
	assert.Equal(t, orders.StateFired, t4.Items[3].State, "added to a fired course: straight to the station")
	assert.Contains(t, t4.Notes, "1 × Synthetic fries was ordered on the till while 86.")

	// Version 3: fewer burgers. Nothing is voided; the pass is told.
	k.clk.Advance(time.Minute)
	o.Version, o.UpdatedAt = 3, k.clk.Now()
	o.Lines = append([]integrations.TillLine{}, o.Lines...)
	o.Lines[0].Quantity = 1
	till.orders = []integrations.TillOrder{o}
	_, err = k.svc.SyncSquare(ctx)
	require.NoError(t, err)
	t4, err = k.svc.GetOrder(ctx, k.chef, t4.ID)
	require.NoError(t, err)
	assert.Len(t, t4.Items, 4)
	assert.Contains(t, t4.Notes, "Changed on the till: Synthetic burger now 1, was 3. Void on the pass what is not wanted.")
	evs := eventsOf(t, k, t4.ID)
	assert.Equal(t, orders.EventPOS, evs[len(evs)-1].Kind)

	// Square down: nothing changes, and the reason is kept.
	till.err = errors.New("connection refused")
	_, err = k.svc.SyncSquare(ctx)
	assert.ErrorContains(t, err, "Could not reach Square: connection refused")
	st, err = k.svc.SquareStatus(ctx, k.cook)
	require.NoError(t, err)
	assert.True(t, st.Connected)
	require.NotNil(t, st.State)
	assert.Equal(t, "connection refused", st.State.LastError)
	assert.True(t, o.UpdatedAt.Equal(st.State.Since), "carries on from the same point")
	require.Len(t, st.Recent, 3)
	assert.Len(t, st.Mappings, 5)

	_, err = k.svc.SyncSquareNow(ctx, k.foh)
	assert.Equal(t, KindForbidden, KindOf(err))
}

func eventsOf(t *testing.T, k *kitchen, id string) []orders.Event {
	t.Helper()
	evs, err := k.svc.OrderEvents(context.Background(), k.chef, id)
	require.NoError(t, err)
	return evs
}
