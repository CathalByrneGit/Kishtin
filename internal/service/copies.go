package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/ai"
	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/books"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/pdftext"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// readFile reads an uploaded file and its type.
func (s *Service) readFile(ctx context.Context, sha string) ([]byte, store.FileMeta, error) {
	sha = strings.ToLower(strings.TrimSpace(sha))
	if !files.ValidHash(sha) {
		return nil, store.FileMeta{}, newErr(KindInvalid, "give the SHA-256 of an uploaded file")
	}
	m, err := s.st.Files().GetFile(ctx, sha)
	if err != nil {
		return nil, m, mapStoreErr(err, "file")
	}
	f, err := s.files.Open(sha)
	if err != nil {
		return nil, m, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	return b, m, err
}

// CopyComparison is a booklet compared with a copy of the official PDF.
type CopyComparison struct {
	Booklet string `json:"booklet"`
	File    string `json:"file"`
	// Edition says whether the copy is the edition the booklet was made
	// from, byte for byte.
	Edition *books.CopyCheck `json:"edition,omitempty"`
	Report  books.CopyReport `json:"report"`
	// AI says whether a local model is set up to suggest columns.
	AI bool `json:"ai"`
}

// CompareCopy reads the text of an uploaded copy of a booklet's official
// PDF and says which of the booklet's columns and questions it has. It
// sends nothing anywhere.
func (s *Service) CompareCopy(ctx context.Context, p *auth.Principal, ref, file string) (CopyComparison, error) {
	if err := s.allow(p, auth.ActExportHACCP); err != nil {
		return CopyComparison{}, err
	}
	rb, err := s.recordBooks(ctx)
	if err != nil {
		return CopyComparison{}, err
	}
	bk, ok := books.Find(rb.Booklets, ref)
	if !ok {
		return CopyComparison{}, newErr(KindNotFound, "no record book called %q", ref)
	}
	data, meta, err := s.readFile(ctx, file)
	if err != nil {
		return CopyComparison{}, err
	}
	if meta.MIME != "application/pdf" {
		return CopyComparison{}, newErr(KindInvalid, "the copy must be a PDF")
	}
	pages, err := pdftext.Pages(data)
	if err != nil {
		return CopyComparison{}, newErr(KindInvalid, "Kishtin could not read the PDF's text: %s", err)
	}
	out := CopyComparison{Booklet: bk.Template.ID, File: meta.SHA256, Report: books.CompareCopy(bk.Template, pages), AI: s.ai.Enabled()}
	if src := bk.Template.Source; src != nil {
		out.Edition = &books.CopyCheck{File: meta.SHA256, Known: true, Matches: src.SHA256 == meta.SHA256}
	}
	if strings.TrimSpace(strings.Join(pages, "")) == "" {
		return out, newErr(KindInvalid, "the PDF has no text Kishtin can read: it may be a scan")
	}
	return out, nil
}

// Suggestion is a local model's reading of a copy: the columns and
// questions it found. It is a draft for a manager to review in Setup ›
// HACCP records; nothing is saved.
type Suggestion struct {
	Booklet   string                   `json:"booklet"`
	Columns   []books.Column           `json:"columns"`
	Checklist []books.ChecklistSection `json:"checklist,omitempty"`
	// Notes say what was changed to make the suggestion fit, such as a
	// field the model named that does not exist.
	Notes []string `json:"notes"`
}

// SuggestColumns asks the local model to read the column headings (and,
// for a checklist or training record, the questions or topics) from a copy
// of a booklet, and maps them to the fields Kishtin records.
func (s *Service) SuggestColumns(ctx context.Context, p *auth.Principal, ref, file string) (Suggestion, error) {
	if err := s.allow(p, auth.ActManageLimits); err != nil {
		return Suggestion{}, err
	}
	if !s.ai.Enabled() {
		return Suggestion{}, newErr(KindInvalid, "%s", ai.ErrNotSetUp)
	}
	_, err := s.CompareCopy(ctx, p, ref, file)
	if err != nil {
		return Suggestion{}, err
	}
	rb, err := s.recordBooks(ctx)
	if err != nil {
		return Suggestion{}, err
	}
	bk, _ := books.Find(rb.Booklets, ref)
	t := bk.Template
	var builder books.Builder
	for _, b := range books.Builders {
		if b.ID == t.Builder {
			builder = b
		}
	}
	if builder.ID == "" {
		return Suggestion{}, newErr(KindInvalid, "%s has no columns to suggest", t.Code)
	}
	data, _, err := s.readFile(ctx, file)
	if err != nil {
		return Suggestion{}, err
	}
	pages, err := pdftext.Pages(data)
	if err != nil {
		return Suggestion{}, err
	}
	text := strings.Join(pages, "\n")
	if len([]rune(text)) > 8000 {
		text = string([]rune(text)[:8000])
	}
	var fieldList, current []string
	for _, f := range builder.Fields {
		fieldList = append(fieldList, fmt.Sprintf("%q: %s", f.Key, f.Label))
	}
	for _, c := range t.Columns {
		current = append(current, fmt.Sprintf("%q (%s)", c.Label, c.Field))
	}
	prompt := fmt.Sprintf(`This is the text of a food safety record book page, %q. Table headings may be split over several lines.

---
%s
---

The booklet now has these columns: %s.
Each column can show one of these fields:
%s
or "" for a column Kishtin does not record (left blank to fill in by hand).

Answer with JSON: {"columns": [{"label": "...", "group": "...", "field": "..."}]}, listing the table's columns in order, with each label written as in the text and "group" for a heading over several columns.`,
		t.Code+" "+t.Title, text, strings.Join(current, ", "), strings.Join(fieldList, "\n"))
	if t.Builder == books.BuilderChecklist || t.Builder == books.BuilderTraining {
		prompt += ` Also list the questions or topics in sections: "checklist": [{"title": "...", "items": ["..."]}].`
	}
	var reply struct {
		Columns []struct {
			Label string `json:"label"`
			Group string `json:"group"`
			Field string `json:"field"`
		} `json:"columns"`
		Checklist []books.ChecklistSection `json:"checklist"`
	}
	if err := s.ai.JSON(ctx, "You read the tables of food safety record books and answer only with JSON.", prompt, nil, &reply); err != nil {
		return Suggestion{}, newErr(KindInvalid, "the local model could not answer: %s", err)
	}
	out := Suggestion{Booklet: t.ID, Columns: []books.Column{}, Notes: []string{}}
	known := map[string]bool{}
	for _, f := range builder.Fields {
		known[f.Key] = true
	}
	width := map[string]float64{}
	for _, c := range t.Columns {
		width[c.Field] = c.Width
	}
	for _, c := range reply.Columns {
		label := strings.TrimSpace(c.Label)
		if label == "" {
			continue
		}
		field := strings.TrimSpace(c.Field)
		if field != "" && !known[field] {
			out.Notes = append(out.Notes, fmt.Sprintf("%q: the model named a field Kishtin does not have (%q), so the column is left blank.", label, field))
			field = ""
		}
		w := width[field]
		if w <= 0 || field == "" {
			w = 20
		}
		out.Columns = append(out.Columns, books.Column{Label: label, Group: strings.TrimSpace(c.Group), Field: field, Width: w})
	}
	if t.Builder == books.BuilderChecklist || t.Builder == books.BuilderTraining {
		out.Checklist = reply.Checklist
	}
	draft := t
	draft.Columns, draft.Checklist = out.Columns, out.Checklist
	if t.Builder != books.BuilderChecklist && t.Builder != books.BuilderTraining {
		draft.Checklist = nil
	}
	if len(draft.Checklist) == 0 {
		draft.Checklist = t.Checklist
	}
	if err := draft.Validate(); err != nil {
		return out, newErr(KindInvalid, "the model's suggestion cannot be used as it is: %s. Change the columns by hand instead", err)
	}
	if !slices.ContainsFunc(out.Columns, func(c books.Column) bool { return c.Field == "ref" }) && slices.ContainsFunc(t.Columns, func(c books.Column) bool { return c.Field == "ref" }) {
		out.Notes = append(out.Notes, "Kishtin's record number column is not on the official page; add it back if you want it.")
	}
	return out, nil
}

// EditionTarget is an official booklet whose edition can be checked.
type EditionTarget struct {
	Booklet   string `json:"booklet"`
	Code      string `json:"code"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published string `json:"published"`
}

// EditionResult is what checking one booklet's edition found.
type EditionResult struct {
	EditionTarget
	// Status is same, changed or unreachable.
	Status string `json:"status"`
	// File is the SHA-256 of the downloaded PDF, kept when it changed so
	// it can be compared with the booklet.
	File  string `json:"file,omitempty"`
	Error string `json:"error,omitempty"`
}

// Edition statuses.
const (
	EditionSame        = "same"
	EditionChanged     = "changed"
	EditionUnreachable = "unreachable"
)

// EditionTargets lists the official booklets of the bundle in use and the
// addresses that checking their editions would download. Nothing is
// fetched.
func (s *Service) EditionTargets(ctx context.Context, p *auth.Principal) ([]EditionTarget, error) {
	if err := s.allow(p, auth.ActExportHACCP); err != nil {
		return nil, err
	}
	rb, err := s.recordBooks(ctx)
	if err != nil {
		return nil, err
	}
	out := []EditionTarget{}
	for _, b := range rb.Booklets {
		if src := b.Template.Source; src != nil && src.URL != "" && !b.Custom {
			out = append(out, EditionTarget{Booklet: b.Template.ID, Code: b.Template.Code, Title: b.Template.Title, URL: src.URL, Published: src.Published})
		}
	}
	return out, nil
}

// maxEditionSize is the largest official PDF downloaded.
const maxEditionSize = 30 << 20

func (s *Service) download(ctx context.Context, url string) ([]byte, error) {
	if s.fetch != nil {
		return s.fetch(ctx, url)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the site answered %s", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, maxEditionSize))
}

// CheckEditions downloads each official booklet of the bundle in use from
// its publisher and compares it with the edition the booklet was made
// from. It only downloads public files and sends nothing about the
// restaurant; a manager asks for it each time. A changed PDF is kept, so
// it can be compared with the booklet.
func (s *Service) CheckEditions(ctx context.Context, p *auth.Principal) ([]EditionResult, error) {
	if err := s.allow(p, auth.ActManageLimits); err != nil {
		return nil, err
	}
	targets, err := s.EditionTargets(ctx, p)
	if err != nil {
		return nil, err
	}
	rb, err := s.recordBooks(ctx)
	if err != nil {
		return nil, err
	}
	out := []EditionResult{}
	for _, t := range targets {
		r := EditionResult{EditionTarget: t}
		bk, _ := books.Find(rb.Booklets, t.Booklet)
		data, err := s.download(ctx, t.URL)
		if err != nil {
			r.Status, r.Error = EditionUnreachable, err.Error()
			out = append(out, r)
			continue
		}
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		if hash == bk.Template.Source.SHA256 {
			r.Status = EditionSame
			out = append(out, r)
			continue
		}
		r.Status = EditionChanged
		saved, err := s.files.Put(bytes.NewReader(data), maxEditionSize)
		if err != nil {
			r.Error = "downloaded, but not kept: " + err.Error()
			out = append(out, r)
			continue
		}
		m := store.FileMeta{SHA256: saved.SHA256, MIME: saved.MIME, Size: saved.Size,
			OriginalName: cleanName(t.Code + "-new-edition.pdf"), UploadedBy: p.User.ID, UploadedAt: s.clk.Now()}
		if err := s.st.Files().PutFile(ctx, m); err != nil {
			return nil, err
		}
		r.File = saved.SHA256
		out = append(out, r)
	}
	return out, nil
}
