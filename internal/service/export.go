package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// ExportSummary describes an export, so the numbers can be checked.
type ExportSummary struct {
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Records int       `json:"records"`
	// Chain is the state of the whole chain when the export was made.
	Chain haccp.ChainReport `json:"chain"`
}

var recordColumns = []string{
	"seq", "record_id", "type", "logged_at_utc", "logged_at_dublin",
	"logged_by", "logged_by_name", "subject", "temp_c", "outcome",
	"outcome_reason", "corrective_action", "limit_version_id", "limit_scope",
	"corrects", "correction_reason", "reading_json", "prev_hash", "hash",
}

// ExportRecordsCSV writes the records logged in [from, to) as CSV for an
// inspection. The summary includes the chain head so a copy of the export
// can be checked against the live system later.
func (s *Service) ExportRecordsCSV(ctx context.Context, p *auth.Principal, from, to time.Time, w io.Writer) (ExportSummary, error) {
	if err := allow(p, auth.ActExportHACCP); err != nil {
		return ExportSummary{}, err
	}
	if from.IsZero() || to.IsZero() || !to.After(from) {
		return ExportSummary{}, newErr(KindInvalid, "choose a date range with the end after the start")
	}
	sum := ExportSummary{From: from, To: to}
	recs, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{From: from, To: to})
	if err != nil {
		return sum, err
	}
	rep, err := s.verifyStore(ctx, s.st, false)
	if err != nil {
		return sum, err
	}
	sum.Chain = rep.Chain

	cw := csv.NewWriter(w)
	if err := cw.Write(recordColumns); err != nil {
		return sum, fmt.Errorf("write CSV: %w", err)
	}
	for _, r := range recs {
		reading, err := json.Marshal(r.Reading)
		if err != nil {
			return sum, fmt.Errorf("encode reading: %w", err)
		}
		var temp, scope string
		if r.Reading.TempC != nil {
			temp = strconv.FormatFloat(*r.Reading.TempC, 'f', 1, 64)
		} else if r.Reading.TempAtCutoffC != nil {
			temp = strconv.FormatFloat(*r.Reading.TempAtCutoffC, 'f', 1, 64)
		}
		if r.LimitScope != nil {
			scope = r.LimitScope.String()
		}
		row := []string{
			strconv.FormatInt(r.Seq, 10), r.ID, string(r.Type),
			r.LoggedAt.UTC().Format(time.RFC3339Nano),
			r.LoggedAt.In(clock.Dublin).Format("2006-01-02 15:04:05"),
			r.LoggedBy, cell(r.LoggedByName), cell(subject(r)), temp, string(r.Outcome),
			cell(r.OutcomeReason), cell(r.CorrectiveAction), r.LimitVersionID, cell(scope),
			r.Corrects, cell(r.CorrectionReason), cell(string(reading)), r.PrevHash, r.Hash,
		}
		if err := cw.Write(row); err != nil {
			return sum, fmt.Errorf("write CSV: %w", err)
		}
		sum.Records++
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return sum, fmt.Errorf("write CSV: %w", err)
	}
	return sum, nil
}

var limitColumns = []string{
	"limit_version_id", "check", "scope", "max_c", "borderline_max_c", "min_c",
	"combos", "cooling", "non_food", "below_fsai", "supersedes",
	"created_by", "created_at_utc", "reason", "plan_ref",
}

// ExportLimitsCSV writes the full limit history as CSV, to go with a
// records export.
func (s *Service) ExportLimitsCSV(ctx context.Context, p *auth.Principal, w io.Writer) error {
	if err := allow(p, auth.ActExportHACCP); err != nil {
		return err
	}
	history, err := s.st.HACCP().ListLimitVersions(ctx)
	if err != nil {
		return err
	}
	num := func(f *float64) string {
		if f == nil {
			return ""
		}
		return strconv.FormatFloat(*f, 'f', 1, 64)
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(limitColumns); err != nil {
		return fmt.Errorf("write CSV: %w", err)
	}
	for _, lv := range history {
		var combos []string
		for _, c := range lv.Combos {
			combos = append(combos, fmt.Sprintf("%.1f°C/%ds", c.MinC, c.HoldSeconds))
		}
		var cooling string
		if c := lv.Cooling; c != nil {
			cooling = fmt.Sprintf("into fridge within %ds; cutoff %ds at %.1f°C", c.IntoFridgeWithinSeconds, c.CutoffAfterSeconds, c.CutoffMaxC)
		}
		row := []string{
			lv.ID, string(lv.Check), cell(lv.Scope.String()), num(lv.MaxC), num(lv.BorderlineMaxC), num(lv.MinC),
			strings.Join(combos, "; "), cooling, strconv.FormatBool(lv.NonFood),
			cell(strings.Join(lv.LooserThanFSAI(), "; ")), lv.Supersedes,
			lv.CreatedBy, lv.CreatedAt.UTC().Format(time.RFC3339Nano), cell(lv.Reason), cell(lv.PlanRef),
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write CSV: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// cell guards free-text CSV cells against spreadsheet formula injection by
// prefixing a quote to text that starts with a formula character.
func cell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
