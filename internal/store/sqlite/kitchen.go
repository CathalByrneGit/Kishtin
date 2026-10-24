package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite/sqlitedb"
)

// Menu returns the menu store.
func (s *Store) Menu() store.MenuStore { return s }

// Orders returns the order store.
func (s *Store) Orders() store.OrderStore { return s }

func nullTime(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: us(*t), Valid: true}
}

func timePtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := fromUS(v.Int64)
	return &t
}

// rowsAffected turns an update that matched nothing into ErrNotFound.
func rowsAffected(n int64, err error) error {
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// --- Stations and equipment ---

func (s *Store) CreateStation(ctx context.Context, st menu.Station) error {
	return mapErr(s.w.InsertStation(ctx, sqlitedb.InsertStationParams{
		ID: st.ID, Name: st.Name, Position: int64(st.Position), Active: boolInt(st.Active),
		CreatedAt: us(st.CreatedAt), UpdatedAt: us(st.UpdatedAt),
	}))
}

func (s *Store) UpdateStation(ctx context.Context, st menu.Station) error {
	return rowsAffected(s.w.UpdateStation(ctx, sqlitedb.UpdateStationParams{
		Name: st.Name, Position: int64(st.Position), Active: boolInt(st.Active), UpdatedAt: us(st.UpdatedAt), ID: st.ID,
	}))
}

func (s *Store) ListStations(ctx context.Context) ([]menu.Station, error) {
	rows, err := s.r.ListStations(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]menu.Station, 0, len(rows))
	for _, r := range rows {
		out = append(out, menu.Station{
			ID: r.ID, Name: r.Name, Position: int(r.Position), Active: r.Active != 0,
			CreatedAt: fromUS(r.CreatedAt), UpdatedAt: fromUS(r.UpdatedAt),
		})
	}
	return out, nil
}

func (s *Store) CreateEquipment(ctx context.Context, e menu.Equipment) error {
	return mapErr(s.w.InsertEquipment(ctx, sqlitedb.InsertEquipmentParams{
		ID: e.ID, StationID: e.StationID, Name: e.Name, Slots: int64(e.Slots), Active: boolInt(e.Active),
		CreatedAt: us(e.CreatedAt), UpdatedAt: us(e.UpdatedAt),
	}))
}

func (s *Store) UpdateEquipment(ctx context.Context, e menu.Equipment) error {
	return rowsAffected(s.w.UpdateEquipment(ctx, sqlitedb.UpdateEquipmentParams{
		StationID: e.StationID, Name: e.Name, Slots: int64(e.Slots), Active: boolInt(e.Active), UpdatedAt: us(e.UpdatedAt), ID: e.ID,
	}))
}

func (s *Store) ListEquipment(ctx context.Context) ([]menu.Equipment, error) {
	rows, err := s.r.ListEquipment(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]menu.Equipment, 0, len(rows))
	for _, r := range rows {
		out = append(out, menu.Equipment{
			ID: r.ID, StationID: r.StationID, Name: r.Name, Slots: int(r.Slots), Active: r.Active != 0,
			CreatedAt: fromUS(r.CreatedAt), UpdatedAt: fromUS(r.UpdatedAt),
		})
	}
	return out, nil
}

// --- Dishes ---

func (s *Store) CreateDish(ctx context.Context, d menu.Dish) error {
	mods, err := json.Marshal(nonNilMods(d.Modifiers))
	if err != nil {
		return fmt.Errorf("encode modifiers: %w", err)
	}
	return mapErr(s.w.InsertDish(ctx, sqlitedb.InsertDishParams{
		ID: d.ID, Name: d.Name, Course: string(d.Course), StationID: d.StationID, EquipmentID: nullStr(d.EquipmentID),
		BatchCapacity: int64(d.BatchCapacity), CookSeconds: int64(d.CookSeconds), RestSeconds: int64(d.RestSeconds),
		PlateSeconds: int64(d.PlateSeconds), HoldSeconds: int64(d.HoldSeconds), Batchable: boolInt(d.Batchable),
		Modifiers: string(mods), FoodCategory: d.FoodCategory, Available: boolInt(d.Available), Active: boolInt(d.Active),
		CreatedAt: us(d.CreatedAt), UpdatedAt: us(d.UpdatedAt),
	}))
}

func (s *Store) UpdateDish(ctx context.Context, d menu.Dish) error {
	mods, err := json.Marshal(nonNilMods(d.Modifiers))
	if err != nil {
		return fmt.Errorf("encode modifiers: %w", err)
	}
	return rowsAffected(s.w.UpdateDish(ctx, sqlitedb.UpdateDishParams{
		Name: d.Name, Course: string(d.Course), StationID: d.StationID, EquipmentID: nullStr(d.EquipmentID),
		BatchCapacity: int64(d.BatchCapacity), CookSeconds: int64(d.CookSeconds), RestSeconds: int64(d.RestSeconds),
		PlateSeconds: int64(d.PlateSeconds), HoldSeconds: int64(d.HoldSeconds), Batchable: boolInt(d.Batchable),
		Modifiers: string(mods), FoodCategory: d.FoodCategory, Available: boolInt(d.Available), Active: boolInt(d.Active),
		UpdatedAt: us(d.UpdatedAt), ID: d.ID,
	}))
}

func nonNilMods(m []menu.Modifier) []menu.Modifier {
	if m == nil {
		return []menu.Modifier{}
	}
	return m
}

func dishFromRow(r sqlitedb.Dish) (menu.Dish, error) {
	var mods []menu.Modifier
	if err := json.Unmarshal([]byte(r.Modifiers), &mods); err != nil {
		return menu.Dish{}, fmt.Errorf("decode modifiers of dish %s: %w", r.ID, err)
	}
	return menu.Dish{
		ID: r.ID, Name: r.Name, Course: menu.Course(r.Course), StationID: r.StationID, EquipmentID: r.EquipmentID.String,
		BatchCapacity: int(r.BatchCapacity), CookSeconds: int(r.CookSeconds), RestSeconds: int(r.RestSeconds),
		PlateSeconds: int(r.PlateSeconds), HoldSeconds: int(r.HoldSeconds), Batchable: r.Batchable != 0,
		Modifiers: nonNilMods(mods), FoodCategory: r.FoodCategory, Available: r.Available != 0, Active: r.Active != 0,
		CreatedAt: fromUS(r.CreatedAt), UpdatedAt: fromUS(r.UpdatedAt),
	}, nil
}

func (s *Store) GetDish(ctx context.Context, id string) (menu.Dish, error) {
	r, err := s.r.GetDish(ctx, id)
	if err != nil {
		return menu.Dish{}, mapErr(err)
	}
	return dishFromRow(r)
}

func (s *Store) ListDishes(ctx context.Context) ([]menu.Dish, error) {
	rows, err := s.r.ListDishes(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]menu.Dish, 0, len(rows))
	for _, r := range rows {
		d, err := dishFromRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// --- Orders ---

func (s *Store) CreateOrder(ctx context.Context, o orders.Order) error {
	err := s.w.InsertOrder(ctx, sqlitedb.InsertOrderParams{
		ID: o.ID, Number: int64(o.Number), Kind: string(o.Kind), TableName: o.Table, Name: o.Name,
		Covers: int64(o.Covers), Notes: o.Notes, Allergy: boolInt(o.Allergy), Status: string(o.Status),
		CreatedBy: o.CreatedBy, CreatedAt: us(o.CreatedAt), ClosedAt: nullTime(o.ClosedAt),
	})
	if err != nil {
		return mapErr(err)
	}
	for _, c := range o.Courses {
		if err := s.SaveCourse(ctx, c); err != nil {
			return err
		}
	}
	for _, it := range o.Items {
		if err := s.AddItem(ctx, it); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpdateOrder(ctx context.Context, o orders.Order) error {
	return rowsAffected(s.w.UpdateOrder(ctx, sqlitedb.UpdateOrderParams{
		TableName: o.Table, Name: o.Name, Covers: int64(o.Covers), Notes: o.Notes, Allergy: boolInt(o.Allergy),
		Status: string(o.Status), ClosedAt: nullTime(o.ClosedAt), ID: o.ID,
	}))
}

func (s *Store) fillOrder(ctx context.Context, r sqlitedb.Order) (orders.Order, error) {
	o := orders.Order{
		ID: r.ID, Number: int(r.Number), Kind: orders.Kind(r.Kind), Table: r.TableName, Name: r.Name,
		Covers: int(r.Covers), Notes: r.Notes, Allergy: r.Allergy != 0, Status: orders.Status(r.Status),
		CreatedBy: r.CreatedBy, CreatedAt: fromUS(r.CreatedAt), ClosedAt: timePtr(r.ClosedAt),
		Courses: []orders.Course{}, Items: []orders.Item{},
	}
	courses, err := s.r.ListCourses(ctx, r.ID)
	if err != nil {
		return o, mapErr(err)
	}
	for _, c := range courses {
		o.Courses = append(o.Courses, orders.Course{
			OrderID: c.OrderID, Course: menu.Course(c.Course), State: orders.CourseState(c.State),
			FiredAt: timePtr(c.FiredAt), TargetAt: timePtr(c.TargetAt), TargetSet: c.TargetSet != 0,
			Urgent: c.Urgent != 0, ServedAt: timePtr(c.ServedAt),
		})
	}
	// Courses in service order, whatever order the database returns.
	sortCourses(o.Courses)
	items, err := s.r.ListItems(ctx, r.ID)
	if err != nil {
		return o, mapErr(err)
	}
	for _, it := range items {
		var mods []string
		if err := json.Unmarshal([]byte(it.Modifiers), &mods); err != nil {
			return o, fmt.Errorf("decode modifiers of item %s: %w", it.ID, err)
		}
		if mods == nil {
			mods = []string{}
		}
		o.Items = append(o.Items, orders.Item{
			ID: it.ID, OrderID: it.OrderID, Position: int(it.Position), DishID: it.DishID, DishName: it.DishName,
			Course: menu.Course(it.Course), Modifiers: mods, Notes: it.Notes, State: orders.State(it.State),
			StationID: it.StationID, EquipmentID: it.EquipmentID, BatchCapacity: int(it.BatchCapacity),
			HoldSeconds: int(it.HoldSeconds), Batchable: it.Batchable != 0,
			Timing: menu.Timing{
				Cook:  time.Duration(it.CookMs) * time.Millisecond,
				Rest:  time.Duration(it.RestMs) * time.Millisecond,
				Plate: time.Duration(it.PlateMs) * time.Millisecond,
			},
			StateAt: fromUS(it.StateAt), StartedAt: timePtr(it.StartedAt), ReadyAt: timePtr(it.ReadyAt),
			ServedAt: timePtr(it.ServedAt), VoidReason: it.VoidReason,
		})
	}
	return o, nil
}

func sortCourses(cs []orders.Course) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j].Course.Order() < cs[j-1].Course.Order(); j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}

func (s *Store) GetOrder(ctx context.Context, id string) (orders.Order, error) {
	r, err := s.r.GetOrder(ctx, id)
	if err != nil {
		return orders.Order{}, mapErr(err)
	}
	return s.fillOrder(ctx, r)
}

func (s *Store) ListOrders(ctx context.Context, f store.OrderFilter) ([]orders.Order, error) {
	var rows []sqlitedb.Order
	var err error
	if f.OpenOnly {
		rows, err = s.r.ListOpenOrders(ctx)
	} else {
		if f.From.IsZero() || f.To.IsZero() {
			return nil, fmt.Errorf("list orders: set From and To, or OpenOnly")
		}
		rows, err = s.r.ListOrdersBetween(ctx, sqlitedb.ListOrdersBetweenParams{CreatedAt: us(f.From), CreatedAt_2: us(f.To)})
	}
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]orders.Order, 0, len(rows))
	for _, r := range rows {
		o, err := s.fillOrder(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func (s *Store) MaxOrderNumberSince(ctx context.Context, t time.Time) (int, error) {
	n, err := s.r.MaxOrderNumberSince(ctx, us(t))
	return int(n), mapErr(err)
}

func (s *Store) SaveCourse(ctx context.Context, c orders.Course) error {
	return mapErr(s.w.UpsertCourse(ctx, sqlitedb.UpsertCourseParams{
		OrderID: c.OrderID, Course: string(c.Course), State: string(c.State), FiredAt: nullTime(c.FiredAt),
		TargetAt: nullTime(c.TargetAt), TargetSet: boolInt(c.TargetSet), Urgent: boolInt(c.Urgent), ServedAt: nullTime(c.ServedAt),
	}))
}

func encodeStrings(v []string) (string, error) {
	if v == nil {
		v = []string{}
	}
	b, err := json.Marshal(v)
	return string(b), err
}

func (s *Store) AddItem(ctx context.Context, it orders.Item) error {
	mods, err := encodeStrings(it.Modifiers)
	if err != nil {
		return fmt.Errorf("encode modifiers: %w", err)
	}
	return mapErr(s.w.InsertItem(ctx, sqlitedb.InsertItemParams{
		ID: it.ID, OrderID: it.OrderID, Position: int64(it.Position), DishID: it.DishID, DishName: it.DishName,
		Course: string(it.Course), Modifiers: mods, Notes: it.Notes, State: string(it.State), StationID: it.StationID,
		EquipmentID: it.EquipmentID, BatchCapacity: int64(it.BatchCapacity), HoldSeconds: int64(it.HoldSeconds),
		Batchable: boolInt(it.Batchable), CookMs: it.Timing.Cook.Milliseconds(), RestMs: it.Timing.Rest.Milliseconds(),
		PlateMs: it.Timing.Plate.Milliseconds(), StateAt: us(it.StateAt), StartedAt: nullTime(it.StartedAt),
		ReadyAt: nullTime(it.ReadyAt), ServedAt: nullTime(it.ServedAt), VoidReason: it.VoidReason,
	}))
}

func (s *Store) UpdateItem(ctx context.Context, it orders.Item) error {
	mods, err := encodeStrings(it.Modifiers)
	if err != nil {
		return fmt.Errorf("encode modifiers: %w", err)
	}
	return rowsAffected(s.w.UpdateItem(ctx, sqlitedb.UpdateItemParams{
		Course: string(it.Course), Modifiers: mods, Notes: it.Notes, State: string(it.State), StateAt: us(it.StateAt),
		StartedAt: nullTime(it.StartedAt), ReadyAt: nullTime(it.ReadyAt), ServedAt: nullTime(it.ServedAt),
		VoidReason: it.VoidReason, ID: it.ID,
	}))
}

func (s *Store) AppendEvent(ctx context.Context, e orders.Event) error {
	return mapErr(s.w.InsertOrderEvent(ctx, sqlitedb.InsertOrderEventParams{
		ID: e.ID, OrderID: e.OrderID, ItemID: e.ItemID, Course: string(e.Course), Kind: string(e.Kind),
		FromVal: e.From, ToVal: e.To, Detail: e.Detail, At: us(e.At), ByUser: e.By, ByName: e.ByName,
	}))
}

func eventFromRow(r sqlitedb.OrderEvent) orders.Event {
	return orders.Event{
		ID: r.ID, OrderID: r.OrderID, ItemID: r.ItemID, Course: menu.Course(r.Course), Kind: orders.EventKind(r.Kind),
		From: r.FromVal, To: r.ToVal, Detail: r.Detail, At: fromUS(r.At), By: r.ByUser, ByName: r.ByName,
	}
}

func (s *Store) ListEvents(ctx context.Context, orderID string) ([]orders.Event, error) {
	rows, err := s.r.ListOrderEvents(ctx, orderID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]orders.Event, 0, len(rows))
	for _, r := range rows {
		out = append(out, eventFromRow(r))
	}
	return out, nil
}

func (s *Store) ListItemEvents(ctx context.Context, from, to time.Time) ([]orders.Event, error) {
	rows, err := s.r.ListItemEventsBetween(ctx, sqlitedb.ListItemEventsBetweenParams{At: us(from), At_2: us(to)})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]orders.Event, 0, len(rows))
	for _, r := range rows {
		out = append(out, eventFromRow(r))
	}
	return out, nil
}
