package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

func TestRecurringCleaningTasks(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	lists, err := f.svc.Lists().Current(ctx, f.manager)
	require.NoError(t, err)
	next := lists.Value
	next.CleaningTasks = []haccp.CleaningTask{{Task: "Wipe surfaces"}, {Task: "Clean beer lines", Area: "Cellar", EveryDays: 7}}
	_, err = f.svc.Lists().Save(ctx, f.manager, SettingsChange[haccp.Lists]{Value: next, Reason: "Weekly line cleaning"})
	require.NoError(t, err)

	lines := func() haccp.CleaningToday {
		t.Helper()
		v, err := f.svc.Today(ctx, f.cook)
		require.NoError(t, err)
		for _, c := range v.Cleaning {
			if c.Task == "Clean beer lines" {
				return c
			}
		}
		t.Fatal("no line cleaning task")
		return haccp.CleaningToday{}
	}
	day := func(n int) string { return start.AddDate(0, 0, n).Format(haccp.DateLayout) }

	c := lines()
	require.NotNil(t, c.Schedule)
	assert.Equal(t, haccp.TaskDue, c.Schedule.Status, "never done: due now")

	_, err = f.svc.LogRecord(ctx, f.cook, cleaning("clean beer lines"))
	require.NoError(t, err)
	c = lines()
	assert.Equal(t, haccp.TaskSchedule{LastDone: day(0), NextDue: day(7), Status: haccp.TaskDone}, *c.Schedule)
	assert.Equal(t, 7, c.EveryDays)

	f.clk.Advance(3 * 24 * time.Hour)
	v, err := f.svc.Today(ctx, f.cook)
	require.NoError(t, err)
	totalNotDue := v.Total
	assert.Equal(t, haccp.TaskNotDue, lines().Schedule.Status)

	f.clk.Advance(4 * 24 * time.Hour)
	assert.Equal(t, haccp.TaskDue, lines().Schedule.Status, "seven days after it was done")
	v, err = f.svc.Today(ctx, f.cook)
	require.NoError(t, err)
	assert.Equal(t, totalNotDue+1, v.Total, "counts as one of today's checks only when due")

	f.clk.Advance(3 * 24 * time.Hour) // day 10
	assert.Equal(t, haccp.TaskOverdue, lines().Schedule.Status)

	cu, err := f.svc.CatchUpGaps(ctx, f.manager, day(0), day(10))
	require.NoError(t, err)
	var lineGaps []CatchUpGap
	daily := 0
	for _, g := range cu.Gaps {
		switch g.Task {
		case "Clean beer lines":
			lineGaps = append(lineGaps, g)
		case "Wipe surfaces":
			daily++
		}
	}
	require.Len(t, lineGaps, 1, "one gap, on the day it fell due")
	assert.Equal(t, day(7), lineGaps[0].Date)
	assert.Equal(t, 7, lineGaps[0].EveryDays)
	assert.Equal(t, 10, daily, "daily tasks still have a gap per day")

	rep, err := f.svc.ReportHACCP(ctx, f.manager, day(0), day(10))
	require.NoError(t, err)
	require.Len(t, rep.CleaningOverdueByTask, 1)
	assert.Equal(t, 2, rep.CleaningOverdueByTask[0].Count, "overdue on days 8 and 9; today is not over")
	for _, m := range rep.CleaningMissedByTask {
		assert.NotEqual(t, "Clean beer lines", m.Key, "not counted as a daily task")
	}
}
