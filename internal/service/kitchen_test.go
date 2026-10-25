package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/scheduler"
)

type kitchen struct {
	*fixture
	chef, foh *auth.Principal
	grill     menu.Station
	cold      menu.Station
	flat      menu.Equipment
	burger    menu.Dish
	salad     menu.Dish
}

func setupKitchen(t *testing.T) *kitchen {
	t.Helper()
	f := setup(t)
	ctx := context.Background()
	k := &kitchen{fixture: f}
	chef, err := f.svc.CreateUser(ctx, f.manager, NewUser{Name: "Aoife Chef", Role: auth.RoleHeadChef, PIN: "2222"})
	require.NoError(t, err)
	foh, err := f.svc.CreateUser(ctx, f.manager, NewUser{Name: "Sean Floor", Role: auth.RoleFrontOfHouse, PIN: "3333"})
	require.NoError(t, err)
	k.chef = &auth.Principal{User: chef, Method: auth.MethodPIN}
	k.foh = &auth.Principal{User: foh, Method: auth.MethodPIN}

	k.grill, err = f.svc.CreateStation(ctx, f.manager, StationInput{Name: "Grill", Position: 1})
	require.NoError(t, err)
	k.cold, err = f.svc.CreateStation(ctx, f.manager, StationInput{Name: "Cold section", Position: 2})
	require.NoError(t, err)
	k.flat, err = f.svc.CreateEquipment(ctx, f.manager, EquipmentInput{StationID: k.grill.ID, Name: "Flat-top", Slots: 3})
	require.NoError(t, err)
	k.burger, err = f.svc.CreateDish(ctx, f.manager, DishInput{
		Name: "Synthetic burger", Course: menu.CourseMain, StationID: k.grill.ID, EquipmentID: k.flat.ID,
		CookSeconds: 480, RestSeconds: 120, PlateSeconds: 60, HoldSeconds: 180, Batchable: true,
		Modifiers: []menu.Modifier{{Name: "Well done", AddSeconds: 120}},
	})
	require.NoError(t, err)
	k.salad, err = f.svc.CreateDish(ctx, f.manager, DishInput{
		Name: "Synthetic salad", Course: menu.CourseStarter, StationID: k.cold.ID, PlateSeconds: 180, HoldSeconds: 300,
	})
	require.NoError(t, err)
	return k
}

func TestMenuRules(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()

	_, err := k.svc.CreateStation(ctx, k.cook, StationInput{Name: "Pastry"})
	assert.Equal(t, KindForbidden, KindOf(err), "only a manager changes the menu")
	pin := &auth.Principal{User: k.manager.User, Method: auth.MethodPIN}
	_, err = k.svc.CreateStation(ctx, pin, StationInput{Name: "Pastry"})
	assert.Equal(t, KindForbidden, KindOf(err), "a manager PIN login is not enough")

	_, err = k.svc.CreateStation(ctx, k.manager, StationInput{Name: "grill"})
	assert.Equal(t, KindConflict, KindOf(err), "names ignore case")

	_, err = k.svc.CreateDish(ctx, k.manager, DishInput{Name: "Wrong place", Course: menu.CourseMain, StationID: k.cold.ID, EquipmentID: k.flat.ID, CookSeconds: 60})
	assert.ErrorContains(t, err, "in another station")

	// With food categories listed, a dish's category must be one of them.
	lists, err := k.svc.Lists().Current(ctx, k.manager)
	require.NoError(t, err)
	lists.Value.FoodCategories = []string{"poultry"}
	_, err = k.svc.Lists().Save(ctx, k.manager, SettingsChange[haccp.Lists]{Value: lists.Value, Reason: "Add categories"})
	require.NoError(t, err)
	_, err = k.svc.CreateDish(ctx, k.manager, DishInput{Name: "Chicken", Course: menu.CourseMain, StationID: k.grill.ID, CookSeconds: 60, FoodCategory: "dragon"})
	assert.ErrorContains(t, err, "not on the list")
	chicken, err := k.svc.CreateDish(ctx, k.manager, DishInput{Name: "Chicken", Course: menu.CourseMain, StationID: k.grill.ID, CookSeconds: 60, FoodCategory: "POULTRY"})
	require.NoError(t, err)
	assert.Equal(t, "poultry", chicken.FoodCategory, "the listed spelling is kept")

	// A station with dishes cannot be taken out of use.
	_, err = k.svc.UpdateStation(ctx, k.manager, k.grill.ID, StationInput{Name: "Grill", Active: new(false)})
	assert.ErrorContains(t, err, "still used by")
	_, err = k.svc.UpdateEquipment(ctx, k.manager, k.flat.ID, EquipmentInput{StationID: k.cold.ID, Name: "Flat-top", Slots: 3})
	assert.ErrorContains(t, err, "used by Synthetic burger")

	// A line cook can 86 a dish; front of house cannot.
	d, err := k.svc.SetAvailability(ctx, k.cook, k.burger.ID, false)
	require.NoError(t, err)
	assert.False(t, d.Available)
	_, err = k.svc.SetAvailability(ctx, k.foh, k.burger.ID, true)
	assert.Equal(t, KindForbidden, KindOf(err))

	_, err = k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: "4", Items: []NewItem{{DishID: k.burger.ID}}})
	assert.ErrorContains(t, err, "sold out")

	setup, err := k.svc.Menu(ctx, k.foh)
	require.NoError(t, err)
	assert.Len(t, setup.Dishes, 3)
}

func TestOrderLifecycle(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()

	o, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{
		Kind: orders.KindTable, Table: "4", Covers: 2,
		Items: []NewItem{
			{DishID: k.salad.ID},
			{DishID: k.burger.ID, Quantity: 2},
			{DishID: k.burger.ID, Modifiers: []string{"well done"}, Notes: "no onion"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, o.Number)
	assert.Equal(t, "Table 4", o.Label())
	require.Len(t, o.Items, 4)
	require.Len(t, o.Courses, 2, "starter and main")
	for _, it := range o.Items {
		assert.Equal(t, orders.StateHeld, it.State)
	}
	wellDone := o.Items[3]
	assert.Equal(t, []string{"Well done"}, wellDone.Modifiers, "the menu's spelling is kept")
	assert.Equal(t, 10*time.Minute, wellDone.Timing.Cook)
	assert.Equal(t, 3, wellDone.BatchCapacity, "capacity defaults to the equipment's slots")

	// Front of house cannot fire.
	_, err = k.svc.FireCourse(ctx, k.foh, o.ID, menu.CourseMain)
	assert.Equal(t, KindForbidden, KindOf(err))

	// Held items cannot be started.
	_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: []string{o.Items[1].ID}})
	assert.ErrorContains(t, err, "fire it first")

	o, err = k.svc.FireCourse(ctx, k.chef, o.ID, menu.CourseMain)
	require.NoError(t, err)
	main, _ := o.Course(menu.CourseMain)
	assert.Equal(t, orders.CourseFired, main.State)
	assert.Equal(t, start.Add(13*time.Minute), *main.TargetAt, "now plus the longest item: 10 + 2 + 1 min")
	_, err = k.svc.FireCourse(ctx, k.chef, o.ID, menu.CourseMain)
	assert.ErrorContains(t, err, "already fired")

	// The plan has grill actions with reasons.
	view, err := k.svc.Kitchen(ctx, k.cook)
	require.NoError(t, err)
	assert.Equal(t, scheduler.BuiltinSettingsID, view.Plan.SettingsVersion)
	require.Len(t, view.Plan.Courses, 1)
	assert.Equal(t, scheduler.StatusOnTrack, view.Plan.Courses[0].Status)
	var grill scheduler.StationPlan
	for _, sp := range view.Plan.Stations {
		if sp.StationID == k.grill.ID {
			grill = sp
		}
	}
	require.NotEmpty(t, grill.Actions)
	assert.True(t, grill.Actions[0].Now)
	assert.Contains(t, grill.Actions[0].Text, "Well done")
	assert.NotEmpty(t, grill.Actions[0].Reason)

	// Cook the three burgers: start all together (a batch, across one call).
	ids := []string{o.Items[1].ID, o.Items[2].ID, o.Items[3].ID}
	k.clk.Advance(time.Minute)
	moved, err := k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: ids})
	require.NoError(t, err)
	require.Len(t, moved, 1)
	for _, id := range ids {
		it, _ := moved[0].Item(id)
		assert.Equal(t, orders.StateCooking, it.State)
		require.NotNil(t, it.StartedAt)
	}
	// A second cook taps Start on the same burgers from a screen that has
	// not refreshed: nothing moves, and the error says who started them.
	_, err = k.svc.MoveItems(ctx, k.chef, ItemMove{ItemIDs: ids, From: orders.StateFired})
	require.Error(t, err)
	assert.Equal(t, KindConflict, KindOf(err))
	assert.Contains(t, err.Error(), "Synthetic burger for Table 4 is already cooking (Liam Cook, 10:01)")
	o, err = k.svc.GetOrder(ctx, k.cook, o.ID)
	require.NoError(t, err)
	for _, id := range ids {
		it, _ := o.Item(id)
		assert.Equal(t, orders.StateCooking, it.State, "the second tap moved nothing")
	}
	// One stale item in a batch stops the whole batch.
	_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: append([]string{o.Items[0].ID}, ids...), From: orders.StateCooking})
	assert.ErrorContains(t, err, "Synthetic salad for Table 4 is already held")
	_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: ids, From: "burnt"})
	assert.ErrorContains(t, err, "unknown state")

	// A cook cannot serve.
	_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: ids, To: orders.StateServed})
	assert.Equal(t, KindForbidden, KindOf(err))
	// Undo one start, then start it again.
	_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: ids[:1], To: orders.StateFired})
	require.NoError(t, err)
	_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: ids[:1]})
	require.NoError(t, err)

	// Serving refuses until every item is ready.
	_, err = k.svc.ServeCourse(ctx, k.chef, o.ID, menu.CourseMain)
	assert.ErrorContains(t, err, "not everything is ready")

	// Cooking → resting → plating → ready, one step per call.
	for range 3 {
		k.clk.Advance(4 * time.Minute)
		_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: ids})
		require.NoError(t, err)
	}
	o, err = k.svc.GetOrder(ctx, k.cook, o.ID)
	require.NoError(t, err)
	for _, id := range ids {
		it, _ := o.Item(id)
		assert.Equal(t, orders.StateReady, it.State)
		require.NotNil(t, it.ReadyAt)
	}
	view, err = k.svc.Kitchen(ctx, k.cook)
	require.NoError(t, err)
	assert.Equal(t, scheduler.StatusReady, view.Plan.Courses[0].Status)

	// Closing refuses while the starter is not served.
	o, err = k.svc.ServeCourse(ctx, k.chef, o.ID, menu.CourseMain)
	require.NoError(t, err)
	main, _ = o.Course(menu.CourseMain)
	assert.Equal(t, orders.CourseServed, main.State)
	_, err = k.svc.CloseOrder(ctx, k.chef, o.ID)
	assert.ErrorContains(t, err, "not served or voided")

	// Void the starter (needs a reason), then close.
	_, err = k.svc.MoveItems(ctx, k.chef, ItemMove{ItemIDs: []string{o.Items[0].ID}, To: orders.StateVoided})
	assert.ErrorContains(t, err, "say why")
	_, err = k.svc.MoveItems(ctx, k.chef, ItemMove{ItemIDs: []string{o.Items[0].ID}, To: orders.StateVoided, Reason: "Synthetic: customer changed mind"})
	require.NoError(t, err)
	o, err = k.svc.CloseOrder(ctx, k.chef, o.ID)
	require.NoError(t, err)
	assert.Equal(t, orders.StatusClosed, o.Status)
	_, err = k.svc.AddItems(ctx, k.foh, o.ID, []NewItem{{DishID: k.salad.ID}})
	assert.ErrorContains(t, err, "closed")

	// The history has every step, with who did it.
	evs, err := k.svc.OrderEvents(ctx, k.chef, o.ID)
	require.NoError(t, err)
	kinds := map[orders.EventKind]int{}
	for _, e := range evs {
		kinds[e.Kind]++
		assert.NotEmpty(t, e.ByName)
	}
	assert.Equal(t, 1, kinds[orders.EventCreated])
	assert.Equal(t, 1, kinds[orders.EventFired])
	assert.Equal(t, 1, kinds[orders.EventServed])
	assert.Equal(t, 1, kinds[orders.EventClosed])
	// 3 starts, an undo and a restart, 3 × 3 steps to ready, 3 served, 1 void.
	assert.Equal(t, 3+2+9+3+1, kinds[orders.EventItemState])

	open, err := k.svc.ListOrders(ctx, k.cook, OrderFilter{})
	require.NoError(t, err)
	assert.Empty(t, open)
	day, err := k.svc.ListOrders(ctx, k.cook, OrderFilter{Date: "2026-05-01"})
	require.NoError(t, err)
	assert.Len(t, day, 1)
}

func TestOverridesAndTakeaway(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()

	// A takeaway entered by the head chef and fired at once.
	o, err := k.svc.CreateOrder(ctx, k.chef, NewOrder{
		Kind: orders.KindTakeaway, Name: "Synthetic customer", Table: "ignored", Allergy: true,
		Items: []NewItem{{DishID: k.burger.ID}}, Fire: []menu.Course{menu.CourseMain},
	})
	require.NoError(t, err)
	assert.Empty(t, o.Table, "a takeaway has no table")
	assert.Equal(t, "Takeaway #1 (Synthetic customer)", o.Label())
	main, _ := o.Course(menu.CourseMain)
	assert.Equal(t, orders.CourseFired, main.State)

	// Front of house cannot fire on entry.
	_, err = k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: "2", Items: []NewItem{{DishID: k.salad.ID}}, Fire: []menu.Course{menu.CourseStarter}})
	assert.Equal(t, KindForbidden, KindOf(err))

	// The second order of the day is number 2.
	o2, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: "2", Items: []NewItem{{DishID: k.salad.ID}}})
	require.NoError(t, err)
	assert.Equal(t, 2, o2.Number)

	// Move the target: logged with its reason.
	target := start.Add(30 * time.Minute)
	o, err = k.svc.SetTarget(ctx, k.chef, o.ID, menu.CourseMain, target, "Customer arriving later")
	require.NoError(t, err)
	main, _ = o.Course(menu.CourseMain)
	assert.Equal(t, target, *main.TargetAt)
	assert.True(t, main.TargetSet)
	_, err = k.svc.SetTarget(ctx, k.chef, o.ID, menu.CourseMain, start.Add(-time.Minute), "")
	assert.ErrorContains(t, err, "future")
	_, err = k.svc.SetTarget(ctx, k.chef, o2.ID, menu.CourseStarter, target, "")
	assert.ErrorContains(t, err, "only a fired course")

	o, err = k.svc.SetUrgent(ctx, k.chef, o.ID, menu.CourseMain, true)
	require.NoError(t, err)
	main, _ = o.Course(menu.CourseMain)
	assert.True(t, main.Urgent)

	evs, err := k.svc.OrderEvents(ctx, k.chef, o.ID)
	require.NoError(t, err)
	var target0 *orders.Event
	for i := range evs {
		if evs[i].Kind == orders.EventTarget {
			target0 = &evs[i]
		}
	}
	require.NotNil(t, target0)
	assert.Contains(t, target0.Detail, "Customer arriving later")
	assert.Equal(t, "Aoife Chef", target0.ByName)

	// Items added to a fired course are fired straight away.
	o, err = k.svc.AddItems(ctx, k.foh, o.ID, []NewItem{{DishID: k.burger.ID}})
	require.NoError(t, err)
	assert.Equal(t, orders.StateFired, o.Items[1].State)

	// The plan names the takeaway as a priority.
	view, err := k.svc.Kitchen(ctx, k.cook)
	require.NoError(t, err)
	require.Len(t, view.Plan.Courses, 1)
	found := false
	for _, sp := range view.Plan.Stations {
		for _, a := range sp.Actions {
			if a.DishID == k.burger.ID {
				found = true
				assert.Contains(t, a.Reason, "priority")
			}
		}
	}
	assert.True(t, found)
}

func TestSchedulerSettings(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	cur, err := k.svc.Scheduler().Current(ctx, k.cook)
	require.NoError(t, err)
	assert.Equal(t, scheduler.StrategyBackward, cur.Value.Strategy)

	next := cur.Value
	next.Strategy = scheduler.StrategyFIFO
	next.StationBuffers = []scheduler.StationBuffer{{StationID: k.grill.ID, Seconds: 30}}
	pv, err := k.svc.Scheduler().Preview(ctx, k.manager, SettingsChange[scheduler.Settings]{Value: next, Reason: "Quiet lunch"})
	require.NoError(t, err)
	assert.NotEmpty(t, pv.Changes)
	assert.Contains(t, pv.Notes, "Grill: buffer 0 s → 30 s.", "buffer changes name the station")
	_, err = k.svc.Scheduler().Save(ctx, k.cook, SettingsChange[scheduler.Settings]{Value: next, Reason: "x"})
	assert.Equal(t, KindForbidden, KindOf(err))
	saved, err := k.svc.Scheduler().Save(ctx, k.manager, SettingsChange[scheduler.Settings]{Value: next, Reason: "Quiet lunch"})
	require.NoError(t, err)

	view, err := k.svc.Kitchen(ctx, k.cook)
	require.NoError(t, err)
	assert.Equal(t, scheduler.StrategyFIFO, view.Plan.Strategy)
	assert.Equal(t, saved.Version.ID, view.Plan.SettingsVersion, "every plan says which settings produced it")
}
