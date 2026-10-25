package haccp

import (
	"slices"
	"strings"
	"time"
)

// Slot statuses.
const (
	SlotDone   = "done"
	SlotDue    = "due"
	SlotMissed = "missed"
)

// Ring states for the day's compliance ring.
const (
	// RingAttention: at least one reading today was out of limit.
	RingAttention = "attention"
	// RingComplete: every check due today is done.
	RingComplete = "complete"
	// RingInProgress: checks are still due.
	RingInProgress = "in_progress"
)

// Batch label statuses.
const (
	BatchExpired  = "expired"
	BatchUseToday = "use_today"
	BatchOK       = "ok"
)

// SlotToday is one temperature check on a unit today.
type SlotToday struct {
	Label string `json:"label"`
	From  string `json:"from"`
	// Until is when the next check starts, or empty for the last one.
	Until  string  `json:"until,omitempty"`
	Status string  `json:"status"`
	Record *Record `json:"record,omitempty"`
}

// UnitToday is a fridge or freezer with its checks today.
type UnitToday struct {
	Name    string       `json:"name"`
	Kind    RecordType   `json:"kind"`
	NonFood bool         `json:"non_food,omitempty"`
	Limit   LimitVersion `json:"limit"`
	Slots   []SlotToday  `json:"slots"`
}

// CleaningToday is a cleaning task and whether it is done today.
type CleaningToday struct {
	Task   string  `json:"task"`
	Area   string  `json:"area,omitempty"`
	Record *Record `json:"record,omitempty"`
}

// BatchToday is a batch label still in use.
type BatchToday struct {
	Record Record `json:"record"`
	UseBy  string `json:"use_by"`
	Status string `json:"status"`
}

// Today is the day's HACCP picture: what is done, due, missed and out of
// limit.
type Today struct {
	// Date is today's local date.
	Date     string          `json:"date"`
	Units    []UnitToday     `json:"units"`
	Cleaning []CleaningToday `json:"cleaning"`
	Done     int             `json:"done"`
	Total    int             `json:"total"`
	Percent  int             `json:"percent"`
	// OutOfLimit counts today's failed and borderline records.
	OutOfLimit int `json:"out_of_limit"`
	// WithoutAction counts the out-of-limit records with no corrective
	// action, which the rules can allow for borderline readings.
	WithoutAction  int          `json:"without_action"`
	Ring           string       `json:"ring"`
	NeedsAttention []Record     `json:"needs_attention"`
	Batches        []BatchToday `json:"batches"`
}

// TodayInput is what BuildToday works from.
type TodayInput struct {
	Now time.Time
	Loc *time.Location
	// Records are the records logged today, in sequence order.
	Records []Record
	// BatchLabels are recent batch label records, in any order.
	BatchLabels []Record
	Lists       Lists
	Limits      []LimitVersion
}

// Effective is a record after corrections, placed at the time of the
// record it corrects.
type Effective struct {
	Record
	// At is when the original record's check was done: when it was logged,
	// or, for a record transcribed from paper, when it was taken.
	At time.Time
}

// TakenAt is when a record's check was done: when it was logged, or when
// it was taken for a record transcribed from paper.
func (r Record) TakenAt() time.Time {
	if r.Reading.TakenAt != nil {
		return *r.Reading.TakenAt
	}
	return r.LoggedAt
}

// ApplyCorrections replaces each corrected record with its latest
// correction. A correction keeps the time of the original it corrects, so
// it fills the same check. Corrections of records not in the set (from
// another day) are left out.
func ApplyCorrections(recs []Record) []Effective {
	byID := map[string]int{}
	out := make([]Effective, 0, len(recs))
	sorted := slices.Clone(recs)
	slices.SortFunc(sorted, func(a, b Record) int { return int(a.Seq - b.Seq) })
	for _, r := range sorted {
		if r.Corrects == "" {
			byID[r.ID] = len(out)
			out = append(out, Effective{Record: r, At: r.TakenAt()})
			continue
		}
		i, ok := byID[r.Corrects]
		if !ok {
			continue
		}
		out[i] = Effective{Record: r, At: out[i].At}
		byID[r.ID] = i
	}
	return out
}

func minutes(t time.Time, loc *time.Location) int {
	l := t.In(loc)
	return l.Hour()*60 + l.Minute()
}

// slotIndex finds which check a time of day falls in.
func slotIndex(checks []CheckTime, m int) int {
	idx := 0
	for i, c := range checks {
		from, err := ParseClock(c.From)
		if err == nil && from <= m {
			idx = i
		}
	}
	return idx
}

// BuildToday works out the day's checks from today's records.
func BuildToday(in TodayInput) Today {
	loc := in.Loc
	if loc == nil {
		loc = time.UTC
	}
	now := in.Now.In(loc)
	nowMin := now.Hour()*60 + now.Minute()
	day := now.Format(DateLayout)
	t := Today{Date: day, Units: []UnitToday{}, Cleaning: []CleaningToday{}, NeedsAttention: []Record{}, Batches: []BatchToday{}}

	recs := ApplyCorrections(in.Records)

	for _, u := range in.Lists.Units {
		checks := u.ChecksOrDefault()
		check := CheckFridge
		if u.Kind == RecordFreezer {
			check = CheckFreezer
		}
		ut := UnitToday{
			Name: u.Name, Kind: u.Kind, NonFood: u.NonFood,
			Limit: Resolve(in.Limits, check, Scopes(u.Kind, Reading{Unit: u.Name})),
		}
		latest := make([]*Effective, len(checks))
		for k := range recs {
			r := &recs[k]
			if r.Type != u.Kind || !strings.EqualFold(r.Reading.Unit, u.Name) {
				continue
			}
			i := slotIndex(checks, minutes(r.At, loc))
			if latest[i] == nil || !r.At.Before(latest[i].At) {
				latest[i] = r
			}
		}
		for i, c := range checks {
			st := SlotToday{Label: c.Label, From: c.From, Status: SlotDue}
			if i+1 < len(checks) {
				st.Until = checks[i+1].From
				if until, err := ParseClock(st.Until); err == nil && nowMin >= until {
					st.Status = SlotMissed
				}
			}
			if latest[i] != nil {
				rec := latest[i].Record
				st.Status, st.Record = SlotDone, &rec
				t.Done++
			}
			t.Total++
			ut.Slots = append(ut.Slots, st)
		}
		t.Units = append(t.Units, ut)
	}

	for _, ct := range in.Lists.CleaningTasks {
		c := CleaningToday{Task: ct.Task, Area: ct.Area}
		for k := range recs {
			r := recs[k].Record
			if r.Type == RecordCleaning && strings.EqualFold(r.Reading.Task, ct.Task) && r.Reading.Done != nil && *r.Reading.Done {
				c.Record = &r
			}
		}
		if c.Record != nil {
			t.Done++
		}
		t.Total++
		t.Cleaning = append(t.Cleaning, c)
	}

	for k := len(recs) - 1; k >= 0; k-- {
		r := recs[k].Record
		if r.Outcome == OutcomeFail || r.Outcome == OutcomeBorderline {
			t.OutOfLimit++
			if strings.TrimSpace(r.CorrectiveAction) == "" {
				t.WithoutAction++
			}
			t.NeedsAttention = append(t.NeedsAttention, r)
		}
	}

	if t.Total > 0 {
		t.Percent = t.Done * 100 / t.Total
	}
	switch {
	case t.OutOfLimit > 0:
		t.Ring = RingAttention
	case t.Total > 0 && t.Done == t.Total:
		t.Ring = RingComplete
	default:
		t.Ring = RingInProgress
	}

	oldest := now.AddDate(0, 0, -3).Format(DateLayout)
	batches := ApplyCorrections(in.BatchLabels)
	slices.SortFunc(batches, func(a, b Effective) int { return int(b.Seq - a.Seq) })
	for _, b := range batches {
		if b.Type != RecordBatchLabel || b.Reading.UseBy < oldest {
			continue
		}
		bt := BatchToday{Record: b.Record, UseBy: b.Reading.UseBy, Status: BatchOK}
		switch {
		case b.Reading.UseBy < day:
			bt.Status = BatchExpired
		case b.Reading.UseBy == day:
			bt.Status = BatchUseToday
		}
		t.Batches = append(t.Batches, bt)
		if len(t.Batches) == 12 {
			break
		}
	}
	return t
}

// DoneToday returns today's effective cleaning record for a task, if the
// task is done.
func DoneToday(todays []Record, task string) *Record {
	for _, e := range ApplyCorrections(todays) {
		r := e.Record
		if r.Type == RecordCleaning && strings.EqualFold(r.Reading.Task, task) && r.Reading.Done != nil && *r.Reading.Done {
			return &r
		}
	}
	return nil
}
