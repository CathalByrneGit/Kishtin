package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/analytics"
	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// maxReportDays is the longest period a report covers.
const maxReportDays = 366

// reportPeriod reads a report's first and last day (YYYY-MM-DD, Dublin).
// Empty dates mean the 28 days up to today.
func (s *Service) reportPeriod(fromDay, toDay string) (time.Time, time.Time, error) {
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
		return to, to, err
	}
	from, err := parse(fromDay, "from", to.AddDate(0, 0, -27))
	if err != nil {
		return from, to, err
	}
	switch {
	case from.After(to):
		return from, to, newErr(KindInvalid, "the period ends before it starts")
	case to.Sub(from) >= maxReportDays*24*time.Hour:
		return from, to, newErr(KindInvalid, "a report covers %d days at most", maxReportDays)
	}
	return from, to, nil
}

// ReportHACCP works out how well the daily HACCP checks were kept up in a
// period: checks done and missed, cleaning, readings out of limit, and each
// fridge and freezer's temperatures.
func (s *Service) ReportHACCP(ctx context.Context, p *auth.Principal, fromDay, toDay string) (analytics.HACCPReport, error) {
	if err := s.allow(p, auth.ActExportHACCP); err != nil {
		return analytics.HACCPReport{}, err
	}
	from, to, err := s.reportPeriod(fromDay, toDay)
	if err != nil {
		return analytics.HACCPReport{}, err
	}
	recs, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{From: from})
	if err != nil {
		return analytics.HACCPReport{}, err
	}
	recs = s.visible(p, recs)
	first, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{Limit: 1})
	if err != nil {
		return analytics.HACCPReport{}, err
	}
	var since time.Time
	if len(first) > 0 {
		since = first[0].LoggedAt
	}
	lists, err := s.Lists().current(ctx, s.st)
	if err != nil {
		return analytics.HACCPReport{}, err
	}
	limits, err := s.st.HACCP().ListLimitVersions(ctx)
	if err != nil {
		return analytics.HACCPReport{}, err
	}
	return analytics.BuildHACCP(analytics.HACCPInput{
		From: from, To: to, Loc: clock.Dublin, Now: s.clk.Now(), Since: since, Records: recs,
		Units: lists.Value.Units, CleaningTasks: lists.Value.CleaningTasks, Limits: limits,
	}), nil
}

// ReportPass works out how fast courses reached the pass in a period, why
// late ones were late, and how each dish's actual times compare with its
// set times.
func (s *Service) ReportPass(ctx context.Context, p *auth.Principal, fromDay, toDay string) (analytics.PassReport, error) {
	if err := s.allow(p, auth.ActRunPass); err != nil {
		return analytics.PassReport{}, err
	}
	from, to, err := s.reportPeriod(fromDay, toDay)
	if err != nil {
		return analytics.PassReport{}, err
	}
	end := to.AddDate(0, 0, 1)
	// Orders taken the day before can have courses fired in the period.
	os, err := s.st.Orders().ListOrders(ctx, store.OrderFilter{From: from.AddDate(0, 0, -1), To: end})
	if err != nil {
		return analytics.PassReport{}, err
	}
	snaps, err := s.st.Activity().ListPlanSnapshots(ctx, from, end.AddDate(0, 0, 1), 0)
	if err != nil {
		return analytics.PassReport{}, err
	}
	var seen []analytics.CourseSeen
	for _, sn := range snaps {
		var sp SnapshotPlan
		if err := json.Unmarshal(sn.Body, &sp); err != nil {
			return analytics.PassReport{}, fmt.Errorf("decode plan snapshot %s: %w", sn.ID, err)
		}
		for _, c := range sp.Courses {
			if c.Cause != "" {
				seen = append(seen, analytics.CourseSeen{At: sn.At, CourseStatus: c})
			}
		}
	}
	return analytics.BuildPass(analytics.PassInput{From: from, To: to, Loc: clock.Dublin, Orders: os, Seen: seen}), nil
}
