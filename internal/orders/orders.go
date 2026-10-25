// Package orders holds orders, their courses and items, and the rules for
// moving an item through the kitchen. It does no I/O.
package orders

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
)

// Kind is where an order is going.
type Kind string

const (
	KindTable    Kind = "table"
	KindTakeaway Kind = "takeaway"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool { return k == KindTable || k == KindTakeaway }

// Status is whether an order is still open.
type Status string

const (
	StatusOpen   Status = "open"
	StatusClosed Status = "closed"
)

// State is where an item is in the kitchen.
type State string

// Item states, in order. An item starts held, waiting for the expo to fire
// its course. Voided can be reached from any state before served.
const (
	StateHeld    State = "held"
	StateFired   State = "fired"
	StateCooking State = "cooking"
	StateResting State = "resting"
	StatePlating State = "plating"
	StateReady   State = "ready"
	StateServed  State = "served"
	StateVoided  State = "voided"
)

// States lists every state.
var States = []State{StateHeld, StateFired, StateCooking, StateResting, StatePlating, StateReady, StateServed, StateVoided}

// Valid reports whether s is a known state.
func (s State) Valid() bool { return slices.Contains(States, s) }

// Done reports whether the item needs nothing more from the kitchen.
func (s State) Done() bool { return s == StateServed || s == StateVoided }

// Started reports whether a cook has started the item.
func (s State) Started() bool {
	return s == StateCooking || s == StateResting || s == StatePlating || s == StateReady || s == StateServed
}

// CourseState is where a course is.
type CourseState string

const (
	// CourseHeld waits for the expo to fire it.
	CourseHeld CourseState = "held"
	// CourseFired is cooking: firing sets its target time.
	CourseFired CourseState = "fired"
	// CourseServed has left the pass.
	CourseServed CourseState = "served"
)

// Course is one course of an order.
type Course struct {
	OrderID string      `json:"order_id"`
	Course  menu.Course `json:"course"`
	State   CourseState `json:"state"`
	// FiredAt and TargetAt are set when the course is fired. TargetAt is
	// when every item should be at the pass together.
	FiredAt  *time.Time `json:"fired_at,omitempty"`
	TargetAt *time.Time `json:"target_at,omitempty"`
	// TargetSet is set when the expo chose the target by hand.
	TargetSet bool `json:"target_set,omitempty"`
	// Urgent asks the scheduler to put this course first.
	Urgent   bool       `json:"urgent,omitempty"`
	ServedAt *time.Time `json:"served_at,omitempty"`
}

// Item is one dish on an order, such as "1x burger, well done".
type Item struct {
	ID       string      `json:"id"`
	OrderID  string      `json:"order_id"`
	Position int         `json:"position"`
	DishID   string      `json:"dish_id"`
	DishName string      `json:"dish_name"`
	Course   menu.Course `json:"course"`
	// Modifiers are the dish modifiers chosen, such as "Well done".
	Modifiers []string `json:"modifiers"`
	Notes     string   `json:"notes,omitempty"`
	State     State    `json:"state"`
	// StationID, EquipmentID, BatchCapacity, HoldSeconds, Batchable and
	// the times are copied from the dish when the item is ordered, so a
	// menu change does not change an order already in the kitchen.
	StationID     string      `json:"station_id"`
	EquipmentID   string      `json:"equipment_id,omitempty"`
	BatchCapacity int         `json:"batch_capacity,omitempty"`
	HoldSeconds   int         `json:"hold_seconds"`
	Batchable     bool        `json:"batchable"`
	Timing        menu.Timing `json:"timing"`
	// StateAt is when the item entered its current state. With the state,
	// it gives how far through a phase the item is.
	StateAt    time.Time  `json:"state_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	ReadyAt    *time.Time `json:"ready_at,omitempty"`
	ServedAt   *time.Time `json:"served_at,omitempty"`
	VoidReason string     `json:"void_reason,omitempty"`
}

// Order is what a table or takeaway customer asked for.
type Order struct {
	ID string `json:"id"`
	// Number is the ticket number, counting from 1 each day.
	Number int    `json:"number"`
	Kind   Kind   `json:"kind"`
	Table  string `json:"table,omitempty"`
	// Name is the customer's name for a takeaway.
	Name   string `json:"name,omitempty"`
	Covers int    `json:"covers,omitempty"`
	Notes  string `json:"notes,omitempty"`
	// Allergy marks an order with an allergy, which the scheduler can put
	// first and every screen highlights.
	Allergy   bool       `json:"allergy,omitempty"`
	Status    Status     `json:"status"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
	Courses   []Course   `json:"courses"`
	Items     []Item     `json:"items"`
}

// Label is how screens name the order: "Table 4" or "Takeaway #12 (Aoife)".
func (o Order) Label() string {
	if o.Kind == KindTable {
		return "Table " + o.Table
	}
	l := fmt.Sprintf("Takeaway #%d", o.Number)
	if o.Name != "" {
		l += " (" + o.Name + ")"
	}
	return l
}

// Course returns the order's course c.
func (o Order) Course(c menu.Course) (Course, bool) {
	i := slices.IndexFunc(o.Courses, func(x Course) bool { return x.Course == c })
	if i < 0 {
		return Course{}, false
	}
	return o.Courses[i], true
}

// Item returns the item with the given ID.
func (o Order) Item(id string) (Item, bool) {
	i := slices.IndexFunc(o.Items, func(x Item) bool { return x.ID == id })
	if i < 0 {
		return Item{}, false
	}
	return o.Items[i], true
}

// ItemsIn returns the items in course c, in order.
func (o Order) ItemsIn(c menu.Course) []Item {
	var out []Item
	for _, it := range o.Items {
		if it.Course == c {
			out = append(out, it)
		}
	}
	return out
}

// Validate checks the order's own fields.
func (o Order) Validate() error {
	var errs []error
	if !o.Kind.Valid() {
		errs = append(errs, fmt.Errorf("kind must be table or takeaway, not %q", o.Kind))
	}
	if o.Kind == KindTable && strings.TrimSpace(o.Table) == "" {
		errs = append(errs, errors.New("a table order needs a table"))
	}
	if len([]rune(o.Table)) > 20 {
		errs = append(errs, errors.New("a table is at most 20 characters"))
	}
	if len([]rune(o.Name)) > 40 {
		errs = append(errs, errors.New("a name is at most 40 characters"))
	}
	if len([]rune(o.Notes)) > 500 {
		errs = append(errs, errors.New("notes are at most 500 characters"))
	}
	if o.Covers < 0 || o.Covers > 100 {
		errs = append(errs, errors.New("covers must be between 0 and 100"))
	}
	return errors.Join(errs...)
}

// NextStates returns where an item can go from its current state, given its
// timings: done cooking skips resting when there is no rest time, and so
// on. The previous state is included, to undo a tap made by mistake.
func NextStates(it Item) []State {
	var out []State
	switch it.State {
	case StateHeld:
		out = []State{StateFired}
	case StateFired:
		out = []State{StateCooking}
	case StateCooking:
		out = []State{after(it, StateCooking), StateFired}
	case StateResting:
		out = []State{after(it, StateResting), StateCooking}
	case StatePlating:
		out = []State{StateReady, before(it, StatePlating)}
	case StateReady:
		out = []State{StateServed, before(it, StateReady)}
	}
	if !it.State.Done() {
		out = append(out, StateVoided)
	}
	return out
}

// Advance returns the next state forward from the current one.
func Advance(it Item) (State, error) {
	switch it.State {
	case StateHeld:
		return StateFired, nil
	case StateFired:
		return StateCooking, nil
	case StateCooking, StateResting:
		return after(it, it.State), nil
	case StatePlating:
		return StateReady, nil
	case StateReady:
		return StateServed, nil
	}
	return "", fmt.Errorf("%s is already %s", it.DishName, it.State)
}

// after is the state that follows cooking or resting for this item.
func after(it Item, s State) State {
	if s == StateCooking && it.Timing.Rest > 0 {
		return StateResting
	}
	if it.Timing.Plate > 0 {
		return StatePlating
	}
	return StateReady
}

// before is the state that comes before plating or ready for this item.
func before(it Item, s State) State {
	if s == StateReady && it.Timing.Plate > 0 {
		return StatePlating
	}
	if it.Timing.Rest > 0 {
		return StateResting
	}
	if it.Timing.Cook > 0 {
		return StateCooking
	}
	return StateFired
}

// CanMove reports whether an item can go to state to, and if not, why.
func CanMove(it Item, to State) error {
	if !to.Valid() {
		return fmt.Errorf("unknown state %q", to)
	}
	if slices.Contains(NextStates(it), to) {
		return nil
	}
	return fmt.Errorf("%s is %s and cannot go to %s", it.DishName, it.State, to)
}

// Move puts an item into state to at time at, recording when it started,
// became ready and was served. It does not check the move: call CanMove
// first.
func Move(it Item, to State, at time.Time) Item {
	from := it.State
	it.State = to
	it.StateAt = at
	switch {
	case to == StateCooking && from == StateFired:
		it.StartedAt = &at
	case to == StateFired || to == StateHeld:
		it.StartedAt = nil
	}
	switch {
	case to == StateReady && from != StateServed:
		it.ReadyAt = &at
	case !to.Done() && to != StateReady:
		it.ReadyAt = nil
	}
	if to == StateServed {
		it.ServedAt = &at
	}
	// A cook that skips straight past cooking (nothing to cook) still
	// started.
	if it.StartedAt == nil && to.Started() {
		it.StartedAt = &at
	}
	return it
}

// CourseDone reports whether every item in the course is served or voided.
func CourseDone(items []Item) bool {
	for _, it := range items {
		if !it.State.Done() {
			return false
		}
	}
	return true
}

// LongestTotal is the longest start-to-pass time among items not voided.
func LongestTotal(items []Item) time.Duration {
	var d time.Duration
	for _, it := range items {
		if it.State != StateVoided && it.Timing.Total() > d {
			d = it.Timing.Total()
		}
	}
	return d
}

// EventKind is what an order event records.
type EventKind string

const (
	EventCreated   EventKind = "created"
	EventItemAdded EventKind = "item_added"
	EventItemState EventKind = "item_state"
	EventFired     EventKind = "fired"
	// EventHeld records a fired course held again before anything started.
	EventHeld   EventKind = "held"
	EventTarget EventKind = "target"
	EventUrgent EventKind = "urgent"
	EventServed EventKind = "served"
	EventClosed EventKind = "closed"
	// EventPOS records a change on the till the pass must deal with, such
	// as a line removed after it reached the kitchen.
	EventPOS EventKind = "pos"
)

// Event records something that happened to an order. Events are
// append-only: they are the order's history, the source of actual cook
// times, and the log of every expo override.
type Event struct {
	ID      string      `json:"id"`
	OrderID string      `json:"order_id"`
	ItemID  string      `json:"item_id,omitempty"`
	Course  menu.Course `json:"course,omitempty"`
	Kind    EventKind   `json:"kind"`
	From    string      `json:"from,omitempty"`
	To      string      `json:"to,omitempty"`
	// Detail says more in words, such as a void reason or "batched with
	// table 7".
	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at"`
	By     string    `json:"by"`
	ByName string    `json:"by_name"`
}
