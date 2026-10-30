package haccp

import (
	"time"
)

// Task statuses for a cleaning task done every N days.
const (
	TaskDone    = "done"    // done today
	TaskDue     = "due"     // due today, or never done yet
	TaskOverdue = "overdue" // past the day it fell due
	TaskNotDue  = "not_due" // done recently enough
)

// TaskSchedule is where a cleaning task done every N days stands on a day.
type TaskSchedule struct {
	// LastDone is the last day it was done, if ever (YYYY-MM-DD).
	LastDone string `json:"last_done,omitempty"`
	// NextDue is the day it falls due: N days after LastDone. Empty when it
	// has never been done: then it is due now.
	NextDue string `json:"next_due,omitempty"`
	Status  string `json:"status"`
}

// TaskScheduleFor works out a task's schedule on day from the days it was
// done (YYYY-MM-DD, any order, up to and including day). A task never done
// is due, not overdue: Kishtin cannot know when it first fell due.
func TaskScheduleFor(t CleaningTask, done []string, day string) TaskSchedule {
	every := max(t.EveryDays, 1)
	var last string
	for _, d := range done {
		if d <= day && d > last {
			last = d
		}
	}
	if last == "" {
		return TaskSchedule{Status: TaskDue}
	}
	s := TaskSchedule{LastDone: last, NextDue: addDays(last, every)}
	switch {
	case last == day:
		s.Status = TaskDone
	case s.NextDue < day:
		s.Status = TaskOverdue
	case s.NextDue == day:
		s.Status = TaskDue
	default:
		s.Status = TaskNotDue
	}
	return s
}

// OverdueDays counts the days from from to to (inclusive, YYYY-MM-DD) on
// which a task done every N days was overdue, given the days it was done.
// Days before it was first done never count.
func OverdueDays(t CleaningTask, done []string, from, to string) int {
	n := 0
	for d := from; d <= to; d = addDays(d, 1) {
		if TaskScheduleFor(t, done, d).Status == TaskOverdue {
			n++
		}
	}
	return n
}

func addDays(day string, n int) string {
	d, err := time.Parse(DateLayout, day)
	if err != nil {
		return day
	}
	return d.AddDate(0, 0, n).Format(DateLayout)
}
