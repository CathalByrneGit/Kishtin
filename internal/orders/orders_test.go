package orders

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
)

var t0 = time.Date(2026, 5, 1, 18, 0, 0, 0, time.UTC)

func item(state State, cook, rest, plate time.Duration) Item {
	return Item{ID: "i1", DishName: "Burger", State: state, Timing: menu.Timing{Cook: cook, Rest: rest, Plate: plate}}
}

func TestNextStates(t *testing.T) {
	full := func(s State) []State { return NextStates(item(s, 8*time.Minute, 2*time.Minute, time.Minute)) }
	assert.Equal(t, []State{StateFired, StateVoided}, full(StateHeld))
	assert.Equal(t, []State{StateCooking, StateVoided}, full(StateFired))
	assert.Equal(t, []State{StateResting, StateFired, StateVoided}, full(StateCooking))
	assert.Equal(t, []State{StatePlating, StateCooking, StateVoided}, full(StateResting))
	assert.Equal(t, []State{StateReady, StateResting, StateVoided}, full(StatePlating))
	assert.Equal(t, []State{StateServed, StatePlating, StateVoided}, full(StateReady))
	assert.Empty(t, full(StateServed))
	assert.Empty(t, full(StateVoided))

	// No rest and no plate: cooking goes straight to ready, and back.
	fries := item(StateCooking, 3*time.Minute, 0, 0)
	assert.Equal(t, []State{StateReady, StateFired, StateVoided}, NextStates(fries))
	fries.State = StateReady
	assert.Equal(t, []State{StateServed, StateCooking, StateVoided}, NextStates(fries))
}

func TestAdvance(t *testing.T) {
	s, err := Advance(item(StateCooking, time.Minute, 0, time.Minute))
	require.NoError(t, err)
	assert.Equal(t, StatePlating, s)
	_, err = Advance(item(StateServed, time.Minute, 0, 0))
	assert.ErrorContains(t, err, "already served")
}

func TestCanMove(t *testing.T) {
	it := item(StateFired, time.Minute, 0, 0)
	assert.NoError(t, CanMove(it, StateCooking))
	assert.ErrorContains(t, CanMove(it, StateServed), "is fired and cannot go to served")
	assert.ErrorContains(t, CanMove(it, "burnt"), "unknown state")
}

func TestMoveRecordsTimes(t *testing.T) {
	it := item(StateFired, 8*time.Minute, 0, time.Minute)
	it = Move(it, StateCooking, t0)
	require.NotNil(t, it.StartedAt)
	assert.Equal(t, t0, *it.StartedAt)
	assert.Equal(t, t0, it.StateAt)

	t1 := t0.Add(9 * time.Minute)
	it = Move(Move(it, StatePlating, t0.Add(8*time.Minute)), StateReady, t1)
	require.NotNil(t, it.ReadyAt)
	assert.Equal(t, t1, *it.ReadyAt)

	// Undo ready: the ready time is cleared.
	it = Move(it, StatePlating, t1.Add(time.Second))
	assert.Nil(t, it.ReadyAt)

	// Undo the start: the start time is cleared.
	it = Move(Move(it, StateCooking, t1), StateFired, t1)
	assert.Nil(t, it.StartedAt)

	t2 := t1.Add(time.Minute)
	it = Move(Move(Move(it, StateCooking, t1), StateReady, t2), StateServed, t2)
	require.NotNil(t, it.ServedAt)
	assert.NotNil(t, it.ReadyAt, "serving keeps the ready time")
}

func TestOrderValidateAndLabel(t *testing.T) {
	o := Order{Kind: KindTable, Table: "4"}
	require.NoError(t, o.Validate())
	assert.Equal(t, "Table 4", o.Label())
	o.Name = "Aoife"
	assert.Equal(t, "Table 4 (Aoife)", o.Label())
	assert.Equal(t, "Table 4", o.Place())
	o.Name = ""

	assert.Error(t, Order{Kind: KindTable}.Validate())
	assert.Error(t, Order{Kind: "delivery"}.Validate())
	assert.Error(t, Order{Kind: KindTakeaway, Covers: -1}.Validate())

	tk := Order{Kind: KindTakeaway, Number: 12, Name: "Aoife"}
	require.NoError(t, tk.Validate())
	assert.Equal(t, "Takeaway #12 (Aoife)", tk.Label())
}

func TestLongestTotalAndDone(t *testing.T) {
	items := []Item{
		item(StateFired, 8*time.Minute, 2*time.Minute, time.Minute),
		item(StateVoided, 30*time.Minute, 0, 0),
		item(StateServed, 3*time.Minute, 0, 0),
	}
	assert.Equal(t, 11*time.Minute, LongestTotal(items))
	assert.False(t, CourseDone(items))
	items[0].State = StateServed
	assert.True(t, CourseDone(items))
}
