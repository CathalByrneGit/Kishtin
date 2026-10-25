package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// NewItem is a dish to add to an order.
type NewItem struct {
	DishID string `json:"dish_id"`
	// Course defaults to the dish's own course.
	Course    menu.Course `json:"course,omitempty"`
	Modifiers []string    `json:"modifiers,omitempty"`
	Notes     string      `json:"notes,omitempty"`
	// Quantity adds that many items; each is cooked and tracked on its own.
	// 0 means 1.
	Quantity int `json:"quantity,omitempty"`
}

// NewOrder is an order to enter.
type NewOrder struct {
	Kind    orders.Kind `json:"kind" enum:"table,takeaway"`
	Table   string      `json:"table,omitempty"`
	Name    string      `json:"name,omitempty"`
	Covers  int         `json:"covers,omitempty"`
	Notes   string      `json:"notes,omitempty"`
	Allergy bool        `json:"allergy,omitempty"`
	Items   []NewItem   `json:"items"`
	// Fire lists courses to fire straight away, such as every course of a
	// takeaway.
	Fire []menu.Course `json:"fire,omitempty"`
}

// ItemMove moves items to a state. Items can belong to different orders,
// such as three burgers for tables 4 and 7 put on together.
type ItemMove struct {
	ItemIDs []string `json:"item_ids" minItems:"1"`
	// To is the state to move to. Empty moves each item one step forward.
	To orders.State `json:"to,omitempty"`
	// From is the state the person saw each item in. If an item has
	// already moved on (someone else tapped first, or the screen was out
	// of date), nothing moves and the error says who moved it and when.
	// Screens always send it; leave it out only from scripts that do not
	// care.
	From orders.State `json:"from,omitempty"`
	// Reason is required to void an item.
	Reason string `json:"reason,omitempty"`
}

const maxItemsPerOrder = 200

// dublinDay returns the start of today in Dublin.
func (s *Service) dublinDay() time.Time {
	now := s.clk.Now().In(clock.Dublin)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, clock.Dublin)
}

func (s *Service) orderChanged(id string) { s.publish(events.KindOrder, id) }

func (s *Service) event(p *auth.Principal, o orders.Order, kind orders.EventKind) orders.Event {
	return orders.Event{ID: newID(), OrderID: o.ID, Kind: kind, At: s.clk.Now(), By: p.User.ID, ByName: p.User.Name}
}

// buildItems turns new items into order items, checking each dish is on
// the menu and available.
func (s *Service) buildItems(setup menu.Setup, o *orders.Order, in []NewItem, now time.Time) ([]orders.Item, error) {
	var out []orders.Item
	pos := len(o.Items)
	for _, ni := range in {
		d, ok := setup.Dish(ni.DishID)
		if !ok || !d.Active {
			return nil, newErr(KindInvalid, "dish %s is not on the menu", ni.DishID)
		}
		if !d.Available {
			return nil, newErr(KindConflict, "%s is sold out (86)", d.Name)
		}
		course := ni.Course
		if course == "" {
			course = d.Course
		}
		if !course.Valid() {
			return nil, newErr(KindInvalid, "course must be one of starter, main, dessert, not %q", course)
		}
		if c, ok := o.Course(course); ok && c.State == orders.CourseServed {
			return nil, newErr(KindConflict, "%s has already been served: add %s to another course", course, d.Name)
		}
		timing, err := d.TimingFor(ni.Modifiers)
		if err != nil {
			return nil, newErr(KindInvalid, "%s", err)
		}
		if len([]rune(ni.Notes)) > 200 {
			return nil, newErr(KindInvalid, "item notes are at most 200 characters")
		}
		mods := make([]string, 0, len(ni.Modifiers))
		for _, m := range ni.Modifiers {
			i := slices.IndexFunc(d.Modifiers, func(x menu.Modifier) bool { return strings.EqualFold(x.Name, strings.TrimSpace(m)) })
			mods = append(mods, d.Modifiers[i].Name)
		}
		qty := ni.Quantity
		if qty == 0 {
			qty = 1
		}
		if qty < 0 || qty > 50 {
			return nil, newErr(KindInvalid, "quantity must be between 1 and 50")
		}
		for range qty {
			pos++
			out = append(out, orders.Item{
				ID: newID(), OrderID: o.ID, Position: pos, DishID: d.ID, DishName: d.Name, Course: course,
				Modifiers: mods, Notes: strings.TrimSpace(ni.Notes), State: orders.StateHeld,
				StationID: d.StationID, EquipmentID: d.EquipmentID, BatchCapacity: setup.Capacity(d),
				HoldSeconds: d.HoldSeconds, Batchable: d.Batchable, Timing: timing, StateAt: now,
			})
		}
	}
	if len(o.Items)+len(out) > maxItemsPerOrder {
		return nil, newErr(KindInvalid, "an order has at most %d items", maxItemsPerOrder)
	}
	return out, nil
}

// CreateOrder enters an order. Its courses are held until the expo fires
// them, unless listed in Fire.
func (s *Service) CreateOrder(ctx context.Context, p *auth.Principal, in NewOrder) (orders.Order, error) {
	if err := allow(p, auth.ActTakeOrders); err != nil {
		return orders.Order{}, err
	}
	if len(in.Fire) > 0 {
		if err := allow(p, auth.ActRunPass); err != nil {
			return orders.Order{}, newErr(KindForbidden, "only the head chef or a manager can fire courses")
		}
	}
	if len(in.Items) == 0 {
		return orders.Order{}, newErr(KindInvalid, "an order needs at least one item")
	}
	var out orders.Order
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		setup, err := menuSetup(ctx, tx)
		if err != nil {
			return err
		}
		n, err := tx.Orders().MaxOrderNumberSince(ctx, s.dublinDay())
		if err != nil {
			return err
		}
		now := s.clk.Now()
		o := orders.Order{
			ID: newID(), Number: n + 1, Kind: in.Kind, Table: strings.TrimSpace(in.Table), Name: strings.TrimSpace(in.Name),
			Covers: in.Covers, Notes: strings.TrimSpace(in.Notes), Allergy: in.Allergy, Status: orders.StatusOpen,
			CreatedBy: p.User.ID, CreatedAt: now, Courses: []orders.Course{}, Items: []orders.Item{},
		}
		if o.Kind == orders.KindTakeaway {
			o.Table = ""
		}
		if err := o.Validate(); err != nil {
			return newErr(KindInvalid, "%s", err)
		}
		items, err := s.buildItems(setup, &o, in.Items, now)
		if err != nil {
			return err
		}
		o.Items = items
		for _, c := range menu.Courses {
			if len(o.ItemsIn(c)) > 0 {
				o.Courses = append(o.Courses, orders.Course{OrderID: o.ID, Course: c, State: orders.CourseHeld})
			}
		}
		if err := tx.Orders().CreateOrder(ctx, o); err != nil {
			return err
		}
		ev := s.event(p, o, orders.EventCreated)
		ev.Detail = fmt.Sprintf("%s, %d items", o.Label(), len(o.Items))
		if err := tx.Orders().AppendEvent(ctx, ev); err != nil {
			return err
		}
		for _, c := range in.Fire {
			if err := s.fire(ctx, tx, p, &o, c); err != nil {
				return err
			}
		}
		out = o
		return nil
	})
	if err != nil {
		return orders.Order{}, err
	}
	s.orderChanged(out.ID)
	return out, nil
}

// AddItems adds items to an open order. Items added to a fired course are
// fired straight away.
func (s *Service) AddItems(ctx context.Context, p *auth.Principal, orderID string, in []NewItem) (orders.Order, error) {
	if err := allow(p, auth.ActTakeOrders); err != nil {
		return orders.Order{}, err
	}
	if len(in) == 0 {
		return orders.Order{}, newErr(KindInvalid, "add at least one item")
	}
	return s.changeOrder(ctx, orderID, func(tx store.Store, o *orders.Order) error {
		setup, err := menuSetup(ctx, tx)
		if err != nil {
			return err
		}
		now := s.clk.Now()
		items, err := s.buildItems(setup, o, in, now)
		if err != nil {
			return err
		}
		for _, it := range items {
			c, ok := o.Course(it.Course)
			if !ok {
				c = orders.Course{OrderID: o.ID, Course: it.Course, State: orders.CourseHeld}
				if err := tx.Orders().SaveCourse(ctx, c); err != nil {
					return err
				}
				o.Courses = append(o.Courses, c)
			}
			if c.State == orders.CourseFired {
				it = orders.Move(it, orders.StateFired, now)
			}
			if err := tx.Orders().AddItem(ctx, it); err != nil {
				return err
			}
			o.Items = append(o.Items, it)
			ev := s.event(p, *o, orders.EventItemAdded)
			ev.ItemID, ev.Course, ev.To, ev.Detail = it.ID, it.Course, string(it.State), it.DishName
			if err := tx.Orders().AppendEvent(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	})
}

// changeOrder runs fn on an open order in a transaction, then publishes the
// change and returns the order as saved.
func (s *Service) changeOrder(ctx context.Context, orderID string, fn func(tx store.Store, o *orders.Order) error) (orders.Order, error) {
	var out orders.Order
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		o, err := tx.Orders().GetOrder(ctx, orderID)
		if err != nil {
			return mapStoreErr(err, "order")
		}
		if o.Status != orders.StatusOpen {
			return newErr(KindConflict, "%s is closed", o.Label())
		}
		if err := fn(tx, &o); err != nil {
			return err
		}
		out, err = tx.Orders().GetOrder(ctx, orderID)
		return err
	})
	if err != nil {
		return orders.Order{}, err
	}
	s.orderChanged(orderID)
	return out, nil
}

// courseNamed checks a course name and returns the order's course.
func courseNamed(o *orders.Order, name menu.Course) (orders.Course, error) {
	if !name.Valid() {
		return orders.Course{}, newErr(KindInvalid, "course must be one of starter, main, dessert, not %q", name)
	}
	c, ok := o.Course(name)
	if !ok {
		return orders.Course{}, newErr(KindNotFound, "%s has no %s course", o.Label(), name)
	}
	return c, nil
}

// fire fires a course: its items go to the stations and its target time is
// set to now plus the longest item's time.
func (s *Service) fire(ctx context.Context, tx store.Store, p *auth.Principal, o *orders.Order, name menu.Course) error {
	c, err := courseNamed(o, name)
	if err != nil {
		return err
	}
	if c.State != orders.CourseHeld {
		return newErr(KindConflict, "%s %s is already %s", o.Label(), name, c.State)
	}
	items := o.ItemsIn(name)
	longest := orders.LongestTotal(items)
	if longest == 0 && orders.CourseDone(items) {
		return newErr(KindConflict, "%s %s has nothing left to cook", o.Label(), name)
	}
	now := s.clk.Now()
	target := now.Add(longest)
	c.State, c.FiredAt, c.TargetAt = orders.CourseFired, &now, &target
	if err := tx.Orders().SaveCourse(ctx, c); err != nil {
		return err
	}
	for i, it := range o.Items {
		if it.Course != name || it.State != orders.StateHeld {
			continue
		}
		moved := orders.Move(it, orders.StateFired, now)
		if err := tx.Orders().UpdateItem(ctx, moved); err != nil {
			return err
		}
		o.Items[i] = moved
	}
	for i := range o.Courses {
		if o.Courses[i].Course == name {
			o.Courses[i] = c
		}
	}
	ev := s.event(p, *o, orders.EventFired)
	ev.Course, ev.To = name, target.Format(time.RFC3339)
	ev.Detail = "target " + target.In(clock.Dublin).Format("15:04")
	return tx.Orders().AppendEvent(ctx, ev)
}

// FireCourse fires a course.
func (s *Service) FireCourse(ctx context.Context, p *auth.Principal, orderID string, course menu.Course) (orders.Order, error) {
	if err := allow(p, auth.ActRunPass); err != nil {
		return orders.Order{}, err
	}
	return s.changeOrder(ctx, orderID, func(tx store.Store, o *orders.Order) error {
		return s.fire(ctx, tx, p, o, course)
	})
}

// HoldCourse holds a fired course again, as if it had never been fired:
// its items leave the station queues and its target time is cleared. It
// refuses once any item has started, since that food is already cooking;
// void or replace those items instead. Logged with the reason.
func (s *Service) HoldCourse(ctx context.Context, p *auth.Principal, orderID string, course menu.Course, reason string) (orders.Order, error) {
	if err := allow(p, auth.ActRunPass); err != nil {
		return orders.Order{}, err
	}
	return s.changeOrder(ctx, orderID, func(tx store.Store, o *orders.Order) error {
		c, err := courseNamed(o, course)
		if err != nil {
			return err
		}
		if c.State != orders.CourseFired {
			return newErr(KindConflict, "only a fired course can be held again; %s %s is %s", o.Label(), course, c.State)
		}
		var started []string
		for _, it := range o.ItemsIn(course) {
			if it.State.Started() {
				started = append(started, fmt.Sprintf("%s (%s)", it.DishName, it.State))
			}
		}
		if len(started) > 0 {
			return newErr(KindConflict, "%s %s cannot be held again: already started: %s. Void or replace those items instead.",
				o.Label(), course, strings.Join(started, ", "))
		}
		now := s.clk.Now()
		for i, it := range o.Items {
			if it.Course != course || it.State != orders.StateFired {
				continue
			}
			held := orders.Move(it, orders.StateHeld, now)
			if err := tx.Orders().UpdateItem(ctx, held); err != nil {
				return err
			}
			o.Items[i] = held
		}
		from := c.TargetAt
		c.State, c.FiredAt, c.TargetAt, c.TargetSet = orders.CourseHeld, nil, nil, false
		if err := tx.Orders().SaveCourse(ctx, c); err != nil {
			return err
		}
		ev := s.event(p, *o, orders.EventHeld)
		ev.Course = course
		if from != nil {
			ev.From = from.Format(time.RFC3339)
		}
		ev.Detail = "held again"
		if r := strings.TrimSpace(reason); r != "" {
			ev.Detail += ": " + r
		}
		return tx.Orders().AppendEvent(ctx, ev)
	})
}

// ItemReplace swaps an ordered item for the right one, such as a burger
// that should have been well done.
type ItemReplace struct {
	// Item is what to add instead. Its course defaults to the old item's
	// course; the quantity defaults to 1.
	Item NewItem `json:"item"`
	// Reason is logged with the void. Empty logs "replaced".
	Reason string `json:"reason,omitempty"`
}

// ReplaceItem voids an item and adds the right one in its place, in one
// step: both happen or neither does. The new item joins the same course;
// if that course is fired, it goes straight to its station. Front of house
// can replace items in a held course; once a course is fired it is the
// pass's call, as for any void.
func (s *Service) ReplaceItem(ctx context.Context, p *auth.Principal, itemID string, in ItemReplace) (orders.Order, error) {
	if err := allow(p, auth.ActTakeOrders); err != nil {
		return orders.Order{}, err
	}
	o, err := s.orderOfItem(ctx, itemID)
	if err != nil {
		return orders.Order{}, err
	}
	return s.changeOrder(ctx, o.ID, func(tx store.Store, o *orders.Order) error {
		old, ok := o.Item(itemID)
		if !ok {
			return newErr(KindNotFound, "item not found")
		}
		if old.State.Done() {
			return newErr(KindConflict, "%s for %s is already %s", old.DishName, o.Label(), old.State)
		}
		if old.State != orders.StateHeld {
			if err := allow(p, auth.ActRunPass); err != nil {
				return newErr(KindForbidden, "%s %s is fired: only the head chef or a manager can change it now", o.Label(), old.Course)
			}
		}
		ni := in.Item
		if ni.Course == "" {
			ni.Course = old.Course
		}
		setup, err := menuSetup(ctx, tx)
		if err != nil {
			return err
		}
		now := s.clk.Now()
		items, err := s.buildItems(setup, o, []NewItem{ni}, now)
		if err != nil {
			return err
		}
		reason := strings.TrimSpace(in.Reason)
		if len([]rune(reason)) > 200 {
			return newErr(KindInvalid, "the reason is at most 200 characters")
		}
		if reason == "" {
			reason = "replaced"
		}
		voided := orders.Move(old, orders.StateVoided, now)
		voided.VoidReason = fmt.Sprintf("%s by %s", reason, itemWords(items[0]))
		if err := tx.Orders().UpdateItem(ctx, voided); err != nil {
			return err
		}
		for i := range o.Items {
			if o.Items[i].ID == old.ID {
				o.Items[i] = voided
			}
		}
		ev := s.event(p, *o, orders.EventItemState)
		ev.ItemID, ev.Course, ev.From, ev.To = old.ID, old.Course, string(old.State), string(orders.StateVoided)
		ev.Detail = old.DishName + ": " + voided.VoidReason
		if err := tx.Orders().AppendEvent(ctx, ev); err != nil {
			return err
		}
		for _, it := range items {
			c, ok := o.Course(it.Course)
			if !ok {
				c = orders.Course{OrderID: o.ID, Course: it.Course, State: orders.CourseHeld}
				if err := tx.Orders().SaveCourse(ctx, c); err != nil {
					return err
				}
				o.Courses = append(o.Courses, c)
			}
			if c.State == orders.CourseServed {
				return newErr(KindConflict, "%s has already been served", it.Course)
			}
			if c.State == orders.CourseFired {
				it = orders.Move(it, orders.StateFired, now)
			}
			if err := tx.Orders().AddItem(ctx, it); err != nil {
				return err
			}
			o.Items = append(o.Items, it)
			ev := s.event(p, *o, orders.EventItemAdded)
			ev.ItemID, ev.Course, ev.To = it.ID, it.Course, string(it.State)
			ev.Detail = itemWords(it) + ", replaces " + itemWords(old)
			if err := tx.Orders().AppendEvent(ctx, ev); err != nil {
				return err
			}
		}
		// Voiding the last unserved item can finish a course.
		return s.settleCourse(ctx, tx, p, o, old.Course)
	})
}

// itemWords names an item with its modifiers: "Burger (Well done)".
func itemWords(it orders.Item) string {
	if len(it.Modifiers) == 0 {
		return it.DishName
	}
	return it.DishName + " (" + strings.Join(it.Modifiers, ", ") + ")"
}

// orderOfItem finds the open order an item is on.
func (s *Service) orderOfItem(ctx context.Context, itemID string) (orders.Order, error) {
	open, err := s.st.Orders().ListOrders(ctx, store.OrderFilter{OpenOnly: true})
	if err != nil {
		return orders.Order{}, err
	}
	for _, o := range open {
		if _, ok := o.Item(itemID); ok {
			return o, nil
		}
	}
	return orders.Order{}, newErr(KindNotFound, "item %s is not on an open order", itemID)
}

// SetTarget changes when a fired course should reach the pass. It is an
// expo override: logged with its reason, and the scheduler re-plans around
// it.
func (s *Service) SetTarget(ctx context.Context, p *auth.Principal, orderID string, course menu.Course, at time.Time, reason string) (orders.Order, error) {
	if err := allow(p, auth.ActRunPass); err != nil {
		return orders.Order{}, err
	}
	return s.changeOrder(ctx, orderID, func(tx store.Store, o *orders.Order) error {
		c, err := courseNamed(o, course)
		if err != nil {
			return err
		}
		if c.State != orders.CourseFired {
			return newErr(KindConflict, "only a fired course has a target time; %s %s is %s", o.Label(), course, c.State)
		}
		now := s.clk.Now()
		if !at.After(now) {
			return newErr(KindInvalid, "the target must be in the future")
		}
		if at.After(now.Add(4 * time.Hour)) {
			return newErr(KindInvalid, "the target must be within 4 hours")
		}
		from := *c.TargetAt
		at = at.UTC().Truncate(time.Second)
		c.TargetAt, c.TargetSet = &at, true
		if err := tx.Orders().SaveCourse(ctx, c); err != nil {
			return err
		}
		ev := s.event(p, *o, orders.EventTarget)
		ev.Course, ev.From, ev.To = course, from.Format(time.RFC3339), at.Format(time.RFC3339)
		ev.Detail = fmt.Sprintf("from %s to %s", from.In(clock.Dublin).Format("15:04"), at.In(clock.Dublin).Format("15:04"))
		if r := strings.TrimSpace(reason); r != "" {
			ev.Detail += ": " + r
		}
		return tx.Orders().AppendEvent(ctx, ev)
	})
}

// SetUrgent flags a course so the scheduler puts it first, or clears the
// flag. It is logged as an expo override.
func (s *Service) SetUrgent(ctx context.Context, p *auth.Principal, orderID string, course menu.Course, urgent bool) (orders.Order, error) {
	if err := allow(p, auth.ActRunPass); err != nil {
		return orders.Order{}, err
	}
	return s.changeOrder(ctx, orderID, func(tx store.Store, o *orders.Order) error {
		c, err := courseNamed(o, course)
		if err != nil {
			return err
		}
		if c.State == orders.CourseServed {
			return newErr(KindConflict, "%s %s has been served", o.Label(), course)
		}
		if c.Urgent == urgent {
			return nil
		}
		c.Urgent = urgent
		if err := tx.Orders().SaveCourse(ctx, c); err != nil {
			return err
		}
		ev := s.event(p, *o, orders.EventUrgent)
		ev.Course, ev.To = course, fmt.Sprint(urgent)
		return tx.Orders().AppendEvent(ctx, ev)
	})
}

// moveAction is the action a move to state to needs: stations cook, the
// pass serves, voids and holds.
func moveAction(from, to orders.State) auth.Action {
	switch to {
	case orders.StateServed, orders.StateVoided:
		return auth.ActRunPass
	case orders.StateFired:
		if from == orders.StateHeld {
			return auth.ActRunPass
		}
	}
	return auth.ActWorkStation
}

// MoveItems moves items to a state: started, rested, plated, ready,
// served, voided, or one step back to undo a mistake. Every move is logged
// with who and when; these times become the actual cook times.
func (s *Service) MoveItems(ctx context.Context, p *auth.Principal, in ItemMove) ([]orders.Order, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return nil, err
	}
	if len(in.ItemIDs) == 0 {
		return nil, newErr(KindInvalid, "choose at least one item")
	}
	if in.To == orders.StateVoided && strings.TrimSpace(in.Reason) == "" {
		return nil, newErr(KindInvalid, "say why the item is voided")
	}
	if in.To != "" && !in.To.Valid() {
		return nil, newErr(KindInvalid, "unknown state %q", in.To)
	}
	if in.From != "" && !in.From.Valid() {
		return nil, newErr(KindInvalid, "unknown state %q", in.From)
	}
	var changed []string
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		open, err := tx.Orders().ListOrders(ctx, store.OrderFilter{OpenOnly: true})
		if err != nil {
			return err
		}
		byItem := map[string]*orders.Order{}
		for i := range open {
			for _, it := range open[i].Items {
				byItem[it.ID] = &open[i]
			}
		}
		now := s.clk.Now()
		seen := map[string]bool{}
		for _, id := range in.ItemIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			o, ok := byItem[id]
			if !ok {
				return newErr(KindNotFound, "item %s is not on an open order", id)
			}
			it, _ := o.Item(id)
			if in.From != "" && it.State != in.From {
				return s.alreadyMoved(ctx, tx, o, it)
			}
			to := in.To
			if to == "" {
				if to, err = orders.Advance(it); err != nil {
					return newErr(KindConflict, "%s", err)
				}
			}
			if c, _ := o.Course(it.Course); c.State == orders.CourseHeld && to != orders.StateVoided {
				return newErr(KindConflict, "%s %s is held: fire it first", o.Label(), it.Course)
			}
			if err := allow(p, moveAction(it.State, to)); err != nil {
				return err
			}
			if err := orders.CanMove(it, to); err != nil {
				return newErr(KindConflict, "%s", err)
			}
			moved := orders.Move(it, to, now)
			if to == orders.StateVoided {
				moved.VoidReason = strings.TrimSpace(in.Reason)
			}
			if err := tx.Orders().UpdateItem(ctx, moved); err != nil {
				return err
			}
			for i := range o.Items {
				if o.Items[i].ID == id {
					o.Items[i] = moved
				}
			}
			ev := s.event(p, *o, orders.EventItemState)
			ev.ItemID, ev.Course, ev.From, ev.To = id, it.Course, string(it.State), string(to)
			ev.Detail = it.DishName
			if to == orders.StateVoided {
				ev.Detail += ": " + moved.VoidReason
			}
			if err := tx.Orders().AppendEvent(ctx, ev); err != nil {
				return err
			}
			if err := s.settleCourse(ctx, tx, p, o, it.Course); err != nil {
				return err
			}
			if !slices.Contains(changed, o.ID) {
				changed = append(changed, o.ID)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]orders.Order, 0, len(changed))
	for _, id := range changed {
		s.orderChanged(id)
		o, err := s.st.Orders().GetOrder(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// alreadyMoved explains that an item is no longer in the state the person
// saw: who moved it, and when.
func (s *Service) alreadyMoved(ctx context.Context, tx store.Store, o *orders.Order, it orders.Item) error {
	what := fmt.Sprintf("%s for %s is already %s", it.DishName, o.Label(), itemStateWords(it.State))
	evs, err := tx.Orders().ListEvents(ctx, o.ID)
	if err != nil {
		return err
	}
	for i := len(evs) - 1; i >= 0; i-- {
		e := evs[i]
		if e.ItemID == it.ID && e.Kind == orders.EventItemState {
			what += fmt.Sprintf(" (%s, %s)", e.ByName, e.At.In(clock.Dublin).Format("15:04"))
			break
		}
	}
	return newErr(KindConflict, "%s. Nothing was changed; the screen now shows the latest.", what)
}

// itemStateWords is a state as staff say it.
func itemStateWords(st orders.State) string {
	switch st {
	case orders.StateFired:
		return "waiting to start"
	case orders.StateHeld:
		return "held"
	}
	return string(st)
}

// settleCourse marks a fired course served once every item in it is served
// or voided.
func (s *Service) settleCourse(ctx context.Context, tx store.Store, p *auth.Principal, o *orders.Order, name menu.Course) error {
	c, ok := o.Course(name)
	if !ok || c.State != orders.CourseFired || !orders.CourseDone(o.ItemsIn(name)) {
		return nil
	}
	now := s.clk.Now()
	c.State, c.ServedAt = orders.CourseServed, &now
	if err := tx.Orders().SaveCourse(ctx, c); err != nil {
		return err
	}
	for i := range o.Courses {
		if o.Courses[i].Course == name {
			o.Courses[i] = c
		}
	}
	ev := s.event(p, *o, orders.EventServed)
	ev.Course = name
	return tx.Orders().AppendEvent(ctx, ev)
}

// ReadyCourse marks a fired course ready at the pass: every started item
// still cooking, resting or plating goes to ready, as when the expo sees
// the plates arrive. It refuses while any item has not been started, so
// nothing is marked ready that no one cooked.
func (s *Service) ReadyCourse(ctx context.Context, p *auth.Principal, orderID string, course menu.Course) (orders.Order, error) {
	if err := allow(p, auth.ActRunPass); err != nil {
		return orders.Order{}, err
	}
	return s.changeOrder(ctx, orderID, func(tx store.Store, o *orders.Order) error {
		c, err := courseNamed(o, course)
		if err != nil {
			return err
		}
		if c.State != orders.CourseFired {
			return newErr(KindConflict, "%s %s is %s, not fired", o.Label(), course, c.State)
		}
		var waiting []string
		var moving []int
		for i, it := range o.Items {
			if it.Course != course {
				continue
			}
			switch it.State {
			case orders.StateFired, orders.StateHeld:
				waiting = append(waiting, it.DishName)
			case orders.StateCooking, orders.StateResting, orders.StatePlating:
				moving = append(moving, i)
			}
		}
		if len(waiting) > 0 {
			return newErr(KindConflict, "%s %s is not all started: %s. Nothing was changed.", o.Label(), course, strings.Join(waiting, ", "))
		}
		if len(moving) == 0 {
			return newErr(KindConflict, "%s %s is already ready", o.Label(), course)
		}
		now := s.clk.Now()
		for _, i := range moving {
			it := o.Items[i]
			ready := orders.Move(it, orders.StateReady, now)
			if err := tx.Orders().UpdateItem(ctx, ready); err != nil {
				return err
			}
			o.Items[i] = ready
			ev := s.event(p, *o, orders.EventItemState)
			ev.ItemID, ev.Course, ev.From, ev.To = it.ID, course, string(it.State), string(orders.StateReady)
			ev.Detail = it.DishName + ": marked ready at the pass"
			if err := tx.Orders().AppendEvent(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	})
}

// ServeCourse serves every ready item in a course. It refuses while any
// item is still cooking, so a course leaves the pass together.
func (s *Service) ServeCourse(ctx context.Context, p *auth.Principal, orderID string, course menu.Course) (orders.Order, error) {
	if err := allow(p, auth.ActRunPass); err != nil {
		return orders.Order{}, err
	}
	o, err := s.st.Orders().GetOrder(ctx, orderID)
	if err != nil {
		return orders.Order{}, mapStoreErr(err, "order")
	}
	if _, err := courseNamed(&o, course); err != nil {
		return orders.Order{}, err
	}
	var ids, waiting []string
	for _, it := range o.ItemsIn(course) {
		switch it.State {
		case orders.StateReady:
			ids = append(ids, it.ID)
		case orders.StateServed, orders.StateVoided:
		default:
			waiting = append(waiting, fmt.Sprintf("%s (%s)", it.DishName, it.State))
		}
	}
	if len(waiting) > 0 {
		return orders.Order{}, newErr(KindConflict, "not everything is ready: %s", strings.Join(waiting, ", "))
	}
	if len(ids) == 0 {
		return orders.Order{}, newErr(KindConflict, "%s %s has nothing ready to serve", o.Label(), course)
	}
	out, err := s.MoveItems(ctx, p, ItemMove{ItemIDs: ids, To: orders.StateServed})
	if err != nil {
		return orders.Order{}, err
	}
	return out[0], nil
}

// RenameOrder sets what the kitchen calls an order, such as a guest's
// name, or clears it with an empty name. The change is logged.
func (s *Service) RenameOrder(ctx context.Context, p *auth.Principal, orderID, name string) (orders.Order, error) {
	if err := allow(p, auth.ActTakeOrders); err != nil {
		return orders.Order{}, err
	}
	return s.changeOrder(ctx, orderID, func(tx store.Store, o *orders.Order) error {
		name = strings.TrimSpace(name)
		if name == o.Name {
			return nil
		}
		from := o.Name
		o.Name = name
		if err := o.Validate(); err != nil {
			return newErr(KindInvalid, "%s", err)
		}
		if err := tx.Orders().UpdateOrder(ctx, *o); err != nil {
			return err
		}
		ev := s.event(p, *o, orders.EventRenamed)
		ev.From, ev.To = from, name
		switch {
		case from == "":
			ev.Detail = "named " + name
		case name == "":
			ev.Detail = "name " + from + " cleared"
		default:
			ev.Detail = "renamed from " + from + " to " + name
		}
		return tx.Orders().AppendEvent(ctx, ev)
	})
}

// CloseOrder closes an order once everything is served or voided.
func (s *Service) CloseOrder(ctx context.Context, p *auth.Principal, orderID string) (orders.Order, error) {
	if err := allow(p, auth.ActRunPass); err != nil {
		return orders.Order{}, err
	}
	return s.changeOrder(ctx, orderID, func(tx store.Store, o *orders.Order) error {
		var left []string
		for _, it := range o.Items {
			if !it.State.Done() {
				left = append(left, fmt.Sprintf("%s (%s)", it.DishName, it.State))
			}
		}
		if len(left) > 0 {
			return newErr(KindConflict, "%d items are not served or voided: %s", len(left), strings.Join(left, ", "))
		}
		now := s.clk.Now()
		o.Status, o.ClosedAt = orders.StatusClosed, &now
		if err := tx.Orders().UpdateOrder(ctx, *o); err != nil {
			return err
		}
		return tx.Orders().AppendEvent(ctx, s.event(p, *o, orders.EventClosed))
	})
}

// GetOrder returns one order.
func (s *Service) GetOrder(ctx context.Context, p *auth.Principal, id string) (orders.Order, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return orders.Order{}, err
	}
	o, err := s.st.Orders().GetOrder(ctx, id)
	if err != nil {
		return orders.Order{}, mapStoreErr(err, "order")
	}
	return o, nil
}

// OrderFilter selects orders to list.
type OrderFilter struct {
	// Date lists every order created that day (Dublin), YYYY-MM-DD. Empty
	// lists open orders.
	Date string
}

// ListOrders lists open orders, or every order of a day.
func (s *Service) ListOrders(ctx context.Context, p *auth.Principal, f OrderFilter) ([]orders.Order, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return nil, err
	}
	if f.Date == "" {
		return s.st.Orders().ListOrders(ctx, store.OrderFilter{OpenOnly: true})
	}
	d, err := time.ParseInLocation("2006-01-02", f.Date, clock.Dublin)
	if err != nil {
		return nil, newErr(KindInvalid, "date must be YYYY-MM-DD")
	}
	return s.st.Orders().ListOrders(ctx, store.OrderFilter{From: d, To: d.AddDate(0, 0, 1)})
}

// OrderEvents returns an order's history, oldest first.
func (s *Service) OrderEvents(ctx context.Context, p *auth.Principal, id string) ([]orders.Event, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return nil, err
	}
	if _, err := s.st.Orders().GetOrder(ctx, id); err != nil {
		return nil, mapStoreErr(err, "order")
	}
	return s.st.Orders().ListEvents(ctx, id)
}
