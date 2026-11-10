package service

import (
	"context"
	"io"
	"strings"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/report"
)

// ExportPackPDF writes the inspection pack for the records matching q: a
// summary with the hash chain and file checks, failed and borderline
// records with their actions, every record by day, and the limit versions
// they were checked against.
func (s *Service) ExportPackPDF(ctx context.Context, p *auth.Principal, q RecordQuery, w io.Writer) (ExportSummary, error) {
	if err := s.allow(p, auth.ActExportHACCP); err != nil {
		return ExportSummary{}, err
	}
	sum := ExportSummary{From: q.From, To: q.To}
	recs, err := s.exportRecords(ctx, q)
	if err != nil {
		return sum, err
	}
	recs = s.visible(p, recs)
	rep, err := s.verifyStore(ctx, s.st, true)
	if err != nil {
		return sum, err
	}
	sum.Chain, sum.Records = rep.Chain, len(recs)

	history, err := s.st.HACCP().ListLimitVersions(ctx)
	if err != nil {
		return sum, err
	}
	used := map[string]bool{}
	for _, r := range recs {
		used[r.LimitVersionID] = true
	}
	var limits []haccp.LimitVersion
	for _, lv := range history {
		if used[lv.ID] {
			limits = append(limits, lv)
		}
	}
	guidance, err := s.Guidance().current(ctx, s.st)
	if err != nil {
		return sum, err
	}
	sys, err := s.System().current(ctx, s.st)
	if err != nil {
		return sum, err
	}
	err = report.Write(report.Pack{
		RestaurantName: sys.Value.RestaurantName,
		From:           q.From, To: q.To,
		GeneratedAt: s.clk.Now(), GeneratedBy: p.User.Name, Loc: clock.Dublin,
		Records: recs, Chain: rep.Chain,
		FilesChecked: rep.FilesChecked, FileProblems: len(rep.FileProblems),
		Guidance: guidance.Value.Name + " (version " + guidance.Version.ID + ")",
		Limits:   limits,
	}, w)
	return sum, err
}

// BatchLabelPDF writes a batch label record as a label to print, in one of
// report.LabelSizes ("" is the default), and returns the record.
func (s *Service) BatchLabelPDF(ctx context.Context, p *auth.Principal, id, size string, w io.Writer) (haccp.Record, error) {
	r, err := s.GetRecord(ctx, p, id)
	if err != nil {
		return haccp.Record{}, err
	}
	if r.Type != haccp.RecordBatchLabel {
		return r, newErr(KindInvalid, "record %d is not a batch label: only batch labels print as labels", r.Seq)
	}
	sz, ok := report.FindLabelSize(size)
	if !ok {
		ids := make([]string, len(report.LabelSizes))
		for i, s := range report.LabelSizes {
			ids[i] = s.ID
		}
		return r, newErr(KindInvalid, "label size %q is not one of %s", size, strings.Join(ids, ", "))
	}
	if err := report.WriteLabel(report.Label{Record: r, Size: sz, Loc: clock.Dublin}, w); err != nil {
		return r, err
	}
	return r, nil
}
