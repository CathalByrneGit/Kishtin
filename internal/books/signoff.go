package books

import (
	"slices"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

// SignOff is a manager's sign-off, as a booklet's check line shows it.
type SignOff struct {
	Seq int64 `json:"seq"`
	// Date is the day it was signed, as YYYY-MM-DD.
	Date string `json:"date"`
	By   string `json:"by"`
	// From and To are the days covered, inclusive.
	From string `json:"from"`
	To   string `json:"to"`
	Note string `json:"note,omitempty"`
}

// Span is a run of days, inclusive, as YYYY-MM-DD.
type Span struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ReviewInput is what ReviewPeriod works from.
type ReviewInput struct {
	// From and To are the first and last day of the period.
	From, To time.Time
	Loc      *time.Location
	Now      time.Time
	// Since is when record keeping started.
	Since time.Time
	// Records are the records logged from From on, including later
	// corrections, in any order.
	Records []haccp.Record
	Units   []haccp.Unit
}

// ReviewResult is what a period holds, for a manager to check before
// signing it off.
type ReviewResult struct {
	Review        haccp.Review   `json:"review"`
	Missed        []Gap          `json:"missed"`
	OutOfLimit    []haccp.Record `json:"out_of_limit"`
	WithoutAction []haccp.Record `json:"without_action"`
}

// reviewGrid finds missed fridge and freezer checks for a review.
var reviewGrid = Template{
	ID: "review-grid", Code: "review", Title: "Missed checks", Builder: BuilderUnitGrid, UnitsPerPage: 12,
	Columns: []Column{{Label: "Day", Field: "date", Width: 1}, {Label: "Checks", Field: "unit_checks", Width: 10}},
}

// ReviewPeriod counts the records, out-of-limit readings and missed checks
// in a period. Sign-offs themselves are not counted.
func ReviewPeriod(in ReviewInput) ReviewResult {
	loc := in.Loc
	if loc == nil {
		loc = time.UTC
	}
	day := func(t time.Time) time.Time {
		t = t.In(loc)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	}
	start, end := day(in.From), day(in.To).AddDate(0, 0, 1)
	out := ReviewResult{Missed: []Gap{}, OutOfLimit: []haccp.Record{}, WithoutAction: []haccp.Record{}}

	recs := slices.Clone(in.Records)
	slices.SortFunc(recs, func(a, b haccp.Record) int { return int(a.Seq - b.Seq) })
	for _, e := range haccp.ApplyCorrections(recs) {
		if e.Type == haccp.RecordSignOff || e.At.Before(start) || !e.At.Before(end) {
			continue
		}
		out.Review.Records++
		if e.Outcome == haccp.OutcomeFail || e.Outcome == haccp.OutcomeBorderline {
			out.OutOfLimit = append(out.OutOfLimit, e.Record)
			if e.CorrectiveAction == "" {
				out.WithoutAction = append(out.WithoutAction, e.Record)
			}
		}
	}
	first, last := start.Format(haccp.DateLayout), end.AddDate(0, 0, -1).Format(haccp.DateLayout)
	for m := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, loc); m.Before(end); m = m.AddDate(0, 1, 0) {
		b, err := Build(Input{Template: reviewGrid, Year: m.Year(), Month: m.Month(), Loc: loc, Now: in.Now, Since: in.Since, Records: in.Records, Units: in.Units})
		if err != nil {
			continue // the review grid is fixed and always valid
		}
		for _, g := range b.Gaps {
			if g.Date >= first && g.Date <= last {
				out.Missed = append(out.Missed, g)
			}
		}
	}
	out.Review.OutOfLimit = len(out.OutOfLimit)
	out.Review.WithoutAction = len(out.WithoutAction)
	out.Review.Missed = len(out.Missed)
	return out
}

// signOffs finds the sign-offs whose period overlaps the book's month, and
// the days up to yesterday that none covers.
func (b *builder) signOffs() {
	first, last := b.from.Format(haccp.DateLayout), b.to.AddDate(0, 0, -1).Format(haccp.DateLayout)
	covered := map[string]bool{}
	for _, r := range b.in.Records {
		if r.Type != haccp.RecordSignOff || r.Corrects != "" {
			continue
		}
		from, to := r.Reading.PeriodFrom, r.Reading.PeriodTo
		if to < first || from > last {
			continue
		}
		b.book.SignOffs = append(b.book.SignOffs, SignOff{
			Seq: r.Seq, Date: r.LoggedAt.In(b.in.Loc).Format(haccp.DateLayout), By: r.LoggedByName,
			From: from, To: to, Note: r.Reading.Notes,
		})
		for d, _ := time.ParseInLocation(haccp.DateLayout, max(from, first), b.in.Loc); !d.After(mustDate(min(to, last), b.in.Loc)); d = d.AddDate(0, 0, 1) {
			covered[d.Format(haccp.DateLayout)] = true
		}
	}
	slices.SortFunc(b.book.SignOffs, func(x, y SignOff) int { return int(x.Seq - y.Seq) })

	today := b.in.Now.In(b.in.Loc).Format(haccp.DateLayout)
	since := ""
	if !b.in.Since.IsZero() {
		since = b.in.Since.In(b.in.Loc).Format(haccp.DateLayout)
	}
	var open *Span
	for d := b.from; d.Before(b.to); d = d.AddDate(0, 0, 1) {
		ds := d.Format(haccp.DateLayout)
		if ds >= today || ds < since || b.in.Since.IsZero() {
			continue
		}
		if covered[ds] {
			open = nil
			continue
		}
		if open != nil {
			open.To = ds
			continue
		}
		b.book.Unsigned = append(b.book.Unsigned, Span{From: ds, To: ds})
		open = &b.book.Unsigned[len(b.book.Unsigned)-1]
	}
}

func mustDate(s string, loc *time.Location) time.Time {
	t, _ := time.ParseInLocation(haccp.DateLayout, s, loc)
	return t
}
