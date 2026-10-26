package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/ai"
	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// PaperTypes are the record types that can be transcribed from paper:
// readings with a temperature, from the fridge, cooking, hot holding,
// delivery and transport booklets.
var PaperTypes = []haccp.RecordType{
	haccp.RecordFridge, haccp.RecordFreezer, haccp.RecordCook, haccp.RecordReheat,
	haccp.RecordHotHold, haccp.RecordDelivery, haccp.RecordTransport,
}

// PaperImport is the rows typed (or read) from a photo of a paper page.
// Each row needs reading.taken_at: when the check was done.
type PaperImport struct {
	// File is the SHA-256 of the uploaded photo of the page.
	File string      `json:"file"`
	Rows []NewRecord `json:"rows"`
}

// PaperCheck is what one row would be saved as, or why it cannot be.
type PaperCheck struct {
	Row     int    `json:"row"`
	Check   *Check `json:"check,omitempty"`
	Problem string `json:"problem,omitempty"`
}

// maxPaperRows is the most rows one photo can hold.
const maxPaperRows = 200

func (s *Service) checkPaperRow(ctx context.Context, st store.Store, p *auth.Principal, file string, nr *NewRecord) (Check, error) {
	if !slices.Contains(PaperTypes, nr.Type) {
		return Check{}, newErr(KindInvalid, "a %s record cannot be transcribed from paper", nr.Type)
	}
	nr.Reading.PaperFile = file
	return s.checkEarlierRow(ctx, st, p, nr)
}

// checkEarlierRow checks one row of checks done earlier (from paper or
// caught up), each with the time it was done.
func (s *Service) checkEarlierRow(ctx context.Context, st store.Store, p *auth.Principal, nr *NewRecord) (Check, error) {
	if nr.Reading.TakenAt == nil {
		return Check{}, newErr(KindInvalid, "say when the check was done (date and time)")
	}
	if nr.Corrects != "" {
		return Check{}, newErr(KindInvalid, "a correction is logged as usual, from the record it corrects")
	}
	if err := s.checkWhen(ctx, st, p, nr); err != nil {
		return Check{}, err
	}
	return s.prepare(ctx, st, nr)
}

func (s *Service) paperFile(ctx context.Context, file string) (string, error) {
	_, meta, err := s.readFileMeta(ctx, file)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(meta.MIME, "image/") && meta.MIME != "application/pdf" {
		return "", newErr(KindInvalid, "the page must be a photo or a PDF")
	}
	return meta.SHA256, nil
}

// readFileMeta checks an uploaded file exists.
func (s *Service) readFileMeta(ctx context.Context, sha string) (string, store.FileMeta, error) {
	sha = strings.ToLower(strings.TrimSpace(sha))
	m, err := s.st.Files().GetFile(ctx, sha)
	if err != nil {
		return "", m, mapStoreErr(err, "photo of the page")
	}
	return sha, m, nil
}

// PreviewPaper checks the rows transcribed from a paper page, without
// saving: each row's result against today's limits, and anything that
// stops it being saved.
func (s *Service) PreviewPaper(ctx context.Context, p *auth.Principal, in PaperImport) ([]PaperCheck, error) {
	if err := s.allow(p, auth.ActInspect); err != nil {
		return nil, err
	}
	file, err := s.paperFile(ctx, in.File)
	if err != nil {
		return nil, err
	}
	if len(in.Rows) == 0 || len(in.Rows) > maxPaperRows {
		return nil, newErr(KindInvalid, "send 1 to %d rows", maxPaperRows)
	}
	return s.previewRows(ctx, in.Rows, "Copy the action taken from the paper: this reading needs one.",
		func(nr *NewRecord) (Check, error) { return s.checkPaperRow(ctx, s.st, p, file, nr) }), nil
}

// previewRows checks rows of records without saving them.
func (s *Service) previewRows(_ context.Context, rows []NewRecord, needAction string, check func(*NewRecord) (Check, error)) []PaperCheck {
	out := make([]PaperCheck, 0, len(rows))
	for i, nr := range rows {
		c, err := check(&nr)
		pc := PaperCheck{Row: i + 1}
		switch {
		case err != nil:
			pc.Problem = err.Error()
		case c.NeedsCorrectiveAction && strings.TrimSpace(nr.CorrectiveAction) == "":
			pc.Check, pc.Problem = &c, needAction
		default:
			pc.Check = &c
		}
		out = append(out, pc)
	}
	return out
}

// saveRows saves rows of records all or none.
func (s *Service) saveRows(ctx context.Context, p *auth.Principal, rows []NewRecord, check func(store.Store, *NewRecord) error) ([]haccp.Record, error) {
	var saved []haccp.Record
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		for i, nr := range rows {
			if err := check(tx, &nr); err != nil {
				return rowErr(i, err)
			}
			rec, err := s.appendRecord(ctx, tx, p, nr)
			if err != nil {
				return rowErr(i, err)
			}
			saved = append(saved, rec)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, r := range saved {
		s.publish(events.KindRecord, r.ID)
	}
	return saved, nil
}

// rowErr names the row an error is about, keeping its kind.
func rowErr(i int, err error) error {
	var e *Error
	if errors.As(err, &e) {
		return &Error{Kind: e.Kind, NeedPassword: e.NeedPassword, Msg: fmt.Sprintf("row %d: %s", i+1, e.Msg)}
	}
	return fmt.Errorf("row %d: %w", i+1, err)
}

// ImportPaper saves the rows transcribed from a paper page, all or none.
// Each record keeps when the check was done, the name signed on the paper
// and the photo, and is logged by the person transcribing. Results are
// checked against the limits in force now.
func (s *Service) ImportPaper(ctx context.Context, p *auth.Principal, in PaperImport) ([]haccp.Record, error) {
	if err := s.allow(p, auth.ActInspect); err != nil {
		return nil, err
	}
	file, err := s.paperFile(ctx, in.File)
	if err != nil {
		return nil, err
	}
	if len(in.Rows) == 0 || len(in.Rows) > maxPaperRows {
		return nil, newErr(KindInvalid, "send 1 to %d rows", maxPaperRows)
	}
	return s.saveRows(ctx, p, in.Rows, func(tx store.Store, nr *NewRecord) error {
		_, err := s.checkPaperRow(ctx, tx, p, file, nr)
		return err
	})
}

// PaperDraft is a local model's reading of a photo of a paper page: rows
// for a person to check against the photo before saving.
type PaperDraft struct {
	Rows []NewRecord `json:"rows"`
	// Problems are the rows the model read that do not fit, by row, so a
	// person can fix them.
	Problems []string `json:"problems"`
}

// ReadPaper asks the local model to read the rows of a photo of a paper
// record book page. Nothing is saved: the rows are a draft.
func (s *Service) ReadPaper(ctx context.Context, p *auth.Principal, file string) (PaperDraft, error) {
	if err := s.allow(p, auth.ActInspect); err != nil {
		return PaperDraft{}, err
	}
	if !s.ai.Enabled() {
		return PaperDraft{}, newErr(KindInvalid, "%s", ai.ErrNotSetUp)
	}
	data, meta, err := s.readFile(ctx, file)
	if err != nil {
		return PaperDraft{}, err
	}
	if !strings.HasPrefix(meta.MIME, "image/") {
		return PaperDraft{}, newErr(KindInvalid, "the local model reads photos: upload the page as a photo")
	}
	lists, err := s.Lists().current(ctx, s.st)
	if err != nil {
		return PaperDraft{}, err
	}
	var units []string
	for _, u := range lists.Value.Units {
		units = append(units, fmt.Sprintf("%q (%s)", u.Name, u.Kind))
	}
	now := s.clk.Now().In(clock.Dublin)
	prompt := fmt.Sprintf(`This is a photo of a handwritten food safety record book page. Read every filled-in row.
Today is %s. The fridges and freezers are: %s.
Answer with JSON: {"rows": [{"type": "fridge|freezer|cook|reheat|hot_hold|delivery|transport", "date": "YYYY-MM-DD", "time": "HH:MM",
"unit": "fridge or freezer name", "food": "food cooked, reheated or held", "product": "delivered food", "supplier": "...", "customer": "...",
"delivery_kind": "chilled|frozen|ambient|hot", "temp_c": number, "signed_by": "name or initials as written", "action": "comments or action as written"}]}.
Leave out anything you cannot read rather than guessing. For a grid with AM and PM columns, give each reading its own row, with 09:00 for AM and 15:00 for PM.`,
		now.Format(haccp.DateLayout), strings.Join(units, ", "))
	var reply struct {
		Rows []struct {
			Type         string   `json:"type"`
			Date         string   `json:"date"`
			Time         string   `json:"time"`
			Unit         string   `json:"unit"`
			Food         string   `json:"food"`
			Product      string   `json:"product"`
			Supplier     string   `json:"supplier"`
			Customer     string   `json:"customer"`
			DeliveryKind string   `json:"delivery_kind"`
			TempC        *float64 `json:"temp_c"`
			SignedBy     string   `json:"signed_by"`
			Action       string   `json:"action"`
		} `json:"rows"`
	}
	if err := s.ai.JSON(ctx, "You transcribe food safety record books exactly as written and answer only with JSON.", prompt,
		[]ai.Image{{MIME: meta.MIME, Data: data}}, &reply); err != nil {
		return PaperDraft{}, newErr(KindInvalid, "the local model could not read the photo: %s", err)
	}
	out := PaperDraft{Rows: []NewRecord{}, Problems: []string{}}
	for i, r := range reply.Rows {
		if len(out.Rows) == maxPaperRows {
			break
		}
		nr := NewRecord{Type: haccp.RecordType(strings.TrimSpace(r.Type)), CorrectiveAction: strings.TrimSpace(r.Action), Reading: haccp.Reading{
			Unit: r.Unit, Food: r.Food, Product: r.Product, Supplier: r.Supplier, Customer: r.Customer,
			DeliveryKind: r.DeliveryKind, TempC: r.TempC, SignedBy: r.SignedBy, PaperFile: meta.SHA256,
		}}
		if t, err := time.ParseInLocation("2006-01-02 15:04", strings.TrimSpace(r.Date)+" "+strings.TrimSpace(r.Time), clock.Dublin); err == nil {
			nr.Reading.TakenAt = &t
		}
		var problem string
		switch {
		case !slices.Contains(PaperTypes, nr.Type):
			problem = fmt.Sprintf("row %d: %q is not a kind of reading Kishtin takes from paper", i+1, r.Type)
		case nr.Reading.TakenAt == nil:
			problem = fmt.Sprintf("row %d: the date or time could not be read", i+1)
		default:
			if err := nr.Reading.Normalize().Validate(nr.Type); err != nil {
				problem = fmt.Sprintf("row %d: %s", i+1, err)
			}
		}
		if problem != "" {
			out.Problems = append(out.Problems, problem)
		}
		out.Rows = append(out.Rows, nr)
	}
	return out, nil
}
