package storetest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

func kitchenTests() map[string]func(*testing.T, store.Store) {
	return map[string]func(*testing.T, store.Store){
		"MenuRoundTrip":       testMenuRoundTrip,
		"MenuUniqueNames":     testMenuUniqueNames,
		"OrderRoundTrip":      testOrderRoundTrip,
		"OrderListAndNumbers": testOrderListAndNumbers,
		"OrderEvents":         testOrderEvents,
		"NoOrderEventUpdates": testNoOrderEventUpdates,
	}
}

func sampleMenu(t *testing.T, s store.Store) (menu.Station, menu.Equipment, menu.Dish) {
	t.Helper()
	ctx := context.Background()
	st := menu.Station{ID: "st-grill", Name: "Grill", Position: 1, Active: true, CreatedAt: t0, UpdatedAt: t0}
	require.NoError(t, s.Menu().CreateStation(ctx, st))
	eq := menu.Equipment{ID: "eq-flat", StationID: st.ID, Name: "Flat-top", Slots: 3, Active: true, CreatedAt: t0, UpdatedAt: t0}
	require.NoError(t, s.Menu().CreateEquipment(ctx, eq))
	d := menu.Dish{
		ID: "dish-burger", Name: "Synthetic burger", Course: menu.CourseMain, StationID: st.ID, EquipmentID: eq.ID,
		BatchCapacity: 6, CookSeconds: 480, RestSeconds: 120, PlateSeconds: 60, HoldSeconds: 180, Batchable: true,
		Modifiers: []menu.Modifier{{Name: "Well done", AddSeconds: 120}}, FoodCategory: "minced meat",
		Available: true, Active: true, CreatedAt: t0, UpdatedAt: t0,
	}
	require.NoError(t, s.Menu().CreateDish(ctx, d))
	return st, eq, d
}

func testMenuRoundTrip(t *testing.T, s store.Store) {
	ctx := context.Background()
	st, eq, d := sampleMenu(t, s)

	stations, err := s.Menu().ListStations(ctx)
	require.NoError(t, err)
	assert.Equal(t, []menu.Station{st}, stations)
	eqs, err := s.Menu().ListEquipment(ctx)
	require.NoError(t, err)
	assert.Equal(t, []menu.Equipment{eq}, eqs)
	got, err := s.Menu().GetDish(ctx, d.ID)
	require.NoError(t, err)
	assert.Equal(t, d, got)

	// A dish with no equipment and no modifiers.
	salad := menu.Dish{
		ID: "dish-salad", Name: "Synthetic salad", Course: menu.CourseStarter, StationID: st.ID, PlateSeconds: 120,
		Modifiers: []menu.Modifier{}, Available: true, Active: true, CreatedAt: t0, UpdatedAt: t0,
	}
	require.NoError(t, s.Menu().CreateDish(ctx, salad))
	dishes, err := s.Menu().ListDishes(ctx)
	require.NoError(t, err)
	assert.Equal(t, []menu.Dish{d, salad}, dishes, "ordered by name")

	st.Name, st.Active, st.UpdatedAt = "Grill station", false, t0.Add(time.Hour)
	require.NoError(t, s.Menu().UpdateStation(ctx, st))
	eq.Slots = 4
	require.NoError(t, s.Menu().UpdateEquipment(ctx, eq))
	d.Available, d.CookSeconds, d.Modifiers = false, 500, nil
	require.NoError(t, s.Menu().UpdateDish(ctx, d))
	got, err = s.Menu().GetDish(ctx, d.ID)
	require.NoError(t, err)
	assert.False(t, got.Available)
	assert.Equal(t, 500, got.CookSeconds)
	assert.Equal(t, []menu.Modifier{}, got.Modifiers)

	stations, err = s.Menu().ListStations(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Grill station", stations[0].Name)
	assert.False(t, stations[0].Active)

	missing := d
	missing.ID = "nope"
	assert.ErrorIs(t, s.Menu().UpdateDish(ctx, missing), store.ErrNotFound)
	_, err = s.Menu().GetDish(ctx, "nope")
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.ErrorIs(t, s.Menu().UpdateStation(ctx, menu.Station{ID: "nope", Name: "x"}), store.ErrNotFound)
}

func testMenuUniqueNames(t *testing.T, s store.Store) {
	ctx := context.Background()
	st, eq, d := sampleMenu(t, s)
	dup := st
	dup.ID, dup.Name = "st-2", "GRILL"
	assert.ErrorIs(t, s.Menu().CreateStation(ctx, dup), store.ErrConflict, "station names ignore case")
	dupEq := eq
	dupEq.ID, dupEq.Name = "eq-2", "flat-top"
	assert.ErrorIs(t, s.Menu().CreateEquipment(ctx, dupEq), store.ErrConflict)
	dupD := d
	dupD.ID, dupD.Name = "dish-2", "synthetic BURGER"
	assert.ErrorIs(t, s.Menu().CreateDish(ctx, dupD), store.ErrConflict)
}

func sampleOrder(id string, number int, at time.Time) orders.Order {
	fired := at.Add(time.Minute)
	target := fired.Add(11 * time.Minute)
	return orders.Order{
		ID: id, Number: number, Kind: orders.KindTable, Table: "4", Covers: 2, Notes: "Synthetic note",
		Allergy: true, Status: orders.StatusOpen, CreatedBy: "u1", CreatedAt: at,
		Courses: []orders.Course{
			{OrderID: id, Course: menu.CourseStarter, State: orders.CourseServed, FiredAt: &at, TargetAt: &at, ServedAt: &at},
			{OrderID: id, Course: menu.CourseMain, State: orders.CourseFired, FiredAt: &fired, TargetAt: &target, Urgent: true},
		},
		Items: []orders.Item{
			{
				ID: id + "-i1", OrderID: id, Position: 1, DishID: "dish-burger", DishName: "Synthetic burger",
				Course: menu.CourseMain, Modifiers: []string{"Well done"}, Notes: "no onion", State: orders.StateCooking,
				StationID: "st-grill", EquipmentID: "eq-flat", BatchCapacity: 6, HoldSeconds: 180, Batchable: true,
				Timing:  menu.Timing{Cook: 10 * time.Minute, Rest: 2 * time.Minute, Plate: time.Minute},
				StateAt: fired, StartedAt: &fired,
			},
			{
				ID: id + "-i2", OrderID: id, Position: 2, DishID: "dish-burger", DishName: "Synthetic burger",
				Course: menu.CourseMain, Modifiers: []string{}, State: orders.StateFired, StationID: "st-grill",
				Timing: menu.Timing{Cook: 8 * time.Minute}, StateAt: fired,
			},
		},
	}
}

func testOrderRoundTrip(t *testing.T, s store.Store) {
	ctx := context.Background()
	sampleMenu(t, s)
	o := sampleOrder("ord-1", 1, t0)
	require.NoError(t, s.Orders().CreateOrder(ctx, o))
	got, err := s.Orders().GetOrder(ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, o, got)

	// Update the order, a course and an item.
	closed := t0.Add(2 * time.Hour)
	o.Status, o.ClosedAt, o.Table = orders.StatusClosed, &closed, "5"
	require.NoError(t, s.Orders().UpdateOrder(ctx, o))
	c := o.Courses[1]
	c.State, c.ServedAt, c.Urgent = orders.CourseServed, &closed, false
	require.NoError(t, s.Orders().SaveCourse(ctx, c))
	it := orders.Move(o.Items[1], orders.StateCooking, closed)
	require.NoError(t, s.Orders().UpdateItem(ctx, it))
	o.Courses[1], o.Items[1] = c, it

	got, err = s.Orders().GetOrder(ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, o, got)

	_, err = s.Orders().GetOrder(ctx, "nope")
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.ErrorIs(t, s.Orders().UpdateItem(ctx, orders.Item{ID: "nope"}), store.ErrNotFound)
	assert.ErrorIs(t, s.Orders().AddItem(ctx, o.Items[0]), store.ErrConflict, "item IDs are unique")
}

func testOrderListAndNumbers(t *testing.T, s store.Store) {
	ctx := context.Background()
	sampleMenu(t, s)
	day1 := t0
	day2 := t0.Add(24 * time.Hour)
	for i, o := range []orders.Order{sampleOrder("a", 1, day1), sampleOrder("b", 2, day1.Add(time.Hour)), sampleOrder("c", 1, day2)} {
		if i == 1 {
			o.Status = orders.StatusClosed
		}
		require.NoError(t, s.Orders().CreateOrder(ctx, o))
	}
	open, err := s.Orders().ListOrders(ctx, store.OrderFilter{OpenOnly: true})
	require.NoError(t, err)
	require.Len(t, open, 2)
	assert.Equal(t, "a", open[0].ID)
	assert.Equal(t, "c", open[1].ID)
	assert.Len(t, open[0].Items, 2, "list fills items")

	between, err := s.Orders().ListOrders(ctx, store.OrderFilter{From: day1, To: day2})
	require.NoError(t, err)
	assert.Len(t, between, 2)

	n, err := s.Orders().MaxOrderNumberSince(ctx, day1)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	n, err = s.Orders().MaxOrderNumberSince(ctx, day2.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func testOrderEvents(t *testing.T, s store.Store) {
	ctx := context.Background()
	sampleMenu(t, s)
	require.NoError(t, s.Orders().CreateOrder(ctx, sampleOrder("ord-1", 1, t0)))
	evs := []orders.Event{
		{ID: "e1", OrderID: "ord-1", Kind: orders.EventCreated, At: t0, By: "u1", ByName: "Synthetic Server"},
		{ID: "e2", OrderID: "ord-1", ItemID: "ord-1-i2", Course: menu.CourseMain, Kind: orders.EventItemState, From: "fired", To: "cooking", At: t0.Add(time.Minute), By: "u2", ByName: "Synthetic Cook"},
		{ID: "e3", OrderID: "ord-1", Course: menu.CourseMain, Kind: orders.EventTarget, Detail: "moved to 19:30", At: t0.Add(2 * time.Minute), By: "u3", ByName: "Synthetic Expo"},
	}
	for _, e := range evs {
		require.NoError(t, s.Orders().AppendEvent(ctx, e))
	}
	assert.ErrorIs(t, s.Orders().AppendEvent(ctx, evs[0]), store.ErrConflict)
	got, err := s.Orders().ListEvents(ctx, "ord-1")
	require.NoError(t, err)
	assert.Equal(t, evs, got)

	items, err := s.Orders().ListItemEvents(ctx, t0, t0.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, []orders.Event{evs[1]}, items)
}

func testNoOrderEventUpdates(t *testing.T, s store.Store) {
	typ := reflect.TypeOf(s.Orders())
	for _, name := range []string{"UpdateEvent", "DeleteEvent", "DeleteOrder", "DeleteItem"} {
		_, has := typ.MethodByName(name)
		assert.False(t, has, "the order store must not have %s", name)
	}
	typ = reflect.TypeOf(s.Menu())
	for _, name := range []string{"DeleteDish", "DeleteStation", "DeleteEquipment"} {
		_, has := typ.MethodByName(name)
		assert.False(t, has, "the menu store must not have %s", name)
	}
}
