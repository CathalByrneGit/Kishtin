package service

import (
	"context"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// TodayView is the Today screen: the day's checks and the restaurant name.
type TodayView struct {
	RestaurantName string `json:"restaurant_name"`
	haccp.Today
	// CorrectiveActions are the quick-pick actions, for the log form.
	CorrectiveActions []haccp.ActionPreset `json:"corrective_actions"`
	// SignOff says how far the records are signed off by a manager.
	SignOff SignOffStatus `json:"sign_off"`
}

// SignOffStatus is how far the records are signed off.
type SignOffStatus struct {
	// SignedTo is the last day covered by a sign-off, if any.
	SignedTo string `json:"signed_to,omitempty"`
	// UnsignedFrom is the first day not covered since then (or since
	// records began), when there is one before today.
	UnsignedFrom string `json:"unsigned_from,omitempty"`
	// UnsignedDays counts the days from UnsignedFrom up to yesterday.
	UnsignedDays int `json:"unsigned_days"`
}

// signOffStatus works out how far the records are signed off.
func (s *Service) signOffStatus(ctx context.Context) (SignOffStatus, error) {
	var st SignOffStatus
	now := s.clk.Now().In(clock.Dublin)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, clock.Dublin)
	last, err := s.lastSignOff(ctx, s.st)
	if err != nil {
		return st, err
	}
	var from time.Time
	if last != nil {
		st.SignedTo = last.Reading.PeriodTo
		end, _ := time.ParseInLocation(haccp.DateLayout, last.Reading.PeriodTo, clock.Dublin)
		from = end.AddDate(0, 0, 1)
	} else {
		first, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{Limit: 1})
		if err != nil || len(first) == 0 {
			return st, err
		}
		f := first[0].LoggedAt.In(clock.Dublin)
		from = time.Date(f.Year(), f.Month(), f.Day(), 0, 0, 0, 0, clock.Dublin)
	}
	if from.Before(today) {
		st.UnsignedFrom = from.Format(haccp.DateLayout)
		st.UnsignedDays = int(today.Sub(from).Hours()/24 + 0.5)
	}
	return st, nil
}

// Today returns the day's checks: what is done, due, missed and out of
// limit, the cleaning checklist, and batches in use.
func (s *Service) Today(ctx context.Context, p *auth.Principal) (TodayView, error) {
	if err := s.allow(p, auth.ActViewHACCP); err != nil {
		return TodayView{}, err
	}
	from, to := s.dayBounds()
	recs, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{DoneFrom: from, DoneTo: to})
	if err != nil {
		return TodayView{}, err
	}
	batches, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{Types: []haccp.RecordType{haccp.RecordBatchLabel}, Newest: true, Limit: 500})
	if err != nil {
		return TodayView{}, err
	}
	lists, err := s.Lists().current(ctx, s.st)
	if err != nil {
		return TodayView{}, err
	}
	limits, err := s.st.HACCP().ListLimitVersions(ctx)
	if err != nil {
		return TodayView{}, err
	}
	sys, err := s.System().current(ctx, s.st)
	if err != nil {
		return TodayView{}, err
	}
	t := haccp.BuildToday(haccp.TodayInput{
		Now: s.clk.Now(), Loc: clock.Dublin, Records: recs, BatchLabels: batches,
		Lists: lists.Value, Limits: limits,
	})
	so, err := s.signOffStatus(ctx)
	if err != nil {
		return TodayView{}, err
	}
	return TodayView{RestaurantName: sys.Value.RestaurantName, Today: t, CorrectiveActions: lists.Value.CorrectiveActions, SignOff: so}, nil
}
