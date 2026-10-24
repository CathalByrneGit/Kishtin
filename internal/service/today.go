package service

import (
	"context"

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
}

// Today returns the day's checks: what is done, due, missed and out of
// limit, the cleaning checklist, and batches in use.
func (s *Service) Today(ctx context.Context, p *auth.Principal) (TodayView, error) {
	if err := allow(p, auth.ActViewHACCP); err != nil {
		return TodayView{}, err
	}
	from, to := s.dayBounds()
	recs, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{From: from, To: to})
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
	return TodayView{RestaurantName: sys.Value.RestaurantName, Today: t, CorrectiveActions: lists.Value.CorrectiveActions}, nil
}
