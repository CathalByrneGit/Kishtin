package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/books"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// CatchUpTypes are the record types the catch-up screen fills in: the
// daily checks Kishtin knows are due, and the other temperature readings.
var CatchUpTypes = append(slices.Clone(PaperTypes), haccp.RecordCleaning)

// maxCatchUpDays is the longest period the catch-up screen looks at.
const maxCatchUpDays = 62

// CatchUpGap is a daily check that was due and has no record: a fridge or
// freezer check, or a cleaning task.
type CatchUpGap struct {
	Date  string           `json:"date"`
	Type  haccp.RecordType `json:"type"`
	Unit  string           `json:"unit,omitempty"`
	Check string           `json:"check,omitempty"`
	Task  string           `json:"task,omitempty"`
	Area  string           `json:"area,omitempty"`
	// EveryDays is how often a cleaning task is due, when not every day;
	// the gap is then the day it fell due.
	EveryDays int `json:"every_days,omitempty"`
	// Time is the start of the check's window (HH:MM), as a starting point
	// for when it was done. Empty for cleaning tasks, which have none.
	Time string `json:"time,omitempty"`
	// SignedOff is set when the day is in a period already signed off.
	SignedOff bool `json:"signed_off,omitempty"`
}

// CatchUp lists the daily checks with no record in a period.
type CatchUp struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Since is the first day records were kept. Days before it are not
	// listed.
	Since string       `json:"since,omitempty"`
	Gaps  []CatchUpGap `json:"gaps"`
}

// CatchUpInput is rows of checks done earlier, each with the time it was
// done.
type CatchUpInput struct {
	Rows []NewRecord `json:"rows"`
}

// CatchUpGaps lists the fridge and freezer checks and cleaning tasks that
// were due in a period and have no record. Empty dates mean the last 14
// days up to today.
func (s *Service) CatchUpGaps(ctx context.Context, p *auth.Principal, fromDay, toDay string) (CatchUp, error) {
	if err := s.allow(p, auth.ActLogHACCP); err != nil {
		return CatchUp{}, err
	}
	if err := s.allow(p, auth.ActBackdate); err != nil {
		return CatchUp{}, err
	}
	now := s.clk.Now().In(clock.Dublin)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, clock.Dublin)
	parse := func(v, name string, def time.Time) (time.Time, error) {
		if strings.TrimSpace(v) == "" {
			return def, nil
		}
		t, err := time.ParseInLocation(haccp.DateLayout, strings.TrimSpace(v), clock.Dublin)
		if err != nil {
			return t, newErr(KindInvalid, "%s must be a date like 2026-10-01", name)
		}
		return t, nil
	}
	to, err := parse(toDay, "to", today)
	if err != nil {
		return CatchUp{}, err
	}
	from, err := parse(fromDay, "from", to.AddDate(0, 0, -13))
	if err != nil {
		return CatchUp{}, err
	}
	switch {
	case to.After(today):
		return CatchUp{}, newErr(KindInvalid, "the period cannot end after today")
	case from.After(to):
		return CatchUp{}, newErr(KindInvalid, "the period ends before it starts")
	case to.Sub(from) >= maxCatchUpDays*24*time.Hour:
		return CatchUp{}, newErr(KindInvalid, "look at %d days at most", maxCatchUpDays)
	}
	out := CatchUp{From: from.Format(haccp.DateLayout), To: to.Format(haccp.DateLayout), Gaps: []CatchUpGap{}}

	first, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{Limit: 1})
	if err != nil || len(first) == 0 {
		return out, err
	}
	since := first[0].LoggedAt
	sinceDay := since.In(clock.Dublin)
	sinceDay = time.Date(sinceDay.Year(), sinceDay.Month(), sinceDay.Day(), 0, 0, 0, 0, clock.Dublin)
	out.Since = sinceDay.Format(haccp.DateLayout)
	if from.Before(sinceDay) {
		from = sinceDay
	}
	if from.After(to) {
		return out, nil
	}
	recs, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{From: from})
	if err != nil {
		return out, err
	}
	lists, err := s.Lists().current(ctx, s.st)
	if err != nil {
		return out, err
	}
	signed := map[string]bool{}
	for _, r := range recs {
		if r.Type == haccp.RecordSignOff && r.Corrects == "" {
			for d, _ := time.ParseInLocation(haccp.DateLayout, r.Reading.PeriodFrom, clock.Dublin); d.Format(haccp.DateLayout) <= r.Reading.PeriodTo; d = d.AddDate(0, 0, 1) {
				signed[d.Format(haccp.DateLayout)] = true
			}
		}
	}
	// Sign-offs logged before the period can still cover it.
	older, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{Types: []haccp.RecordType{haccp.RecordSignOff}, To: from})
	if err != nil {
		return out, err
	}
	for _, r := range older {
		if r.Corrects == "" {
			for d, _ := time.ParseInLocation(haccp.DateLayout, r.Reading.PeriodFrom, clock.Dublin); d.Format(haccp.DateLayout) <= r.Reading.PeriodTo; d = d.AddDate(0, 0, 1) {
				signed[d.Format(haccp.DateLayout)] = true
			}
		}
	}

	units := map[string]haccp.Unit{}
	for _, u := range lists.Value.Units {
		units[strings.ToLower(u.Name)] = u
	}
	res := books.ReviewPeriod(books.ReviewInput{
		From: from, To: to, Loc: clock.Dublin, Now: s.clk.Now(), Since: since, Records: recs, Units: lists.Value.Units,
	})
	for _, g := range res.Missed {
		u := units[strings.ToLower(g.Unit)]
		out.Gaps = append(out.Gaps, CatchUpGap{
			Date: g.Date, Type: u.Kind, Unit: u.Name, Check: g.Check, Time: checkStart(u, g.Check), SignedOff: signed[g.Date],
		})
	}

	// Daily cleaning tasks are due every day; today's are not missed until
	// the day is over. A task done every N days is missed from the day
	// after it fell due.
	done := map[string]bool{}
	for _, e := range haccp.ApplyCorrections(recs) {
		if e.Type == haccp.RecordCleaning && e.Reading.Done != nil && *e.Reading.Done {
			done[e.At.In(clock.Dublin).Format(haccp.DateLayout)+"\x00"+strings.ToLower(e.Reading.Task)] = true
		}
	}
	for d := from; d.Before(today) && !d.After(to); d = d.AddDate(0, 0, 1) {
		day := d.Format(haccp.DateLayout)
		for _, t := range lists.Value.CleaningTasks {
			if t.Daily() && !done[day+"\x00"+strings.ToLower(t.Task)] {
				out.Gaps = append(out.Gaps, CatchUpGap{Date: day, Type: haccp.RecordCleaning, Task: t.Task, Area: t.Area, SignedOff: signed[day]})
			}
		}
	}
	interval, err := s.cleaningDays(ctx, lists.Value, today.AddDate(0, 0, -2*haccp.MaxEveryDays), today.AddDate(0, 0, 1))
	if err != nil {
		return out, err
	}
	todayDay := today.Format(haccp.DateLayout)
	for _, t := range lists.Value.CleaningTasks {
		if t.Daily() {
			continue
		}
		sched := haccp.TaskScheduleFor(t, interval[strings.ToLower(t.Task)], todayDay)
		due := sched.NextDue
		if sched.Status != haccp.TaskOverdue || due < from.Format(haccp.DateLayout) || due > to.Format(haccp.DateLayout) {
			continue
		}
		out.Gaps = append(out.Gaps, CatchUpGap{Date: due, Type: haccp.RecordCleaning, Task: t.Task, Area: t.Area, EveryDays: t.EveryDays, SignedOff: signed[due]})
	}
	slices.SortStableFunc(out.Gaps, func(a, b CatchUpGap) int { return strings.Compare(a.Date, b.Date) })
	return out, nil
}

// checkStart is when a unit's check window starts, as a starting point
// for when a missed check was done. A window starting at midnight starts
// at 09:00, the start of a working day.
func checkStart(u haccp.Unit, label string) string {
	for _, c := range u.ChecksOrDefault() {
		if c.Label == label {
			if c.From == "00:00" {
				return "09:00"
			}
			return c.From
		}
	}
	return ""
}

func (s *Service) checkCatchUpRow(ctx context.Context, st store.Store, p *auth.Principal, nr *NewRecord) (Check, error) {
	if !slices.Contains(CatchUpTypes, nr.Type) {
		return Check{}, newErr(KindInvalid, "a %s record cannot be caught up here: log it as usual with the time it was done", nr.Type)
	}
	if err := notFromPaper(*nr); err != nil {
		return Check{}, err
	}
	return s.checkEarlierRow(ctx, st, p, nr)
}

// PreviewCatchUp checks rows of checks done earlier, without saving.
func (s *Service) PreviewCatchUp(ctx context.Context, p *auth.Principal, in CatchUpInput) ([]PaperCheck, error) {
	if err := s.allowCatchUp(p, in); err != nil {
		return nil, err
	}
	return s.previewRows(ctx, in.Rows, "This reading needs the corrective action that was taken.",
		func(nr *NewRecord) (Check, error) { return s.checkCatchUpRow(ctx, s.st, p, nr) }), nil
}

// SaveCatchUp saves rows of checks done earlier, all or none. Each record
// keeps the time the check was done and, if given, who did it; it is
// logged by the person entering it. Results are checked against the limits
// in force now.
func (s *Service) SaveCatchUp(ctx context.Context, p *auth.Principal, in CatchUpInput) ([]haccp.Record, error) {
	if err := s.allowCatchUp(p, in); err != nil {
		return nil, err
	}
	ctx = withSource(ctx, SourceCatchUp)
	return s.saveRows(ctx, p, in.Rows, func(tx store.Store, nr *NewRecord) error {
		_, err := s.checkCatchUpRow(ctx, tx, p, nr)
		return err
	})
}

func (s *Service) allowCatchUp(p *auth.Principal, in CatchUpInput) error {
	if err := s.allow(p, auth.ActLogHACCP); err != nil {
		return err
	}
	if err := s.allow(p, auth.ActBackdate); err != nil {
		return err
	}
	if len(in.Rows) == 0 || len(in.Rows) > maxPaperRows {
		return newErr(KindInvalid, "send 1 to %d rows", maxPaperRows)
	}
	return nil
}
