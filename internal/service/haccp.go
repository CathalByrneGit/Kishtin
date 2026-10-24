package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// NewRecord is a HACCP record to check or save.
type NewRecord struct {
	Type             haccp.RecordType `json:"type"`
	Reading          haccp.Reading    `json:"reading"`
	CorrectiveAction string           `json:"corrective_action,omitempty"`
	// Corrects is the ID of an earlier record this one corrects.
	Corrects         string `json:"corrects,omitempty"`
	CorrectionReason string `json:"correction_reason,omitempty"`
}

// Check is the result of checking a reading before it is saved.
type Check struct {
	Result haccp.Result `json:"result"`
	// Limit is the limit version the reading was checked against, if any.
	Limit                 *haccp.LimitVersion `json:"limit,omitempty"`
	NeedsCorrectiveAction bool                `json:"needs_corrective_action"`
	// RulesVersionID is the food-safety rules version that decided whether
	// a corrective action is needed.
	RulesVersionID string `json:"rules_version_id"`
}

// prepare normalizes and validates a new record and checks it against the
// current limits. It reads through tx, so a save sees the same limits.
func (s *Service) prepare(ctx context.Context, tx store.Store, nr *NewRecord) (Check, error) {
	if !nr.Type.Valid() {
		return Check{}, newErr(KindInvalid, "unknown record type %q", nr.Type)
	}
	nr.Reading = nr.Reading.Normalize()
	nr.CorrectiveAction = strings.TrimSpace(nr.CorrectiveAction)
	nr.Corrects = strings.TrimSpace(nr.Corrects)
	nr.CorrectionReason = strings.TrimSpace(nr.CorrectionReason)
	if err := nr.Reading.Validate(nr.Type); err != nil {
		return Check{}, newErr(KindInvalid, "%s", err)
	}
	rules, err := s.Rules().current(ctx, tx)
	if err != nil {
		return Check{}, err
	}
	lists, err := s.Lists().current(ctx, tx)
	if err != nil {
		return Check{}, err
	}
	if nr.Reading, err = lists.Value.Apply(nr.Type, nr.Reading, rules.Value); err != nil {
		return Check{}, newErr(KindInvalid, "%s", err)
	}
	if f := nr.Reading.DocketFile; f != "" {
		if !files.ValidHash(f) {
			return Check{}, newErr(KindInvalid, "docket_file must be the SHA-256 of an uploaded file")
		}
		if _, err := tx.Files().GetFile(ctx, f); err != nil {
			return Check{}, mapStoreErr(err, "attached file")
		}
	}

	c := Check{RulesVersionID: rules.Version.ID}
	if check, ok := haccp.CheckFor(nr.Type, nr.Reading); ok {
		history, err := tx.HACCP().ListLimitVersions(ctx)
		if err != nil {
			return Check{}, err
		}
		lv := haccp.Resolve(history, check, haccp.Scopes(nr.Type, nr.Reading))
		c.Limit = &lv
	}
	res, err := haccp.Evaluate(nr.Type, nr.Reading, c.Limit)
	if err != nil {
		return Check{}, newErr(KindInvalid, "%s", err)
	}
	c.Result = res
	c.NeedsCorrectiveAction = rules.Value.NeedsAction(res.Outcome)
	return c, nil
}

// CheckRecord checks a reading without saving it, so a screen can show the
// result before the record is saved.
func (s *Service) CheckRecord(ctx context.Context, p *auth.Principal, nr NewRecord) (Check, error) {
	if err := allow(p, auth.ActLogHACCP); err != nil {
		return Check{}, err
	}
	return s.prepare(ctx, s.st, &nr)
}

// LogRecord checks and saves a HACCP record. A failed reading, and a
// borderline one if the rules say so, is only saved with a corrective
// action.
func (s *Service) LogRecord(ctx context.Context, p *auth.Principal, nr NewRecord) (haccp.Record, error) {
	if err := allow(p, auth.ActLogHACCP); err != nil {
		return haccp.Record{}, err
	}
	var saved haccp.Record
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		c, err := s.prepare(ctx, tx, &nr)
		if err != nil {
			return err
		}
		if c.NeedsCorrectiveAction && nr.CorrectiveAction == "" {
			return newErr(KindInvalid, "%s %s Add a corrective action to save it.", outcomeLabel(c.Result.Outcome), c.Result.Reason)
		}
		if nr.Corrects != "" {
			orig, err := tx.HACCP().GetRecord(ctx, nr.Corrects)
			if err != nil {
				return mapStoreErr(err, "the record being corrected")
			}
			if orig.Type != nr.Type {
				return newErr(KindInvalid, "a correction must be the same type as the record it corrects (%s)", orig.Type)
			}
			if nr.CorrectionReason == "" {
				return newErr(KindInvalid, "say why the original record was wrong")
			}
		} else if nr.CorrectionReason != "" {
			return newErr(KindInvalid, "correction_reason is only for a record that corrects another")
		}

		rec := haccp.Record{
			ID:               newID(),
			Type:             nr.Type,
			LoggedBy:         p.User.ID,
			LoggedByName:     p.User.Name,
			LoggedAt:         haccp.NormalizeTime(s.clk.Now()),
			Reading:          nr.Reading,
			Outcome:          c.Result.Outcome,
			OutcomeReason:    c.Result.Reason,
			RulesVersionID:   c.RulesVersionID,
			CorrectiveAction: nr.CorrectiveAction,
			Corrects:         nr.Corrects,
			CorrectionReason: nr.CorrectionReason,
		}
		if c.Limit != nil {
			rec.LimitVersionID = c.Limit.ID
			sc := c.Limit.Scope
			rec.LimitScope = &sc
		}
		prev, err := tx.HACCP().LastRecord(ctx)
		if err != nil {
			return err
		}
		saved = haccp.Seal(rec, prev)
		if err := tx.HACCP().AppendRecord(ctx, saved); err != nil {
			return fmt.Errorf("save record: %w", err)
		}
		return nil
	})
	return saved, err
}

func outcomeLabel(o haccp.Outcome) string {
	switch o {
	case haccp.OutcomeFail:
		return "This reading failed:"
	case haccp.OutcomeBorderline:
		return "This reading is borderline:"
	}
	return ""
}

// GetRecord returns one record.
func (s *Service) GetRecord(ctx context.Context, p *auth.Principal, id string) (haccp.Record, error) {
	if err := allow(p, auth.ActViewHACCP); err != nil {
		return haccp.Record{}, err
	}
	r, err := s.st.HACCP().GetRecord(ctx, id)
	if err != nil {
		return haccp.Record{}, mapStoreErr(err, "record")
	}
	return r, nil
}

// RecordQuery selects records to list.
type RecordQuery struct {
	From, To time.Time
	Types    []haccp.RecordType
	Limit    int
	Newest   bool
}

// MaxListLimit caps one page of records.
const MaxListLimit = 1000

// ListRecords lists records.
func (s *Service) ListRecords(ctx context.Context, p *auth.Principal, q RecordQuery) ([]haccp.Record, error) {
	if err := allow(p, auth.ActViewHACCP); err != nil {
		return nil, err
	}
	for _, t := range q.Types {
		if !t.Valid() {
			return nil, newErr(KindInvalid, "unknown record type %q", t)
		}
	}
	if !q.From.IsZero() && !q.To.IsZero() && !q.To.After(q.From) {
		return nil, newErr(KindInvalid, "the end of the range must be after the start")
	}
	if q.Limit <= 0 || q.Limit > MaxListLimit {
		q.Limit = MaxListLimit
	}
	return s.st.HACCP().ListRecords(ctx, store.RecordFilter{From: q.From, To: q.To, Types: q.Types, Limit: q.Limit, Newest: q.Newest})
}

// FileProblem is an attached file that failed its check.
type FileProblem struct {
	SHA256  string `json:"sha256"`
	Problem string `json:"problem"`
}

// VerifyReport is the result of checking every HACCP record and file.
type VerifyReport struct {
	Chain haccp.ChainReport `json:"chain"`
	// UnknownLimits lists records that name a limit version that doesn't
	// exist.
	UnknownLimits []string      `json:"unknown_limits,omitempty"`
	FilesChecked  int           `json:"files_checked"`
	FileProblems  []FileProblem `json:"file_problems,omitempty"`
	OK            bool          `json:"ok"`
}

// Verify re-checks the hash chain, the limit versions records refer to, and
// every attached file.
func (s *Service) Verify(ctx context.Context, p *auth.Principal) (VerifyReport, error) {
	if err := allow(p, auth.ActViewHACCP); err != nil {
		return VerifyReport{}, err
	}
	return s.verifyStore(ctx, s.st, true)
}

func (s *Service) verifyStore(ctx context.Context, st store.Store, checkFiles bool) (VerifyReport, error) {
	var rep VerifyReport
	history, err := st.HACCP().ListLimitVersions(ctx)
	if err != nil {
		return rep, err
	}
	known := map[string]bool{}
	for _, lv := range history {
		known[lv.ID] = true
	}
	var v haccp.ChainVerifier
	err = st.HACCP().EachRecord(ctx, func(r haccp.Record) error {
		if !v.Add(r) {
			return errStop
		}
		if r.LimitVersionID != "" && !known[r.LimitVersionID] {
			rep.UnknownLimits = append(rep.UnknownLimits, r.ID)
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		return rep, fmt.Errorf("read records: %w", err)
	}
	rep.Chain = v.Report()

	if checkFiles && s.files != nil {
		metas, err := st.Files().ListFiles(ctx)
		if err != nil {
			return rep, err
		}
		for _, m := range metas {
			rep.FilesChecked++
			if err := s.files.Verify(m.SHA256); err != nil {
				problem := err.Error()
				if errors.Is(err, files.ErrNotFound) {
					problem = "file is missing from disk"
				}
				rep.FileProblems = append(rep.FileProblems, FileProblem{SHA256: m.SHA256, Problem: problem})
			}
		}
	}
	rep.OK = rep.Chain.OK && len(rep.UnknownLimits) == 0 && len(rep.FileProblems) == 0
	return rep, nil
}

var errStop = errors.New("stop")
