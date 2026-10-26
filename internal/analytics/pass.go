package analytics

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/scheduler"
)

// OnTimeGrace is how far past its target a course can reach the pass and
// still count as on time.
const OnTimeGrace = time.Minute

// CourseSeen is one course's status in one kept plan snapshot.
type CourseSeen struct {
	At time.Time
	scheduler.CourseStatus
}

// PassInput is what the pass report works from.
type PassInput struct {
	// From and To are the first and last day of the period. A course
	// counts in the day it was fired.
	From, To time.Time
	Loc      *time.Location
	// Orders are the orders that may have courses fired in the period.
	Orders []orders.Order
	// Seen are course statuses from kept plans, in time order, for why
	// courses ran late.
	Seen []CourseSeen
}

// PassReport is how fast courses reached the pass in a period.
type PassReport struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Courses counts fired courses whose items all reached the pass (or
	// were voided). Courses still cooking are left out.
	Courses int `json:"courses"`
	OnTime  int `json:"on_time"`
	// OnTimePercent is the share of courses at the pass within a minute of
	// their target.
	OnTimePercent int `json:"on_time_percent"`
	Late          int `json:"late"`
	// Ticket is the time from firing to the last item reaching the pass.
	Ticket Spread `json:"ticket"`
	// PassWait is the time from the last item reaching the pass to the
	// course being served.
	PassWait Spread `json:"pass_wait"`
	// LateByCause counts late courses by the last cause the scheduler gave
	// before the course reached the pass; "unknown" when no kept plan gave
	// one.
	LateByCause []Count `json:"late_by_cause"`
	// ByCourse is the same for starters, mains and desserts.
	ByCourse []CourseStats `json:"by_course"`
	// ByHour is courses fired in each hour of the day, with how many were
	// on time.
	ByHour []HourStats `json:"by_hour"`
	// Dishes compares each dish's actual time, from start to ready, with
	// its set cook, rest and plate time, slowest first.
	Dishes []DishTimes `json:"dishes"`
	// LateCourses are the late courses, latest first.
	LateCourses []LateCourse `json:"late_courses"`
}

// CourseStats is the pass report for one kind of course.
type CourseStats struct {
	Course        menu.Course `json:"course"`
	Courses       int         `json:"courses"`
	OnTimePercent int         `json:"on_time_percent"`
	Ticket        Spread      `json:"ticket"`
	PassWait      Spread      `json:"pass_wait"`
}

// HourStats is courses fired in one hour of the day (local time).
type HourStats struct {
	Hour    int `json:"hour"`
	Courses int `json:"courses"`
	OnTime  int `json:"on_time"`
}

// DishTimes is one dish's actual times against its set time.
type DishTimes struct {
	DishID   string `json:"dish_id"`
	DishName string `json:"dish_name"`
	// SetSeconds is the dish's cook, rest and plate time on the items.
	SetSeconds int    `json:"set_seconds"`
	Actual     Spread `json:"actual"`
	// OverSeconds is how much longer than set the median took; negative
	// when quicker.
	OverSeconds int `json:"over_seconds"`
}

// LateCourse is one course that reached the pass late.
type LateCourse struct {
	OrderID string      `json:"order_id"`
	Label   string      `json:"label"`
	Course  menu.Course `json:"course"`
	FiredAt time.Time   `json:"fired_at"`
	Target  time.Time   `json:"target"`
	ReadyAt time.Time   `json:"ready_at"`
	LateBy  int         `json:"late_by_seconds"`
	Cause   string      `json:"cause"`
	Reason  string      `json:"reason,omitempty"`
}

// CauseUnknown is a late course no kept plan gave a cause for.
const CauseUnknown = "unknown"

// maxLateCourses caps the late courses listed.
const maxLateCourses = 100

type courseKey struct {
	order  string
	course menu.Course
}

// BuildPass works out the pass report.
func BuildPass(in PassInput) PassReport {
	loc := in.Loc
	if loc == nil {
		loc = time.UTC
	}
	start, end := day(in.From, loc), day(in.To, loc).AddDate(0, 0, 1)
	r := PassReport{
		From: day(in.From, loc).Format(haccp.DateLayout), To: day(in.To, loc).Format(haccp.DateLayout),
		LateByCause: []Count{}, ByCourse: []CourseStats{}, ByHour: []HourStats{}, Dishes: []DishTimes{}, LateCourses: []LateCourse{},
	}

	// The last cause and reason each course had while at risk or late.
	type why struct {
		at            time.Time
		cause, reason string
	}
	causes := map[courseKey][]why{}
	for _, s := range in.Seen {
		if s.Cause != "" && (s.Status == scheduler.StatusLate || s.Status == scheduler.StatusAtRisk) {
			k := courseKey{s.OrderID, s.Course}
			causes[k] = append(causes[k], why{s.At, s.Cause, s.Reason})
		}
	}

	var tickets, waits []time.Duration
	perCourse := map[menu.Course]*struct {
		n, onTime      int
		tickets, waits []time.Duration
	}{}
	hours := map[int]*HourStats{}
	causeCount := map[string]int{}
	dishActual := map[string][]time.Duration{}
	dishSet := map[string]time.Duration{}
	dishName := map[string]string{}

	for _, o := range in.Orders {
		for _, c := range o.Courses {
			if c.FiredAt == nil || c.TargetAt == nil || c.FiredAt.Before(start) || !c.FiredAt.Before(end) {
				continue
			}
			var ready time.Time
			finished, any := true, false
			for _, it := range o.ItemsIn(c.Course) {
				if it.State == orders.StateVoided {
					continue
				}
				any = true
				if it.ReadyAt == nil {
					finished = false
					break
				}
				if it.ReadyAt.After(ready) {
					ready = *it.ReadyAt
				}
				if it.StartedAt != nil && it.ReadyAt.After(*it.StartedAt) {
					dishActual[it.DishID] = append(dishActual[it.DishID], it.ReadyAt.Sub(*it.StartedAt))
					dishSet[it.DishID] = it.Timing.Cook + it.Timing.Rest + it.Timing.Plate
					dishName[it.DishID] = it.DishName
				}
			}
			if !any || !finished {
				continue
			}
			r.Courses++
			ticket := ready.Sub(*c.FiredAt)
			tickets = append(tickets, ticket)
			pc := perCourse[c.Course]
			if pc == nil {
				pc = &struct {
					n, onTime      int
					tickets, waits []time.Duration
				}{}
				perCourse[c.Course] = pc
			}
			pc.n++
			pc.tickets = append(pc.tickets, ticket)
			if c.ServedAt != nil && !c.ServedAt.Before(ready) {
				waits = append(waits, c.ServedAt.Sub(ready))
				pc.waits = append(pc.waits, c.ServedAt.Sub(ready))
			}
			h := c.FiredAt.In(loc).Hour()
			if hours[h] == nil {
				hours[h] = &HourStats{Hour: h}
			}
			hours[h].Courses++
			if !ready.After(c.TargetAt.Add(OnTimeGrace)) {
				r.OnTime++
				pc.onTime++
				hours[h].OnTime++
				continue
			}
			r.Late++
			lc := LateCourse{
				OrderID: o.ID, Label: o.Label(), Course: c.Course, FiredAt: *c.FiredAt, Target: *c.TargetAt,
				ReadyAt: ready, LateBy: secs(ready.Sub(*c.TargetAt)), Cause: CauseUnknown,
			}
			for _, w := range causes[courseKey{o.ID, c.Course}] {
				if !w.at.After(ready) {
					lc.Cause, lc.Reason = w.cause, w.reason
				}
			}
			causeCount[lc.Cause]++
			r.LateCourses = append(r.LateCourses, lc)
		}
	}

	r.OnTimePercent = percent(r.OnTime, r.Courses)
	r.Ticket, r.PassWait = spread(tickets), spread(waits)
	for _, cause := range []string{scheduler.CauseEquipmentFull, scheduler.CauseStartedLate, scheduler.CauseCookingLong, scheduler.CauseWaitingAtPass, scheduler.CauseBusy, CauseUnknown} {
		if n := causeCount[cause]; n > 0 {
			r.LateByCause = append(r.LateByCause, Count{Key: cause, Label: CauseLabel(cause), Count: n})
		}
	}
	sortCounts(r.LateByCause)
	for _, course := range []menu.Course{menu.CourseStarter, menu.CourseMain, menu.CourseDessert} {
		if pc := perCourse[course]; pc != nil {
			r.ByCourse = append(r.ByCourse, CourseStats{Course: course, Courses: pc.n, OnTimePercent: percent(pc.onTime, pc.n), Ticket: spread(pc.tickets), PassWait: spread(pc.waits)})
		}
	}
	for h := 0; h < 24; h++ {
		if hs := hours[h]; hs != nil {
			r.ByHour = append(r.ByHour, *hs)
		}
	}
	for id, ds := range dishActual {
		sp := spread(ds)
		r.Dishes = append(r.Dishes, DishTimes{DishID: id, DishName: dishName[id], SetSeconds: secs(dishSet[id]), Actual: sp, OverSeconds: sp.Median - secs(dishSet[id])})
	}
	sort.Slice(r.Dishes, func(i, j int) bool {
		if r.Dishes[i].OverSeconds != r.Dishes[j].OverSeconds {
			return r.Dishes[i].OverSeconds > r.Dishes[j].OverSeconds
		}
		return r.Dishes[i].DishName < r.Dishes[j].DishName
	})
	sort.SliceStable(r.LateCourses, func(i, j int) bool { return r.LateCourses[i].ReadyAt.After(r.LateCourses[j].ReadyAt) })
	if len(r.LateCourses) > maxLateCourses {
		r.LateCourses = r.LateCourses[:maxLateCourses]
	}
	return r
}

// CauseLabel says a late-course cause in words.
func CauseLabel(cause string) string {
	switch cause {
	case scheduler.CauseEquipmentFull:
		return "Equipment full"
	case scheduler.CauseStartedLate:
		return "Started late"
	case scheduler.CauseCookingLong:
		return "Took longer than set"
	case scheduler.CauseWaitingAtPass:
		return "Waited too long at the pass"
	case scheduler.CauseBusy:
		return "Equipment busy"
	case CauseUnknown:
		return "No cause recorded"
	}
	return strings.ReplaceAll(fmt.Sprint(cause), "_", " ")
}
