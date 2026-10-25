package service

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/books"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/report"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// RecordBookSettings is the record book bundle in use and the restaurant's
// changes to its booklets.
func (s *Service) RecordBookSettings() *Group[books.RecordBookSettings] {
	return &Group[books.RecordBookSettings]{s: s, spec: groupSpec[books.RecordBookSettings]{
		kind: settings.KindRecordBooks, builtinID: books.BuiltinSettingsID, seeded: true,
		builtin:   func(*Service) books.RecordBookSettings { return books.DefaultSettings() },
		normalize: books.RecordBookSettings.Normalize,
		validate: func(v books.RecordBookSettings) error {
			bundles, err := books.Builtin()
			if err != nil {
				return err
			}
			return v.Validate(bundles)
		},
		lessStrict: func(_, _ books.RecordBookSettings) []string { return nil },
		notes:      recordBookNotes,
		changes: func(cur, next books.RecordBookSettings) []string {
			bundles, err := books.Builtin()
			if err != nil {
				return nil
			}
			return books.Summarize(bundles, cur, next)
		},
		edit: auth.ActManageLimits,
	}}
}

// recordBookNotes says what a record book change does to the booklets.
func recordBookNotes(_ context.Context, _ *Service, _ store.Store, cur, next books.RecordBookSettings) ([]string, error) {
	bundles, err := books.Builtin()
	if err != nil {
		return nil, err
	}
	var head, out []string
	_, before, err := books.Resolve(bundles, cur)
	if err != nil {
		before = nil
	}
	bundle, after, err := books.Resolve(bundles, next)
	if err != nil {
		return []string{"Saved records do not change. Only how record books are printed changes."}, nil
	}
	if cur.Bundle != next.Bundle {
		head = append(head, fmt.Sprintf("Record books will use the %s (%s).", bundle.Name, bundle.Country))
	}
	old := map[string]books.Booklet{}
	for _, b := range before {
		old[b.Template.ID] = b
	}
	for _, b := range after {
		was, existed := old[b.Template.ID]
		if existed {
			var gone []string
			for _, c := range was.Template.Columns {
				if c.Field != "" && !slices.ContainsFunc(b.Template.Columns, func(n books.Column) bool { return n.Field == c.Field }) {
					gone = append(gone, c.Label)
				}
			}
			if len(gone) > 0 {
				out = append(out, fmt.Sprintf("%s will no longer show: %s.", b.Template.Code, strings.Join(gone, ", ")))
			}
		}
		if c := b.Copy; c != nil && c.Known && !c.Matches && (!existed || was.Copy == nil || was.Copy.File != c.File) {
			out = append(out, fmt.Sprintf("%s: the uploaded copy is not the %s edition this booklet was made from. Check its columns match your copy, and change them here if they do not.",
				b.Template.Code, b.Template.Source.Published))
		}
	}
	slices.Sort(out)
	out = append(head, out...)
	return append(out, "Saved records do not change. Only how record books are printed changes."), nil
}

// RecordBooks is the bundle in use, its booklets as the restaurant uses
// them, and what else is available for setting them up.
type RecordBooks struct {
	Bundle   books.Bundle    `json:"bundle"`
	Booklets []books.Booklet `json:"booklets"`
	// SettingsVersion is the record book settings version in force.
	SettingsVersion string `json:"settings_version"`
	// Bundles are every bundle Kishtin has.
	Bundles []books.Bundle `json:"bundles"`
	// Builders are the kinds of booklet, with the fields each can show.
	Builders []books.Builder `json:"builders"`
}

func (s *Service) recordBooks(ctx context.Context) (RecordBooks, error) {
	bundles, err := books.Builtin()
	if err != nil {
		return RecordBooks{}, err
	}
	set, err := s.RecordBookSettings().current(ctx, s.st)
	if err != nil {
		return RecordBooks{}, err
	}
	bundle, booklets, err := books.Resolve(bundles, set.Value)
	if err != nil {
		return RecordBooks{}, fmt.Errorf("record book settings %s: %w", set.Version.ID, err)
	}
	return RecordBooks{
		Bundle: bundle, Booklets: booklets, SettingsVersion: set.Version.ID,
		Bundles: bundles, Builders: books.Builders,
	}, nil
}

// RecordBooks lists the record books in use, with the booklets Kishtin
// cannot fill yet and why.
func (s *Service) RecordBooks(ctx context.Context, p *auth.Principal) (RecordBooks, error) {
	if err := allow(p, auth.ActExportHACCP); err != nil {
		return RecordBooks{}, err
	}
	return s.recordBooks(ctx)
}

// RecordBook fills in a record book (by ID or code, such as "SC2") for a
// month (YYYY-MM; empty means this month) from the records.
func (s *Service) RecordBook(ctx context.Context, p *auth.Principal, ref, month string) (books.Book, error) {
	if err := allow(p, auth.ActExportHACCP); err != nil {
		return books.Book{}, err
	}
	rb, err := s.recordBooks(ctx)
	if err != nil {
		return books.Book{}, err
	}
	bk, ok := books.Find(rb.Booklets, ref)
	if !ok {
		return books.Book{}, newErr(KindNotFound, "no record book called %q in the %s: run kishtin haccp books to list them", ref, rb.Bundle.Name)
	}
	tpl := bk.Template
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
		Template: tpl, Bundle: rb.Bundle, Year: start.Year(), Month: start.Month(), Loc: clock.Dublin,
		Now: now, Since: since, Records: recs, Units: lists.Value.Units,
	})
	if err != nil {
		return books.Book{}, fmt.Errorf("fill %s: %w", tpl.Code, err)
	}
	b.Changed, b.SettingsVersion = bk.Changed, rb.SettingsVersion
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
