package service

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/books"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/report"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// RecordBookPacks lists the record book packs Kishtin can fill in, with
// the booklets it cannot fill yet and why.
func (s *Service) RecordBookPacks(_ context.Context, p *auth.Principal) ([]books.Pack, error) {
	if err := allow(p, auth.ActExportHACCP); err != nil {
		return nil, err
	}
	return books.Builtin()
}

// RecordBook fills in a record book (by ID or code, such as "SC2") for a
// month (YYYY-MM; empty means this month) from the records.
func (s *Service) RecordBook(ctx context.Context, p *auth.Principal, ref, month string) (books.Book, error) {
	if err := allow(p, auth.ActExportHACCP); err != nil {
		return books.Book{}, err
	}
	packs, err := books.Builtin()
	if err != nil {
		return books.Book{}, err
	}
	tpl, pack, ok := books.Find(packs, ref)
	if !ok {
		return books.Book{}, newErr(KindNotFound, "no record book called %q: run kishtin haccp books to list them", ref)
	}
	if !tpl.Supported() {
		return books.Book{}, newErr(KindInvalid, "Kishtin cannot fill %s yet: %s", tpl.Code, tpl.Missing)
	}
	now := s.clk.Now()
	start := time.Date(now.In(clock.Dublin).Year(), now.In(clock.Dublin).Month(), 1, 0, 0, 0, 0, clock.Dublin)
	if month != "" {
		m, err := time.ParseInLocation("2006-01", month, clock.Dublin)
		if err != nil {
			return books.Book{}, newErr(KindInvalid, "month must look like 2026-10")
		}
		if m.After(start) {
			return books.Book{}, newErr(KindInvalid, "choose this month or an earlier one")
		}
		start = m
	}
	// Corrections can be logged after the month ends, so read on to now.
	recs, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{From: start})
	if err != nil {
		return books.Book{}, err
	}
	first, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{Limit: 1})
	if err != nil {
		return books.Book{}, err
	}
	var since time.Time
	if len(first) > 0 {
		since = first[0].LoggedAt
	}
	lists, err := s.Lists().current(ctx, s.st)
	if err != nil {
		return books.Book{}, err
	}
	b, err := books.Build(books.Input{
		Template: tpl, Pack: pack, Year: start.Year(), Month: start.Month(), Loc: clock.Dublin,
		Now: now, Since: since, Records: recs, Units: lists.Value.Units,
	})
	if err != nil {
		return books.Book{}, fmt.Errorf("fill %s: %w", tpl.Code, err)
	}
	return b, nil
}

// RecordBookPDF writes a filled-in record book as a PDF, with the state of
// the hash chain in its footer.
func (s *Service) RecordBookPDF(ctx context.Context, p *auth.Principal, ref, month string, w io.Writer) (books.Book, ExportSummary, error) {
	b, err := s.RecordBook(ctx, p, ref, month)
	if err != nil {
		return b, ExportSummary{}, err
	}
	rep, err := s.verifyStore(ctx, s.st, false)
	if err != nil {
		return b, ExportSummary{}, err
	}
	sys, err := s.System().current(ctx, s.st)
	if err != nil {
		return b, ExportSummary{}, err
	}
	sum := ExportSummary{Records: b.Records, Chain: rep.Chain}
	err = report.WriteBook(report.Book{
		Book: b, RestaurantName: sys.Value.RestaurantName,
		GeneratedAt: s.clk.Now(), GeneratedBy: p.User.Name, Loc: clock.Dublin, Chain: rep.Chain,
	}, w)
	return b, sum, err
}
