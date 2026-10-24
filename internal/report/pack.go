// Package report renders the HACCP inspection pack as a PDF. It only
// formats data it is given; it reads nothing itself.
package report

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

// Pack is everything the inspection pack shows.
type Pack struct {
	RestaurantName string
	// From and To are the period covered; zero means open-ended.
	From, To    time.Time
	GeneratedAt time.Time
	GeneratedBy string
	Loc         *time.Location
	// Records are the records in the period, oldest first.
	Records []haccp.Record
	// Chain is the state of the whole chain when the pack was made.
	Chain        haccp.ChainReport
	FilesChecked int
	FileProblems int
	Guidance     string
	// Limits are the limit versions the records were checked against.
	Limits []haccp.LimitVersion
}

// Colours, as RGB.
var (
	ink      = [3]int{17, 17, 17}
	muted    = [3]int{90, 90, 90}
	rule     = [3]int{200, 200, 200}
	band     = [3]int{240, 240, 240}
	failBand = [3]int{252, 232, 234}
	warnBand = [3]int{253, 243, 224}
)

type writer struct {
	pdf *fpdf.Fpdf
	tr  func(string) string
	loc *time.Location
}

// clean keeps text to the characters the core PDF fonts can show: Latin-1,
// with a few common symbols spelled out. Irish fadas and the degree sign
// are kept.
func clean(s string) string {
	s = strings.NewReplacer("−", "-", "→", "->", "≤", "<=", "≥", ">=", "…", "...", "’", "'", "‘", "'", "“", "\"", "”", "\"", "–", "-", "—", "-").Replace(s)
	return strings.Map(func(r rune) rune {
		if r > 255 {
			return '?'
		}
		return r
	}, s)
}

func (w *writer) text(s string) string { return w.tr(clean(s)) }

func (w *writer) color(c [3]int) { w.pdf.SetTextColor(c[0], c[1], c[2]) }
func (w *writer) fill(c [3]int)  { w.pdf.SetFillColor(c[0], c[1], c[2]) }

func (w *writer) when(t time.Time) string { return t.In(w.loc).Format("2006-01-02 15:04") }

func (w *writer) heading(s string) {
	w.pdf.Ln(4)
	w.pdf.SetFont("Helvetica", "B", 13)
	w.color(ink)
	w.pdf.CellFormat(0, 8, w.text(s), "", 1, "L", false, 0, "")
}

func (w *writer) para(s string) {
	w.pdf.SetFont("Helvetica", "", 10)
	w.color(ink)
	w.pdf.MultiCell(0, 5, w.text(s), "", "L", false)
}

// row draws one table row, wrapping each cell, and starts a new page (with
// the header repeated) when it does not fit.
func (w *writer) row(widths []float64, cells []string, bold bool, bg *[3]int, header func()) {
	const lineH = 4.2
	style := ""
	if bold {
		style = "B"
	}
	w.pdf.SetFont("Helvetica", style, 8)
	lines := make([][]string, len(cells))
	maxLines := 1
	for i, c := range cells {
		lines[i] = w.pdf.SplitText(clean(c), widths[i]-2)
		if len(lines[i]) == 0 {
			lines[i] = []string{""}
		}
		maxLines = max(maxLines, len(lines[i]))
	}
	h := float64(maxLines)*lineH + 1.6
	_, pageH := w.pdf.GetPageSize()
	_, _, _, bottom := w.pdf.GetMargins()
	if w.pdf.GetY()+h > pageH-bottom-8 && header != nil {
		w.pdf.AddPage()
		header()
		w.pdf.SetFont("Helvetica", style, 8)
	}
	x, y := w.pdf.GetX(), w.pdf.GetY()
	total := 0.0
	for _, wd := range widths {
		total += wd
	}
	if bg != nil {
		w.fill(*bg)
		w.pdf.Rect(x, y, total, h, "F")
	}
	cx := x
	for i, wd := range widths {
		w.pdf.SetXY(cx+1, y+0.8)
		for _, l := range lines[i] {
			w.pdf.CellFormat(wd-2, lineH, w.tr(l), "", 2, "L", false, 0, "")
		}
		cx += wd
	}
	w.pdf.SetDrawColor(rule[0], rule[1], rule[2])
	w.pdf.Line(x, y+h, x+total, y+h)
	w.pdf.SetXY(x, y+h)
}

func (w *writer) reading(r haccp.Record) string {
	rd := r.Reading
	var parts []string
	if rd.TempC != nil {
		parts = append(parts, strconv.FormatFloat(*rd.TempC, 'f', 1, 64)+"°C")
	}
	if rd.HoldSeconds != nil && *rd.HoldSeconds > 0 {
		parts = append(parts, fmt.Sprintf("held %ds", *rd.HoldSeconds))
	}
	if rd.CookedAt != nil {
		s := "cooked " + rd.CookedAt.In(w.loc).Format("15:04")
		if rd.IntoFridgeAt != nil {
			s += ", fridge " + rd.IntoFridgeAt.In(w.loc).Format("15:04")
		}
		parts = append(parts, s)
	}
	if rd.TempAtCutoffC != nil {
		parts = append(parts, strconv.FormatFloat(*rd.TempAtCutoffC, 'f', 1, 64)+"°C at 2.5 h")
	}
	if rd.UseBy != "" {
		parts = append(parts, "use by "+rd.UseBy)
	}
	if rd.Done != nil {
		parts = append(parts, map[bool]string{true: "done", false: "not done"}[*rd.Done])
	}
	if rd.PackagingOK != nil && !*rd.PackagingOK {
		parts = append(parts, "packaging damaged")
	}
	if rd.InDate != nil && !*rd.InDate {
		parts = append(parts, "out of date")
	}
	return strings.Join(parts, "; ")
}

func subject(r haccp.Record) string {
	rd := r.Reading
	for _, s := range []string{rd.Unit, rd.Food, rd.Product, rd.Task} {
		if s != "" {
			if rd.Supplier != "" {
				return rd.Supplier + ": " + s
			}
			return s
		}
	}
	return ""
}

func outcomeStyle(o haccp.Outcome) (string, *[3]int) {
	switch o {
	case haccp.OutcomeFail:
		return "FAIL", &failBand
	case haccp.OutcomeBorderline:
		return "CHECK", &warnBand
	case haccp.OutcomePass:
		return "PASS", nil
	}
	return "RECORDED", nil
}

func limitLabel(lv haccp.LimitVersion) string {
	var parts []string
	f := func(p *float64) string { return strconv.FormatFloat(*p, 'f', 1, 64) + "°C" }
	if lv.MaxC != nil {
		parts = append(parts, "at most "+f(lv.MaxC))
	}
	if lv.BorderlineMaxC != nil {
		parts = append(parts, "borderline to "+f(lv.BorderlineMaxC))
	}
	if lv.MinC != nil {
		parts = append(parts, "at least "+f(lv.MinC))
	}
	for _, c := range lv.Combos {
		s := fmt.Sprintf("core %.1f°C", c.MinC)
		if c.HoldSeconds > 0 {
			s += fmt.Sprintf(" for %ds", c.HoldSeconds)
		}
		parts = append(parts, s)
	}
	if c := lv.Cooling; c != nil {
		parts = append(parts, fmt.Sprintf("into fridge within %d min; serve or discard if above %.1f°C at %d min",
			c.IntoFridgeWithinSeconds/60, c.CutoffMaxC, c.CutoffAfterSeconds/60))
	}
	return strings.Join(parts, " or ")
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// Write renders the pack as a PDF.
func Write(p Pack, out io.Writer) error {
	loc := p.Loc
	if loc == nil {
		loc = time.UTC
	}
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 14)
	pdf.SetTitle("HACCP inspection pack", true)
	pdf.SetCreator("Kishtin", true)
	pdf.AliasNbPages("{nb}")
	w := &writer{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor(""), loc: loc}

	period := "All records"
	switch {
	case !p.From.IsZero() && !p.To.IsZero():
		period = p.From.In(loc).Format("2 January 2006") + " to " + p.To.Add(-time.Second).In(loc).Format("2 January 2006")
	case !p.From.IsZero():
		period = "From " + p.From.In(loc).Format("2 January 2006")
	case !p.To.IsZero():
		period = "Up to " + p.To.Add(-time.Second).In(loc).Format("2 January 2006")
	}
	name := p.RestaurantName
	if name == "" {
		name = "Kishtin"
	}
	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("Helvetica", "", 7)
		w.color(muted)
		pdf.CellFormat(0, 4, w.text(fmt.Sprintf("%s · HACCP inspection pack · %s · page %d of {nb}", name, period, pdf.PageNo())), "", 0, "C", false, 0, "")
	})

	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 20)
	w.color(ink)
	pdf.CellFormat(0, 10, w.text("HACCP inspection pack"), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 12)
	pdf.CellFormat(0, 7, w.text(name+" · "+period), "", 1, "L", false, 0, "")
	pdf.Ln(2)

	failures := 0
	for _, r := range p.Records {
		if r.Outcome == haccp.OutcomeFail || r.Outcome == haccp.OutcomeBorderline {
			failures++
		}
	}
	chain := "OK"
	if !p.Chain.OK {
		chain = "BROKEN"
		if b := p.Chain.Break; b != nil {
			chain += fmt.Sprintf(" at record %d: %s", b.Seq, b.Problem)
		}
	}
	files := fmt.Sprintf("%d checked, all match their hashes", p.FilesChecked)
	if p.FileProblems > 0 {
		files = fmt.Sprintf("%d checked, %d do not match: run kishtin haccp verify", p.FilesChecked, p.FileProblems)
	}
	summary := [][]string{
		{"Records in this pack", strconv.Itoa(len(p.Records))},
		{"Failed or borderline", strconv.Itoa(failures)},
		{"Hash chain (all records)", fmt.Sprintf("%s · %d records · head %s", chain, p.Chain.Checked, p.Chain.HeadHash)},
		{"Attached files", files},
		{"Limits measured against", p.Guidance},
		{"Generated", w.when(p.GeneratedAt) + " by " + p.GeneratedBy},
	}
	for _, s := range summary {
		w.row([]float64{60, 213}, s, false, nil, nil)
	}
	pdf.Ln(3)
	w.para("Every record is append-only and hash-chained: each stores a SHA-256 of its content and of the record before it. " +
		"To check this pack against the live system, run \"kishtin haccp verify\" and compare the head hash above. " +
		"A mistake is fixed by a later record that corrects the original; both appear below.")

	cols := []float64{26, 20, 38, 34, 16, 85, 28, 26}
	header := func() {
		w.row(cols, []string{"Logged", "Type", "What", "Reading", "Result", "Reason and corrective action", "By", "Record / hash"}, true, &band, nil)
	}
	record := func(r haccp.Record) {
		res, bg := outcomeStyle(r.Outcome)
		detail := r.OutcomeReason
		if r.CorrectiveAction != "" {
			detail += "\nAction: " + r.CorrectiveAction
		}
		if r.Corrects != "" {
			detail += "\nCorrects " + r.Corrects + ": " + r.CorrectionReason
		}
		w.row(cols, []string{
			w.when(r.LoggedAt), strings.ReplaceAll(string(r.Type), "_", " "), subject(r), w.reading(r), res, detail,
			r.LoggedByName, fmt.Sprintf("#%d %s", r.Seq, shortHash(r.Hash)),
		}, false, bg, header)
	}

	w.heading("Failed and borderline records")
	if failures == 0 {
		w.para("None in this period.")
	} else {
		header()
		for _, r := range p.Records {
			if r.Outcome == haccp.OutcomeFail || r.Outcome == haccp.OutcomeBorderline {
				record(r)
			}
		}
	}

	pdf.AddPage()
	w.heading("All records")
	if len(p.Records) == 0 {
		w.para("No records in this period.")
	}
	day := ""
	for _, r := range p.Records {
		if d := r.LoggedAt.In(loc).Format("Monday 2 January 2006"); d != day {
			day = d
			pdf.SetFont("Helvetica", "B", 10)
			w.color(ink)
			pdf.Ln(2)
			pdf.CellFormat(0, 6, w.text(d), "", 1, "L", false, 0, "")
			header()
		}
		record(r)
	}

	w.heading("Limits used")
	w.para("The limit version each record above was checked against. Limits are versioned: a change adds a new version and never re-grades saved records.")
	lcols := []float64{40, 22, 30, 90, 91}
	lheader := func() {
		w.row(lcols, []string{"Version", "Check", "Applies to", "Limit", "Reason, plan reference and below FSAI"}, true, &band, nil)
	}
	lheader()
	limits := slices.Clone(p.Limits)
	slices.SortFunc(limits, func(a, b haccp.LimitVersion) int { return strings.Compare(string(a.Check)+a.ID, string(b.Check)+b.ID) })
	for _, lv := range limits {
		notes := lv.Reason
		if lv.PlanRef != "" {
			notes += " · plan: " + lv.PlanRef
		}
		if below := lv.LooserThanFSAI(); len(below) > 0 {
			notes += " · below FSAI: " + strings.Join(below, "; ")
		}
		w.row(lcols, []string{lv.ID, string(lv.Check), lv.Scope.String(), limitLabel(lv), notes}, false, nil, lheader)
	}

	if err := pdf.Error(); err != nil {
		return fmt.Errorf("render inspection pack: %w", err)
	}
	if err := pdf.Output(out); err != nil {
		return fmt.Errorf("write inspection pack: %w", err)
	}
	return nil
}
