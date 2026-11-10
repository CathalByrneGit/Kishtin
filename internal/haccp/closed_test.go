package haccp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClosedDays(t *testing.T) {
	c := ClosedDays{
		Weekdays: []string{" Tue", "mon", "MON"},
		Dates:    []ClosedDate{{From: "2026-12-24", To: "2026-12-26", Note: " Christmas "}, {From: "2026-03-17"}},
	}.Normalize()
	require.NoError(t, c.Validate())
	assert.Equal(t, []string{"mon", "tue"}, c.Weekdays)
	assert.Equal(t, ClosedDate{From: "2026-03-17", To: "2026-03-17"}, c.Dates[0], "sorted, and a single day ends when it starts")
	assert.Equal(t, "Christmas", c.Dates[1].Note)

	for day, want := range map[string]bool{
		"2026-11-09": true,  // a Monday
		"2026-11-10": true,  // Tuesday
		"2026-11-11": false, // Wednesday
		"2026-12-23": false,
		"2026-12-24": true,
		"2026-12-25": true,
		"2026-12-26": true,
		"2026-12-27": false, // Sunday
		"2026-03-17": true,
		"not a day":  false,
	} {
		assert.Equal(t, want, c.On(day), day)
	}
	assert.False(t, ClosedDays{}.On("2026-11-09"))

	for name, bad := range map[string]ClosedDays{
		"unknown weekday": {Weekdays: []string{"funday"}},
		"every day":       {Weekdays: Weekdays},
		"bad date":        {Dates: []ClosedDate{{From: "25/12/2026", To: "25/12/2026"}}},
		"backwards":       {Dates: []ClosedDate{{From: "2026-12-26", To: "2026-12-24"}}},
		"too long":        {Dates: []ClosedDate{{From: "2026-01-01", To: "2028-01-01"}}},
	} {
		assert.Error(t, bad.Normalize().Validate(), name)
	}
}

func TestTodayOnAClosedDay(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 11, 9, 15, 0, 0, 0, loc) // a Monday afternoon
	lists := DefaultLists()
	open := BuildToday(TodayInput{Now: now, Loc: loc, Lists: lists})
	require.Positive(t, open.Total)
	assert.False(t, open.Closed)

	lists.Closed = ClosedDays{Weekdays: []string{"mon"}}
	closed := BuildToday(TodayInput{Now: now, Loc: loc, Lists: lists})
	assert.True(t, closed.Closed)
	assert.Zero(t, closed.Total, "nothing is due")
	for _, u := range closed.Units {
		for _, s := range u.Slots {
			assert.Equal(t, SlotClosed, s.Status, "%s %s", u.Name, s.Label)
		}
	}

	// A check logged anyway still counts.
	temp := 3.0
	rec := Record{ID: "r1", Seq: 1, Type: RecordFridge, Outcome: OutcomePass, LoggedAt: now.Add(-time.Hour), Reading: Reading{Unit: lists.Units[0].Name, TempC: &temp}}
	logged := BuildToday(TodayInput{Now: now, Loc: loc, Lists: lists, Records: []Record{rec}})
	assert.Equal(t, 1, logged.Total)
	assert.Equal(t, 1, logged.Done)
}
