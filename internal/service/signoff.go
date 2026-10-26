package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/books"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// SignOffInput is a period to sign off. Empty dates mean the default
// period: from the day after the last sign-off up to yesterday.
type SignOffInput struct {
	// From and To are the first and last day, as YYYY-MM-DD (Dublin).
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Note string `json:"note,omitempty"`
}

// SignOffPreview is what a sign-off would cover, for the manager to check
// before signing.
type SignOffPreview struct {
	From string `json:"from"`
	To   string `json:"to"`
	books.ReviewResult
	// NoteRequired is set when there are missed checks or readings without
	// a corrective action: the sign-off needs a note about them.
	NoteRequired bool `json:"note_required"`
	// Last is the most recent sign-off, if any.
	Last *haccp.Record `json:"last,omitempty"`
	// Overlaps are earlier sign-offs that already cover some of the period.
	Overlaps []haccp.Record `json:"overlaps"`
	// UpToDate is set when no dates were given and every day up to today
	// is already signed off. The rest of the preview is empty.
	UpToDate bool `json:"up_to_date"`
}

// errUpToDate means every day up to today is signed off.
var errUpToDate = newErr(KindConflict, "every day up to today is already signed off")

func (s *Service) lastSignOff(ctx context.Context, st store.Store) (*haccp.Record, error) {
	recs, err := st.HACCP().ListRecords(ctx, store.RecordFilter{Types: []haccp.RecordType{haccp.RecordSignOff}, Newest: true, Limit: 1})
	if err != nil || len(recs) == 0 {
		return nil, err
	}
	return &recs[0], nil
}

// signOffPeriod works out the period to sign off and checks it.
func (s *Service) signOffPeriod(ctx context.Context, st store.Store, in SignOffInput) (time.Time, time.Time, *haccp.Record, error) {
	now := s.clk.Now().In(clock.Dublin)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, clock.Dublin)
	last, err := s.lastSignOff(ctx, st)
	if err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	parse := func(v, name string) (time.Time, error) {
		t, err := time.ParseInLocation(haccp.DateLayout, strings.TrimSpace(v), clock.Dublin)
		if err != nil {
			return t, newErr(KindInvalid, "%s must be a date like 2026-10-01", name)
		}
		return t, nil
	}
	to := today.AddDate(0, 0, -1)
	if in.To != "" {
		if to, err = parse(in.To, "to"); err != nil {
			return time.Time{}, time.Time{}, nil, err
		}
	}
	var from time.Time
	switch {
	case in.From != "":
		if from, err = parse(in.From, "from"); err != nil {
			return time.Time{}, time.Time{}, nil, err
		}
	case last != nil:
		end, _ := time.ParseInLocation(haccp.DateLayout, last.Reading.PeriodTo, clock.Dublin)
		from = end.AddDate(0, 0, 1)
	default:
		first, err := st.HACCP().ListRecords(ctx, store.RecordFilter{Limit: 1})
		if err != nil {
			return time.Time{}, time.Time{}, nil, err
		}
		from = today
		if len(first) > 0 {
			f := first[0].LoggedAt.In(clock.Dublin)
			from = time.Date(f.Year(), f.Month(), f.Day(), 0, 0, 0, 0, clock.Dublin)
		}
	}
	if in.To == "" && from.After(to) {
		if in.From == "" && from.After(today) {
			return time.Time{}, time.Time{}, last, errUpToDate
		}
		to = today // nothing left before today: offer today
	}
	switch {
	case to.After(today):
		return time.Time{}, time.Time{}, nil, newErr(KindInvalid, "a sign-off cannot cover days that have not happened yet")
	case from.After(to):
		return time.Time{}, time.Time{}, nil, newErr(KindInvalid, "the period ends before it starts")
	case to.Sub(from) >= haccp.MaxSignOffDays*24*time.Hour:
		return time.Time{}, time.Time{}, nil, newErr(KindInvalid, "a sign-off covers at most %d days: sign off a shorter period", haccp.MaxSignOffDays)
	}
	return from, to, last, nil
}

func (s *Service) previewSignOff(ctx context.Context, st store.Store, in SignOffInput) (SignOffPreview, error) {
	from, to, last, err := s.signOffPeriod(ctx, st, in)
	if errors.Is(err, errUpToDate) {
		return SignOffPreview{UpToDate: true, Last: last, Missed: []books.Gap{}, OutOfLimit: []haccp.Record{}, WithoutAction: []haccp.Record{}, Overlaps: []haccp.Record{}}, nil
	}
	if err != nil {
		return SignOffPreview{}, err
	}
	recs, err := st.HACCP().ListRecords(ctx, store.RecordFilter{From: from})
	if err != nil {
		return SignOffPreview{}, err
	}
	first, err := st.HACCP().ListRecords(ctx, store.RecordFilter{Limit: 1})
	if err != nil {
		return SignOffPreview{}, err
	}
	var since time.Time
	if len(first) > 0 {
		since = first[0].LoggedAt
	}
	lists, err := s.Lists().current(ctx, st)
	if err != nil {
		return SignOffPreview{}, err
	}
	res := books.ReviewPeriod(books.ReviewInput{
		From: from, To: to, Loc: clock.Dublin, Now: s.clk.Now(), Since: since,
		Records: recs, Units: lists.Value.Units,
	})
	pv := SignOffPreview{
		From: from.Format(haccp.DateLayout), To: to.Format(haccp.DateLayout), ReviewResult: res,
		NoteRequired: res.Review.WithoutAction > 0 || res.Review.Missed > 0, Last: last, Overlaps: []haccp.Record{},
	}
	signOffs, err := st.HACCP().ListRecords(ctx, store.RecordFilter{Types: []haccp.RecordType{haccp.RecordSignOff}})
	if err != nil {
		return SignOffPreview{}, err
	}
	for _, so := range signOffs {
		if so.Reading.PeriodTo >= pv.From && so.Reading.PeriodFrom <= pv.To {
			pv.Overlaps = append(pv.Overlaps, so)
		}
	}
	return pv, nil
}

// PreviewSignOff shows what signing off a period would cover: its records,
// out-of-limit readings, readings without a corrective action and missed
// checks. Nothing is saved.
func (s *Service) PreviewSignOff(ctx context.Context, p *auth.Principal, in SignOffInput) (SignOffPreview, error) {
	if err := s.allow(p, auth.ActExportHACCP); err != nil {
		return SignOffPreview{}, err
	}
	return s.previewSignOff(ctx, s.st, in)
}

// SignOff saves a manager's sign-off of a period as a HACCP record. The
// review is counted again when saving, so the record holds what was true
// at that moment. A period with missed checks or readings without a
// corrective action needs a note.
func (s *Service) SignOff(ctx context.Context, p *auth.Principal, in SignOffInput) (haccp.Record, error) {
	if err := s.allow(p, auth.ActSignOff); err != nil {
		return haccp.Record{}, err
	}
	var saved haccp.Record
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		pv, err := s.previewSignOff(ctx, tx, in)
		if err != nil {
			return err
		}
		if pv.UpToDate {
			return errUpToDate
		}
		review := pv.Review
		reading := haccp.Reading{PeriodFrom: pv.From, PeriodTo: pv.To, Review: &review, Notes: in.Note}.Normalize()
		if err := reading.Validate(haccp.RecordSignOff); err != nil {
			return newErr(KindInvalid, "%s", err)
		}
		rules, err := s.Rules().current(ctx, tx)
		if err != nil {
			return err
		}
		reason := fmt.Sprintf("Signed off %s to %s: %d records, %d out of limit, %d without an action, %d missed checks.",
			pv.From, pv.To, review.Records, review.OutOfLimit, review.WithoutAction, review.Missed)
		rec := haccp.Record{
			ID: newID(), Type: haccp.RecordSignOff,
			LoggedBy: p.User.ID, LoggedByName: p.User.Name, LoggedAt: haccp.NormalizeTime(s.clk.Now()),
			Reading: reading, Outcome: haccp.OutcomeRecorded, OutcomeReason: reason, RulesVersionID: rules.Version.ID,
		}
		prev, err := tx.HACCP().LastRecord(ctx)
		if err != nil {
			return err
		}
		saved = haccp.Seal(rec, prev)
		if err := tx.HACCP().AppendRecord(ctx, saved); err != nil {
			return fmt.Errorf("save sign-off: %w", err)
		}
		return nil
	})
	if err == nil {
		s.publish(events.KindRecord, saved.ID)
	}
	return saved, err
}
