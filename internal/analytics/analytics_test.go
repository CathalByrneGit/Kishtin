package analytics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/scheduler"
)

var loc = time.UTC

func at(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, loc) }
func fp(v float64) *float64    { return &v }

func TestSpread(t *testing.T) {
	var ds []time.Duration
	for i := 1; i <= 10; i++ {
		ds = append(ds, time.Duration(i)*time.Minute)
	}
	assert.Equal(t, Spread{Count: 10, Median: 300, P90: 540}, spread(ds))
	assert.Equal(t, Spread{}, spread(nil))
	assert.Equal(t, Spread{Count: 1, Median: 60, P90: 60}, spread(ds[:1]))
}

func TestBuildHACCP(t *testing.T) {
	units := []haccp.Unit{{Name: "Walk-in fridge", Kind: haccp.RecordFridge}, {Name: "Chest freezer", Kind: haccp.RecordFreezer}}
	var recs []haccp.Record
	seq := int64(0)
	add := func(r haccp.Record) {
		seq++
		r.Seq, r.ID = seq, string(rune('a'+seq))
		if r.LoggedAt.IsZero() {
			r.LoggedAt = at(19, 9, 0)
		}
		recs = append(recs, r)
	}
	// Mon 19 and Tue 20 October: the fridge is checked AM and PM both days,
	// the freezer only on the Monday morning. One fridge reading fails.
	for _, d := range []int{19, 20} {
		add(haccp.Record{Type: haccp.RecordFridge, LoggedAt: at(d, 9, 0), Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: fp(3)}, Outcome: haccp.OutcomePass})
		add(haccp.Record{Type: haccp.RecordFridge, LoggedAt: at(d, 15, 0), Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: fp(4.4)}, Outcome: haccp.OutcomePass})
	}
	add(haccp.Record{Type: haccp.RecordFridge, LoggedAt: at(20, 16, 0), Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: fp(7)}, Outcome: haccp.OutcomeFail, CorrectiveAction: "Door shut"})
	add(haccp.Record{Type: haccp.RecordFreezer, LoggedAt: at(19, 9, 5), Reading: haccp.Reading{Unit: "Chest freezer", TempC: fp(-19)}, Outcome: haccp.OutcomePass})
	done := true
	add(haccp.Record{Type: haccp.RecordCleaning, LoggedAt: at(19, 22, 0), Reading: haccp.Reading{Task: "Floors", Done: &done}, Outcome: haccp.OutcomePass})

	r := BuildHACCP(HACCPInput{
		From: at(19, 0, 0), To: at(20, 0, 0), Loc: loc, Now: at(21, 8, 0), Since: at(19, 9, 0),
		Records: recs, Units: units, CleaningTasks: []haccp.CleaningTask{{Task: "Floors"}, {Task: "Bins"}},
		Limits: haccp.FSAIDefaults(),
	})
	assert.Equal(t, 5, r.ChecksDone, "fridge 4 slots, freezer 1")
	assert.Equal(t, 3, r.ChecksMissed, "freezer Monday PM and all Tuesday")
	assert.Equal(t, 8, r.ChecksDue)
	assert.Equal(t, 63, r.ChecksPercent)
	require.Len(t, r.MissedByWeekday, 7)
	assert.Equal(t, Count{Key: "mon", Label: "Monday", Count: 1, Of: 4}, r.MissedByWeekday[0])
	assert.Equal(t, Count{Key: "tue", Label: "Tuesday", Count: 2, Of: 4}, r.MissedByWeekday[1])
	assert.Equal(t, []Count{{Key: "Chest freezer", Label: "Chest freezer", Count: 3}}, r.MissedByUnit)
	assert.Equal(t, 4, r.CleaningDue)
	assert.Equal(t, 1, r.CleaningDone)
	assert.Equal(t, []Count{{Key: "Bins", Label: "Bins", Count: 2}, {Key: "Floors", Label: "Floors", Count: 1}}, r.CleaningMissedByTask)
	assert.Equal(t, 1, r.OutOfLimit)
	assert.Equal(t, 0, r.WithoutAction)
	require.Len(t, r.Recent, 1)

	require.Len(t, r.Temperatures, 2)
	fridge := r.Temperatures[0]
	require.NotNil(t, fridge.MaxC)
	assert.InDelta(t, 5.0, *fridge.MaxC, 0.001)
	require.Len(t, fridge.Days, 2)
	assert.Equal(t, 3, fridge.Days[1].Count)
	assert.InDelta(t, 7.0, *fridge.Days[1].MaxC, 0.001)
	assert.InDelta(t, 4.8, *fridge.Days[1].AvgC, 0.001)
	assert.Nil(t, r.Temperatures[1].Days[1].MaxC, "no freezer reading on Tuesday")
}

func TestBuildPass(t *testing.T) {
	ptr := func(t time.Time) *time.Time { return &t }
	burger := menu.Timing{Cook: 8 * time.Minute, Rest: 2 * time.Minute, Plate: time.Minute}
	order := func(id string, fired, target, ready, served time.Time, started time.Time) orders.Order {
		return orders.Order{
			ID: id, Number: 1, Kind: orders.KindTable, Table: id,
			Courses: []orders.Course{{OrderID: id, Course: menu.CourseMain, State: orders.CourseServed, FiredAt: ptr(fired), TargetAt: ptr(target), ServedAt: ptr(served)}},
			Items: []orders.Item{
				{ID: id + "-1", OrderID: id, DishID: "burger", DishName: "Burger", Course: menu.CourseMain, State: orders.StateServed, Timing: burger, StartedAt: ptr(started), ReadyAt: ptr(ready)},
				{ID: id + "-2", OrderID: id, DishID: "burger", DishName: "Burger", Course: menu.CourseMain, State: orders.StateVoided, Timing: burger},
			},
		}
	}
	os := []orders.Order{
		// On time: fired 19:00, target 19:12, ready 19:12:30, served 19:14.
		order("4", at(20, 19, 0), at(20, 19, 12), at(20, 19, 12).Add(30*time.Second), at(20, 19, 14), at(20, 19, 1)),
		// Late by 6 minutes, the flat-top was full.
		order("7", at(20, 19, 5), at(20, 19, 17), at(20, 19, 23), at(20, 19, 24), at(20, 19, 10)),
		// Late, with no cause in any kept plan.
		order("9", at(20, 20, 0), at(20, 20, 11), at(20, 20, 15), at(20, 20, 16), at(20, 20, 2)),
		// Outside the period.
		order("2", at(18, 19, 0), at(18, 19, 11), at(18, 19, 30), at(18, 19, 31), at(18, 19, 1)),
	}
	still := order("5", at(20, 21, 0), at(20, 21, 11), at(20, 21, 11), at(20, 21, 11), at(20, 21, 0))
	still.Items[0].ReadyAt = nil
	os = append(os, still)

	seen := []CourseSeen{
		{At: at(20, 19, 8), CourseStatus: scheduler.CourseStatus{OrderID: "7", Course: menu.CourseMain, Status: scheduler.StatusAtRisk, Cause: scheduler.CauseEquipmentFull, Reason: "The flat-top is full until 19:10."}},
		{At: at(20, 19, 30), CourseStatus: scheduler.CourseStatus{OrderID: "7", Course: menu.CourseMain, Status: scheduler.StatusLate, Cause: scheduler.CauseWaitingAtPass}},
	}
	r := BuildPass(PassInput{From: at(20, 0, 0), To: at(20, 0, 0), Loc: loc, Orders: os, Seen: seen})
	assert.Equal(t, 3, r.Courses, "the course still cooking and the one before the period are left out")
	assert.Equal(t, 1, r.OnTime)
	assert.Equal(t, 2, r.Late)
	assert.Equal(t, 33, r.OnTimePercent)
	assert.Equal(t, Spread{Count: 3, Median: 900, P90: 1080}, r.Ticket)
	assert.Equal(t, Spread{Count: 3, Median: 60, P90: 90}, r.PassWait)
	assert.Equal(t, []Count{{Key: scheduler.CauseEquipmentFull, Label: "Equipment full", Count: 1}, {Key: CauseUnknown, Label: "No cause recorded", Count: 1}}, r.LateByCause)
	require.Len(t, r.LateCourses, 2)
	assert.Equal(t, "Table 9", r.LateCourses[0].Label, "latest first")
	assert.Equal(t, scheduler.CauseEquipmentFull, r.LateCourses[1].Cause, "a cause after the course was ready does not count")
	assert.Equal(t, 360, r.LateCourses[1].LateBy)
	require.Len(t, r.ByCourse, 1)
	assert.Equal(t, menu.CourseMain, r.ByCourse[0].Course)
	assert.Equal(t, []HourStats{{Hour: 19, Courses: 2, OnTime: 1}, {Hour: 20, Courses: 1}}, r.ByHour)
	require.Len(t, r.Dishes, 1)
	assert.Equal(t, 660, r.Dishes[0].SetSeconds)
	assert.Equal(t, 780, r.Dishes[0].Actual.Median)
	assert.Equal(t, 120, r.Dishes[0].OverSeconds)
}
