package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/diskfree"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/store"
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

func TestStorageWarning(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	free := uint64(50 << 30)
	f.svc.storage = StorageConfig{DataDir: t.TempDir(), Free: func(string) (diskfree.Space, error) {
		return diskfree.Space{Free: free, Total: 100 << 30}, nil
	}}
	st, err := f.svc.Storage(ctx, f.cook)
	require.NoError(t, err)
	assert.Equal(t, StorageOK, st.Level)
	assert.Empty(t, st.Warning)
	td, err := f.svc.Today(ctx, f.cook)
	require.NoError(t, err)
	assert.Empty(t, td.StorageWarning)

	free = 1 << 30
	st, err = f.svc.Storage(ctx, f.cook)
	require.NoError(t, err)
	assert.Equal(t, StorageLow, st.Level)
	assert.Contains(t, st.Warning, "1.0 GB left")

	free = 100 << 20
	td, err = f.svc.Today(ctx, f.cook)
	require.NoError(t, err)
	assert.Contains(t, td.StorageWarning, "almost full (100 MB left)")
}

func TestPurgePlanSnapshots(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	old := time.Now().AddDate(0, 0, -200)
	require.NoError(t, k.svc.st.Activity().AppendPlanSnapshot(ctx, store.PlanSnapshot{ID: "old", At: old, Trigger: "timer", Hash: "h", Body: []byte(`{}`)}))
	k.clk.Set(time.Now())

	n, err := k.svc.PurgePlanSnapshots(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "kept by default")

	sys, err := k.svc.System().current(ctx, k.svc.st)
	require.NoError(t, err)
	next := sys.Value
	next.PlanSnapshotDays = 10
	bad, err := k.svc.System().Preview(ctx, k.manager, SettingsChange[settings.System]{Value: next, Reason: "Save space"})
	require.NoError(t, err)
	assert.Contains(t, bad.Problem, "between 30 and", "at least 30 days")
	next.PlanSnapshotDays = 90
	pv, err := k.svc.System().Preview(ctx, k.manager, SettingsChange[settings.System]{Value: next, Reason: "Save space"})
	require.NoError(t, err)
	assert.Contains(t, strings.Join(pv.Notes, " "), "older than 90 days will be removed: 1 now")
	_, err = k.svc.System().Save(ctx, k.manager, SettingsChange[settings.System]{Value: next, Reason: "Save space"})
	require.NoError(t, err)

	n, err = k.svc.PurgePlanSnapshots(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	acts, err := k.svc.Activity(ctx, k.manager, ActivityQuery{Kinds: []string{ActivityPlansPurged}})
	require.NoError(t, err)
	require.Len(t, acts, 1)
	assert.Equal(t, "1", acts[0].Detail["plans"])
}
