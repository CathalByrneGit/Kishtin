package storetest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
)

func testActivity(t *testing.T, s store.Store) {
	ctx := context.Background()
	a := s.Activity()
	for i, kind := range []string{"dish.availability", "login", "dish.availability"} {
		require.NoError(t, a.AppendActivity(ctx, store.Activity{
			ID: "a" + string(rune('1'+i)), At: t0.Add(time.Duration(i) * time.Minute), Kind: kind,
			UserID: "u1", UserName: "Synthetic Chef", DeviceID: "k1", DeviceName: "Pass tablet",
			Subject: "d1", Detail: map[string]string{"available": "false"},
		}))
	}
	assert.ErrorIs(t, a.AppendActivity(ctx, store.Activity{ID: "a1", At: t0, Kind: "x"}), store.ErrConflict)

	all, err := a.ListActivity(ctx, store.ActivityFilter{})
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, "Pass tablet", all[0].DeviceName)
	assert.Equal(t, map[string]string{"available": "false"}, all[0].Detail)
	dishes, err := a.ListActivity(ctx, store.ActivityFilter{Kinds: []string{"dish.availability"}, From: t0.Add(time.Minute)})
	require.NoError(t, err)
	require.Len(t, dishes, 1)
	assert.Equal(t, "a3", dishes[0].ID)

	_, err = a.LastPlanSnapshot(ctx)
	assert.ErrorIs(t, err, store.ErrNotFound)
	for i := range 3 {
		require.NoError(t, a.AppendPlanSnapshot(ctx, store.PlanSnapshot{
			ID: "p" + string(rune('1'+i)), At: t0.Add(time.Duration(i) * time.Minute), Trigger: "timer", Strategy: "backward",
			SettingsVersion: "sv1", Hash: "h" + string(rune('1'+i)), Body: json.RawMessage(`{"courses":[]}`),
		}))
	}
	last, err := a.LastPlanSnapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, "p3", last.ID)
	window, err := a.ListPlanSnapshots(ctx, t0, t0.Add(2*time.Minute), 0)
	require.NoError(t, err)
	require.Len(t, window, 2)
	assert.Equal(t, "h1", window[0].Hash)
	assert.JSONEq(t, `{"courses":[]}`, string(window[0].Body))
}

func testImportedVersions(t *testing.T, s store.Store) {
	ctx := context.Background()
	o := integrations.Imported{POS: "square", ExternalID: "sq-9", Version: 1, State: integrations.OrderOpen, Label: "Table 9", FirstSeen: t0, LastSeen: t0,
		Payload: integrations.TillOrder{ID: "sq-9", Version: 1, State: integrations.OrderOpen, Lines: []integrations.TillLine{{UID: "l1", Name: "Synthetic soup", Quantity: 1}}}}
	require.NoError(t, s.POS().SaveImported(ctx, o))
	require.NoError(t, s.POS().SaveImported(ctx, o), "seeing the same version again keeps one copy")
	o.Version, o.Payload.Version, o.LastSeen = 2, 2, t0.Add(time.Minute)
	o.Payload.Lines = append(o.Payload.Lines, integrations.TillLine{UID: "l2", Name: "Synthetic bread", Quantity: 2})
	require.NoError(t, s.POS().SaveImported(ctx, o))
	vs, err := s.POS().ListImportedVersions(ctx, "square", "sq-9")
	require.NoError(t, err)
	require.Len(t, vs, 2)
	assert.Len(t, vs[0].Payload.Lines, 1)
	assert.Len(t, vs[1].Payload.Lines, 2)
	assert.True(t, t0.Add(time.Minute).Equal(vs[1].SeenAt))
}

func testAnonymiseUser(t *testing.T, s store.Store) {
	ctx := context.Background()
	u := auth.User{ID: "u-leaver", Name: "Synthetic Leaver", Role: auth.RoleLineCook, Active: true, CreatedAt: t0, UpdatedAt: t0}
	require.NoError(t, s.Users().CreateUser(ctx, u))
	sampleMenu(t, s)
	o := sampleOrder("o-leaver", 1, t0)
	require.NoError(t, s.Orders().CreateOrder(ctx, o))
	require.NoError(t, s.Orders().AppendEvent(ctx, orders.Event{ID: "e1", OrderID: o.ID, Kind: orders.EventCreated, At: t0, By: u.ID, ByName: u.Name, Device: "k1"}))
	require.NoError(t, s.Orders().AppendEvent(ctx, orders.Event{ID: "e2", OrderID: o.ID, Kind: orders.EventCreated, At: t0, By: "u-other", ByName: "Synthetic Other"}))
	require.NoError(t, s.Voice().AppendVoiceLog(ctx, voice.LogEntry{ID: "v1", At: t0, UserID: u.ID, UserName: u.Name, Source: voice.SourceTyped,
		Transcript: "86 salmon", Outcome: voice.OutcomeDone, Reply: "Done.", Device: "k1", AudioFile: "v1.webm"}))
	require.NoError(t, s.Activity().AppendActivity(ctx, store.Activity{ID: "a1", At: t0, Kind: "login", UserID: u.ID, UserName: u.Name}))

	m, err := s.Users().CountUserMentions(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, store.UserMentions{OrderEvents: 1, VoiceLog: 1, Activity: 1}, m)

	got, err := s.Users().AnonymiseUser(ctx, u.ID, "Former staff 1")
	require.NoError(t, err)
	assert.Equal(t, m, got)
	ev, err := s.Orders().ListEvents(ctx, o.ID)
	require.NoError(t, err)
	names := map[string]string{}
	for _, e := range ev {
		names[e.ID] = e.ByName
	}
	assert.Equal(t, map[string]string{"e1": "Former staff 1", "e2": "Synthetic Other"}, names)
	assert.Equal(t, "k1", ev[0].Device)
	vl, err := s.Voice().ListVoiceLog(ctx, time.Time{}, 10)
	require.NoError(t, err)
	assert.Equal(t, "Former staff 1", vl[0].UserName)
	assert.Equal(t, "v1.webm", vl[0].AudioFile)
	act, err := s.Activity().ListActivity(ctx, store.ActivityFilter{Kinds: []string{"login"}})
	require.NoError(t, err)
	assert.Equal(t, "Former staff 1", act[0].UserName)

	at := t0.Add(time.Hour)
	u.Name, u.Active, u.AnonymisedAt, u.UpdatedAt = "Former staff 1", false, &at, at
	require.NoError(t, s.Users().UpdateUser(ctx, u))
	back, err := s.Users().GetUser(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, back.AnonymisedAt)
	assert.True(t, at.Equal(*back.AnonymisedAt))
}

func testClearCustomerDetails(t *testing.T, s store.Store) {
	ctx := context.Background()
	sampleMenu(t, s)
	o := sampleOrder("ord-c", 1, t0)
	o.Kind, o.Table, o.Name, o.Notes = orders.KindTakeaway, "", "Aoife \"Synthetic\" Ní Bhriain", "Severe nut allergy"
	require.NoError(t, s.Orders().CreateOrder(ctx, o))
	other := sampleOrder("ord-d", 2, t0)
	other.Name = "Liam"
	require.NoError(t, s.Orders().CreateOrder(ctx, other))
	label := o.Label()
	require.NoError(t, s.Orders().AppendEvent(ctx, orders.Event{ID: "ev-1", OrderID: o.ID, Kind: orders.EventCreated, Detail: label + ", 2 items", At: t0, By: "u1", ByName: "Synthetic cook"}))
	require.NoError(t, s.Orders().AppendEvent(ctx, orders.Event{ID: "ev-2", OrderID: o.ID, Kind: orders.EventRenamed, From: "Aoife", To: o.Name, Detail: "renamed from Aoife to " + o.Name, At: t0, By: "u1", ByName: "Synthetic cook"}))
	body, err := json.Marshal(map[string]any{"courses": []map[string]string{{"label": label}, {"label": other.Label()}}})
	require.NoError(t, err)
	require.NoError(t, s.Activity().AppendPlanSnapshot(ctx, store.PlanSnapshot{ID: "ps-1", At: t0.Add(time.Minute), Trigger: "timer", Strategy: "backward", SettingsVersion: "v", Hash: "h", Body: body}))
	require.NoError(t, s.Voice().AppendVoiceLog(ctx, voice.LogEntry{ID: "vl-1", At: t0.Add(time.Minute), UserID: "u1", UserName: "Synthetic cook", Source: "push", Transcript: "fire aoife mains", Outcome: "done", Reply: label + " mains fired"}))
	imp := integrations.Imported{POS: "square", ExternalID: "sq-c", Version: 1, State: integrations.OrderOpen, OrderID: o.ID, Label: label, FirstSeen: t0, LastSeen: t0,
		Payload: integrations.TillOrder{ID: "sq-c", Version: 1, State: integrations.OrderOpen, Customer: o.Name, Takeaway: true,
			Lines: []integrations.TillLine{{UID: "l1", Name: "Synthetic soup", Quantity: 1, Note: "Severe nut allergy"}}}}
	require.NoError(t, s.POS().SaveImported(ctx, imp))

	due, err := s.Orders().OrdersWithCustomerDetails(ctx, t0, 10)
	require.NoError(t, err)
	assert.Empty(t, due, "nothing from before the orders")
	due, err = s.Orders().OrdersWithCustomerDetails(ctx, t0.Add(time.Second), 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"ord-c", "ord-d"}, due)
	due, err = s.Orders().OrdersWithCustomerDetails(ctx, t0.Add(time.Second), 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"ord-c"}, due)

	require.NoError(t, s.WithTx(ctx, func(tx store.Store) error {
		return tx.Orders().ClearCustomerDetails(ctx, store.CustomerClear{
			OrderID: o.ID, Names: []string{o.Name, "Aoife"}, Notes: []string{o.Notes, "no onion"}, From: t0, To: t0.Add(time.Hour),
		})
	}))

	got, err := s.Orders().GetOrder(ctx, o.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Name)
	assert.Empty(t, got.Notes)
	for _, it := range got.Items {
		assert.Empty(t, it.Notes)
	}
	evs, err := s.Orders().ListEvents(ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, "Takeaway #1, 2 items", evs[0].Detail)
	assert.Equal(t, "name cleared", evs[1].Detail)
	assert.Empty(t, evs[1].From)
	assert.Empty(t, evs[1].To)
	assert.Equal(t, "Synthetic cook", evs[0].ByName, "staff names stay")

	snaps, err := s.Activity().ListPlanSnapshots(ctx, time.Time{}, time.Time{}, 10)
	require.NoError(t, err)
	require.Len(t, snaps, 1)
	assert.NotContains(t, string(snaps[0].Body), "Bhriain")
	assert.Contains(t, string(snaps[0].Body), "Takeaway #1")
	assert.Contains(t, string(snaps[0].Body), "Liam", "other orders' labels stay")

	vl, err := s.Voice().ListVoiceLog(ctx, time.Time{}, 10)
	require.NoError(t, err)
	assert.Equal(t, "Takeaway #1 mains fired", vl[0].Reply)

	gotImp, err := s.POS().GetImported(ctx, "square", "sq-c")
	require.NoError(t, err)
	assert.Equal(t, "Takeaway #1", gotImp.Label)
	assert.Empty(t, gotImp.Payload.Customer)
	assert.Empty(t, gotImp.Payload.Lines[0].Note)
	vs, err := s.POS().ListImportedVersions(ctx, "square", "sq-c")
	require.NoError(t, err)
	assert.Empty(t, vs[0].Payload.Customer)
	assert.Empty(t, vs[0].Payload.Lines[0].Note)

	kept, err := s.Orders().GetOrder(ctx, other.ID)
	require.NoError(t, err)
	assert.Equal(t, "Liam", kept.Name, "other orders are not touched")
	assert.Equal(t, "no onion", kept.Items[0].Notes)
	due, err = s.Orders().OrdersWithCustomerDetails(ctx, t0.Add(time.Second), 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"ord-d"}, due, "a cleared order is not found again")
}

func testPurgePlanSnapshots(t *testing.T, s store.Store) {
	ctx := context.Background()
	a := s.Activity()
	now := time.Now().UTC()
	snap := func(id string, at time.Time) store.PlanSnapshot {
		return store.PlanSnapshot{ID: id, At: at, Trigger: "timer", Strategy: "backward", SettingsVersion: "v", Hash: id, Body: []byte(`{}`)}
	}
	require.NoError(t, a.AppendPlanSnapshot(ctx, snap("old-1", now.AddDate(0, 0, -400))))
	require.NoError(t, a.AppendPlanSnapshot(ctx, snap("old-2", now.AddDate(0, 0, -40))))
	require.NoError(t, a.AppendPlanSnapshot(ctx, snap("new-1", now.AddDate(0, 0, -1))))

	n, err := a.CountPlanSnapshotsBefore(ctx, now.AddDate(0, 0, -35))
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	n, err = a.PurgePlanSnapshotsBefore(ctx, now.AddDate(0, 0, -35))
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	// Nothing from the last 30 days can be removed.
	_, err = a.PurgePlanSnapshotsBefore(ctx, now)
	require.Error(t, err)
	left, err := a.ListPlanSnapshots(ctx, time.Time{}, time.Time{}, 10)
	require.NoError(t, err)
	require.Len(t, left, 1)
	assert.Equal(t, "new-1", left[0].ID)
}

func testClientIDs(t *testing.T, s store.Store) {
	ctx := context.Background()
	h := s.HACCP()
	_, err := h.RecordIDByClientID(ctx, "c1")
	require.ErrorIs(t, err, store.ErrNotFound)
	require.NoError(t, h.SaveClientID(ctx, "c1", "rec-1", t0))
	id, err := h.RecordIDByClientID(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "rec-1", id)
	assert.ErrorIs(t, h.SaveClientID(ctx, "c1", "rec-2", t0), store.ErrConflict, "one record per client ID")
}
