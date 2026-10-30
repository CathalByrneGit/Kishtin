package analytics

import (
	"math"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/books"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

// HACCPInput is what the HACCP report works from.
type HACCPInput struct {
	// From and To are the first and last day of the period.
	From, To time.Time
	Loc      *time.Location
	Now      time.Time
	// Since is when record keeping started; days before it are not counted.
	Since time.Time
	// Records are the records logged from From on, with later corrections,
	// in any order.
	Records       []haccp.Record
	Units         []haccp.Unit
	CleaningTasks []haccp.CleaningTask
	// Limits is the limit history, for each unit's limit now.
	Limits []haccp.LimitVersion
	// CleaningDoneBefore gives, per cleaning task done every N days (lower
	// case), the days it was done before From, so the period's first due
	// day is known.
	CleaningDoneBefore map[string][]string
}

// HACCPReport is how well the daily checks were kept up in a period.
type HACCPReport struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Checks counts fridge and freezer checks: due, done and missed.
	ChecksDue    int `json:"checks_due"`
	ChecksDone   int `json:"checks_done"`
	ChecksMissed int `json:"checks_missed"`
	// ChecksPercent is the share of due checks done.
	ChecksPercent int `json:"checks_percent"`
	// MissedByWeekday counts missed checks on each day of the week, Monday
	// first, out of the checks due that day.
	MissedByWeekday []Count `json:"missed_by_weekday"`
	// MissedByUnit counts missed checks per fridge and freezer, most first.
	MissedByUnit []Count `json:"missed_by_unit"`
	// Cleaning counts daily cleaning tasks on days that are over.
	CleaningDue     int `json:"cleaning_due"`
	CleaningDone    int `json:"cleaning_done"`
	CleaningPercent int `json:"cleaning_percent"`
	// CleaningMissedByTask counts daily cleaning tasks not done, most first.
	CleaningMissedByTask []Count `json:"cleaning_missed_by_task"`
	// CleaningOverdueByTask counts, for tasks done every N days (weekly and
	// longer), the days in the period they were overdue, most first.
	CleaningOverdueByTask []Count `json:"cleaning_overdue_by_task"`
	// Readings counts every record in the period, and the failed and
	// borderline ones.
	Records       int `json:"records"`
	OutOfLimit    int `json:"out_of_limit"`
	WithoutAction int `json:"without_action"`
	// OutOfLimitByType counts failed and borderline records per type.
	OutOfLimitByType []Count `json:"out_of_limit_by_type"`
	// Temperatures is each fridge and freezer's readings per day.
	Temperatures []UnitTemps `json:"temperatures"`
	// Recent are the latest failed and borderline records, newest first.
	Recent []haccp.Record `json:"recent"`
}

// UnitTemps is one fridge or freezer's readings per day, with its limit.
type UnitTemps struct {
	Unit string           `json:"unit"`
	Kind haccp.RecordType `json:"kind"`
	// MaxC is the unit's limit now, if it has one.
	MaxC *float64  `json:"max_c,omitempty"`
	Days []DayTemp `json:"days"`
}

// DayTemp is one day's readings on a unit. Days with no reading have
// Count 0 and no temperatures.
type DayTemp struct {
	Date  string   `json:"date"`
	Count int      `json:"count"`
	MinC  *float64 `json:"min_c,omitempty"`
	MaxC  *float64 `json:"max_c,omitempty"`
	AvgC  *float64 `json:"avg_c,omitempty"`
}

// maxRecent caps the out-of-limit records listed.
const maxRecent = 50

func round1(v float64) *float64 {
	r := math.Round(v*10) / 10
	return &r
}

// slotIndex finds which of a unit's checks a time of day falls in.
func slotIndex(checks []haccp.CheckTime, minutes int) int {
	idx := 0
	for i, c := range checks {
		if from, err := haccp.ParseClock(c.From); err == nil && from <= minutes {
			idx = i
		}
	}
	return idx
}

// BuildHACCP works out the HACCP report. Missed checks are counted the way
// the record books count them.
func BuildHACCP(in HACCPInput) HACCPReport {
	loc := in.Loc
	if loc == nil {
		loc = time.UTC
	}
	start, end := day(in.From, loc), day(in.To, loc).AddDate(0, 0, 1)
	today := day(in.Now, loc)
	if !in.Since.IsZero() && day(in.Since, loc).After(start) {
		start = day(in.Since, loc)
	}
	r := HACCPReport{
		From: day(in.From, loc).Format(haccp.DateLayout), To: day(in.To, loc).Format(haccp.DateLayout),
		MissedByWeekday: []Count{}, MissedByUnit: []Count{}, CleaningMissedByTask: []Count{}, CleaningOverdueByTask: []Count{},
		OutOfLimitByType: []Count{}, Temperatures: []UnitTemps{}, Recent: []haccp.Record{},
	}
	if !start.Before(end) {
		return r
	}

	effective := []haccp.Effective{}
	for _, e := range haccp.ApplyCorrections(in.Records) {
		if e.Type != haccp.RecordSignOff && !e.At.Before(start) && e.At.Before(end) {
			effective = append(effective, e)
		}
	}

	// Missed checks, as the record books count them.
	review := books.ReviewPeriod(books.ReviewInput{From: start, To: end.AddDate(0, 0, -1), Loc: loc, Now: in.Now, Since: in.Since, Records: in.Records, Units: in.Units})
	missedDay := map[time.Weekday]int{}
	missedUnit := map[string]int{}
	for _, g := range review.Missed {
		d, err := time.ParseInLocation(haccp.DateLayout, g.Date, loc)
		if err != nil {
			continue
		}
		missedDay[d.Weekday()]++
		missedUnit[strings.ToLower(g.Unit)]++
	}
	r.ChecksMissed = len(review.Missed)

	// Checks done: a slot with at least one reading.
	done := map[string]bool{}
	doneDay := map[time.Weekday]int{}
	byUnit := map[string]haccp.Unit{}
	for _, u := range in.Units {
		byUnit[strings.ToLower(u.Name)] = u
	}
	for _, e := range effective {
		if e.Type != haccp.RecordFridge && e.Type != haccp.RecordFreezer {
			continue
		}
		u, ok := byUnit[strings.ToLower(e.Reading.Unit)]
		if !ok || u.Kind != e.Type {
			continue
		}
		at := e.At.In(loc)
		slot := slotIndex(u.ChecksOrDefault(), at.Hour()*60+at.Minute())
		key := at.Format(haccp.DateLayout) + "\x00" + strings.ToLower(u.Name) + "\x00" + string(rune('0'+slot))
		if !done[key] {
			done[key] = true
			doneDay[at.Weekday()]++
		}
	}
	r.ChecksDone = len(done)
	r.ChecksDue = r.ChecksDone + r.ChecksMissed
	r.ChecksPercent = percent(r.ChecksDone, r.ChecksDue)
	for _, wd := range Weekdays {
		r.MissedByWeekday = append(r.MissedByWeekday, Count{Key: strings.ToLower(wd.String()[:3]), Label: wd.String(), Count: missedDay[wd], Of: missedDay[wd] + doneDay[wd]})
	}
	for _, u := range in.Units {
		if n := missedUnit[strings.ToLower(u.Name)]; n > 0 {
			r.MissedByUnit = append(r.MissedByUnit, Count{Key: u.Name, Label: u.Name, Count: n})
		}
	}
	sortCounts(r.MissedByUnit)

	// Cleaning tasks, on days that are over.
	cleaned := map[string]bool{}
	for _, e := range effective {
		if e.Type == haccp.RecordCleaning && e.Reading.Done != nil && *e.Reading.Done {
			cleaned[e.At.In(loc).Format(haccp.DateLayout)+"\x00"+strings.ToLower(e.Reading.Task)] = true
		}
	}
	missedTask := map[string]int{}
	for d := start; d.Before(end) && d.Before(today); d = d.AddDate(0, 0, 1) {
		for _, t := range in.CleaningTasks {
			if !t.Daily() {
				continue
			}
			r.CleaningDue++
			if cleaned[d.Format(haccp.DateLayout)+"\x00"+strings.ToLower(t.Task)] {
				r.CleaningDone++
			} else {
				missedTask[t.Task]++
			}
		}
	}
	r.CleaningPercent = percent(r.CleaningDone, r.CleaningDue)
	for _, t := range in.CleaningTasks {
		if n := missedTask[t.Task]; n > 0 {
			r.CleaningMissedByTask = append(r.CleaningMissedByTask, Count{Key: t.Task, Label: t.Task, Count: n})
		}
	}
	sortCounts(r.CleaningMissedByTask)
	lastDay := end.AddDate(0, 0, -1)
	if !lastDay.Before(today) {
		lastDay = today.AddDate(0, 0, -1)
	}
	for _, t := range in.CleaningTasks {
		if t.Daily() || lastDay.Before(start) {
			continue
		}
		done := append([]string{}, in.CleaningDoneBefore[strings.ToLower(t.Task)]...)
		for k := range cleaned {
			day, task, _ := strings.Cut(k, "\x00")
			if task == strings.ToLower(t.Task) {
				done = append(done, day)
			}
		}
		if n := haccp.OverdueDays(t, done, start.Format(haccp.DateLayout), lastDay.Format(haccp.DateLayout)); n > 0 {
			r.CleaningOverdueByTask = append(r.CleaningOverdueByTask, Count{Key: t.Task, Label: t.Task, Count: n})
		}
	}
	sortCounts(r.CleaningOverdueByTask)

	// Out-of-limit readings.
	byType := map[haccp.RecordType]int{}
	for i := len(effective) - 1; i >= 0; i-- {
		e := effective[i]
		r.Records++
		if e.Outcome != haccp.OutcomeFail && e.Outcome != haccp.OutcomeBorderline {
			continue
		}
		r.OutOfLimit++
		byType[e.Type]++
		if strings.TrimSpace(e.CorrectiveAction) == "" {
			r.WithoutAction++
		}
		if len(r.Recent) < maxRecent {
			r.Recent = append(r.Recent, e.Record)
		}
	}
	for _, t := range haccp.RecordTypes {
		if n := byType[t]; n > 0 {
			r.OutOfLimitByType = append(r.OutOfLimitByType, Count{Key: string(t), Label: string(t), Count: n})
		}
	}
	sortCounts(r.OutOfLimitByType)

	// Temperatures per unit per day.
	type acc struct {
		n             int
		lo, hi, total float64
	}
	temps := map[string]map[string]*acc{}
	for _, e := range effective {
		if (e.Type != haccp.RecordFridge && e.Type != haccp.RecordFreezer) || e.Reading.TempC == nil {
			continue
		}
		u := strings.ToLower(e.Reading.Unit)
		if temps[u] == nil {
			temps[u] = map[string]*acc{}
		}
		d := e.At.In(loc).Format(haccp.DateLayout)
		a := temps[u][d]
		v := *e.Reading.TempC
		if a == nil {
			a = &acc{lo: v, hi: v}
			temps[u][d] = a
		}
		a.n++
		a.total += v
		a.lo, a.hi = math.Min(a.lo, v), math.Max(a.hi, v)
	}
	for _, u := range in.Units {
		check := haccp.CheckFridge
		if u.Kind == haccp.RecordFreezer {
			check = haccp.CheckFreezer
		}
		ut := UnitTemps{Unit: u.Name, Kind: u.Kind, Days: []DayTemp{}}
		if len(in.Limits) > 0 {
			ut.MaxC = haccp.Resolve(in.Limits, check, haccp.Scopes(u.Kind, haccp.Reading{Unit: u.Name})).MaxC
		}
		for d := start; d.Before(end) && !d.After(today); d = d.AddDate(0, 0, 1) {
			ds := d.Format(haccp.DateLayout)
			dt := DayTemp{Date: ds}
			if a := temps[strings.ToLower(u.Name)][ds]; a != nil {
				dt.Count, dt.MinC, dt.MaxC, dt.AvgC = a.n, round1(a.lo), round1(a.hi), round1(a.total/float64(a.n))
			}
			ut.Days = append(ut.Days, dt)
		}
		r.Temperatures = append(r.Temperatures, ut)
	}
	return r
}
