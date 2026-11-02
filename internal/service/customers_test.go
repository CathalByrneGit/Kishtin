package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
)

func TestClearCustomerDetails(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()

	old, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTakeaway, Name: "Niamh Synthetic", Notes: "Coeliac",
		Items: []NewItem{{DishID: k.salad.ID, Notes: "no croutons"}}})
	require.NoError(t, err)
	_, err = k.svc.RenameOrder(ctx, k.foh, old.ID, "Niamh S.")
	require.NoError(t, err)
	k.clk.Advance(31 * 24 * time.Hour)
	recent, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: "4", Name: "Ciarán Synthetic", Notes: "Birthday",
		Items: []NewItem{{DishID: k.salad.ID}}})
	require.NoError(t, err)

	_, err = k.svc.PreviewClearCustomerDetails(ctx, k.chef)
	assert.Equal(t, KindForbidden, KindOf(err))
	pv, err := k.svc.PreviewClearCustomerDetails(ctx, k.manager)
	require.NoError(t, err)
	assert.Equal(t, settings.DefaultCustomerDetailsDays, pv.Days)
	assert.Equal(t, 1, pv.Orders, "only the order from 31 days ago")

	_, err = k.svc.ClearCustomerDetails(ctx, k.chef)
	assert.Equal(t, KindForbidden, KindOf(err))
	res, err := k.svc.ClearCustomerDetails(ctx, k.manager)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Orders)

	got, err := k.svc.GetOrder(ctx, k.chef, old.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Name)
	assert.Empty(t, got.Notes)
	assert.Empty(t, got.Items[0].Notes)
	assert.Equal(t, k.salad.ID, got.Items[0].DishID, "what was ordered stays")
	evs, err := k.svc.OrderEvents(ctx, k.chef, old.ID)
	require.NoError(t, err)
	for _, e := range evs {
		assert.NotContains(t, e.Detail+e.From+e.To, "Niamh", e.Kind)
		assert.Equal(t, "Sean Floor", e.ByName, "staff names stay")
	}
	kept, err := k.svc.GetOrder(ctx, k.chef, recent.ID)
	require.NoError(t, err)
	assert.Equal(t, "Ciarán Synthetic", kept.Name)
	assert.Equal(t, "Birthday", kept.Notes)

	acts, err := k.svc.Activity(ctx, k.manager, ActivityQuery{Kinds: []string{ActivityCustomerDetailsCleared}})
	require.NoError(t, err)
	require.Len(t, acts, 1)
	assert.Equal(t, "1", acts[0].Detail["orders"])

	res, err = k.svc.PurgeCustomerDetails(ctx)
	require.NoError(t, err)
	assert.Zero(t, res.Orders, "nothing left to clear")
	acts, err = k.svc.Activity(ctx, k.manager, ActivityQuery{Kinds: []string{ActivityCustomerDetailsCleared}})
	require.NoError(t, err)
	assert.Len(t, acts, 1, "a run that clears nothing is not logged")

	// Shortening the time shows how many orders it clears.
	sys, err := k.svc.System().current(ctx, k.svc.st)
	require.NoError(t, err)
	next := sys.Value
	next.CustomerDetailsDays = 0
	spv, err := k.svc.System().Preview(ctx, k.manager, SettingsChange[settings.System]{Value: next, Reason: "Keep them"})
	require.NoError(t, err)
	assert.Contains(t, strings.Join(spv.Notes, " "), "kept for as long as the orders are")
	_, err = k.svc.System().Save(ctx, k.manager, SettingsChange[settings.System]{Value: next, Reason: "Keep them"})
	require.NoError(t, err)
	k.clk.Advance(400 * 24 * time.Hour)
	res, err = k.svc.PurgeCustomerDetails(ctx)
	require.NoError(t, err)
	assert.Zero(t, res.Orders, "0 keeps them")
	pv, err = k.svc.PreviewClearCustomerDetails(ctx, k.manager)
	require.NoError(t, err)
	assert.Zero(t, pv.Orders)
	assert.Contains(t, pv.Notes[0], "kept")

	next.CustomerDetailsDays = 7
	spv, err = k.svc.System().Preview(ctx, k.manager, SettingsChange[settings.System]{Value: next, Reason: "Clear after a week"})
	require.NoError(t, err)
	assert.Contains(t, strings.Join(spv.Notes, " "), "1 orders now")
}

func TestSystemSettingsFromBeforeCustomerDetails(t *testing.T) {
	var s settings.System
	require.NoError(t, s.UnmarshalJSON([]byte(`{"restaurant_name":"Synthetic Bistro","max_upload_mb":15,"preview_records":50}`)))
	assert.Equal(t, settings.DefaultCustomerDetailsDays, s.CustomerDetailsDays, "settings saved before take the default")
	require.NoError(t, s.UnmarshalJSON([]byte(`{"customer_details_days":0}`)))
	assert.Zero(t, s.CustomerDetailsDays, "0 saved on purpose stays 0")
}
