package haccp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskScheduleFor(t *testing.T) {
	lines := CleaningTask{Task: "Clean beer lines", EveryDays: 7}
	done := []string{"2026-10-01", "2026-10-08"}
	for _, tc := range []struct {
		day  string
		want TaskSchedule
	}{
		{"2026-09-30", TaskSchedule{Status: TaskDue}},
		{"2026-10-08", TaskSchedule{LastDone: "2026-10-08", NextDue: "2026-10-15", Status: TaskDone}},
		{"2026-10-14", TaskSchedule{LastDone: "2026-10-08", NextDue: "2026-10-15", Status: TaskNotDue}},
		{"2026-10-15", TaskSchedule{LastDone: "2026-10-08", NextDue: "2026-10-15", Status: TaskDue}},
		{"2026-10-16", TaskSchedule{LastDone: "2026-10-08", NextDue: "2026-10-15", Status: TaskOverdue}},
	} {
		assert.Equal(t, tc.want, TaskScheduleFor(lines, done, tc.day), tc.day)
	}
	assert.Equal(t, TaskSchedule{Status: TaskDue}, TaskScheduleFor(lines, nil, "2026-10-08"), "never done is due, not overdue")
	daily := CleaningTask{Task: "Wipe surfaces"}
	assert.Equal(t, TaskOverdue, TaskScheduleFor(daily, []string{"2026-10-06"}, "2026-10-08").Status)
	assert.True(t, daily.Daily())
	assert.False(t, lines.Daily())

	// Done 1 and 8 October, then 20 October: overdue 16 to 19 October.
	assert.Equal(t, 4, OverdueDays(lines, []string{"2026-10-01", "2026-10-08", "2026-10-20"}, "2026-09-25", "2026-10-25"))
	assert.Equal(t, 0, OverdueDays(lines, nil, "2026-10-01", "2026-10-31"), "never done never counts")
}

func TestListsEveryDays(t *testing.T) {
	l := Lists{CleaningTasks: []CleaningTask{{Task: " Lines ", EveryDays: 1}, {Task: "Deep clean", EveryDays: 28}}}.Normalize()
	assert.Equal(t, CleaningTask{Task: "Lines"}, l.CleaningTasks[0], "every 1 day is daily")
	assert.Equal(t, 28, l.CleaningTasks[1].EveryDays)
	assert.NoError(t, l.Validate())
	l.CleaningTasks[1].EveryDays = 400
	assert.ErrorContains(t, l.Validate(), "between 1 and 366 days")
}
