package report

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/cathalbyrnegit/kishtin/internal/books"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

// Book is a filled-in record book and what its pages say about it.
type Book struct {
	Book           books.Book
	RestaurantName string
	GeneratedAt    time.Time
	GeneratedBy    string
	Loc            *time.Location
	// Chain is the state of the whole chain when the book was made.
	Chain haccp.ChainReport
}

var missedBand = [3]int{228, 228, 228}

// flagged is how a flagged cell prints: the flag as a word, and a tint.
func flagged(c books.Cell) (string, *[3]int) {
	switch c.Flag {
	case books.FlagFail:
		return c.Text + " FAIL", &failBand
	case books.FlagBorderline:
		return c.Text + " CHECK", &warnBand
	case books.FlagMissed:
		return "missed", &missedBand
	}
	return c.Text, nil
}

// gridRow draws one boxed row, wrapping each cell, and starts a new page
// (with the header repeated) when it does not fit.
func (w *writer) gridRow(widths []float64, cells []string, fills []*[3]int, bold bool, size float64, header func()) {
	const lineH = 3.2
	style := ""
	if bold {
		style = "B"
	}
	w.pdf.SetFont("Helvetica", style, size)
	lines := make([][]string, len(cells))
	maxLines := 1
	for i, c := range cells {
		for _, part := range strings.Split(clean(c), "\n") {
			lines[i] = append(lines[i], w.pdf.SplitText(part, widths[i]-1.6)...)
		}
		maxLines = max(maxLines, len(lines[i]))
	}
	h := float64(maxLines)*lineH + 1.1
	_, pageH := w.pdf.GetPageSize()
	_, _, _, bottom := w.pdf.GetMargins()
	if w.pdf.GetY()+h > pageH-bottom-8 && header != nil {
		w.pdf.AddPage()
		header()
		w.pdf.SetFont("Helvetica", style, size)
	}
	x, y := w.pdf.GetX(), w.pdf.GetY()
	w.pdf.SetDrawColor(120, 120, 120)
	cx := x
	for i, wd := range widths {
		mode := "D"
		if fills != nil && fills[i] != nil {
			w.fill(*fills[i])
			mode = "FD"
		}
		w.pdf.Rect(cx, y, wd, h, mode)
		w.pdf.SetXY(cx+0.8, y+0.55)
		for _, l := range lines[i] {
			w.pdf.CellFormat(wd-1.6, lineH, w.tr(l), "", 2, "L", false, 0, "")
		}
		cx += wd
	}
	w.pdf.SetXY(x, y+h)
}

// groupRow draws the headings over groups of columns, such as a unit name
// over its AM and PM checks. Columns with no group get an empty box.
func (w *writer) groupRow(cols []books.Column) {
	var widths []float64
	var cells []string
	for i := 0; i < len(cols); {
		j, wd := i, 0.0
		for j < len(cols) && cols[j].Group == cols[i].Group && (cols[i].Group != "" || j == i) {
			wd += cols[j].Width
			j++
		}
		widths = append(widths, wd)
		cells = append(cells, cols[i].Group)
		i = j
	}
	fills := make([]*[3]int, len(cells))
	for i := range fills {
		fills[i] = &band
	}
	w.gridRow(widths, cells, fills, true, 7.5, nil)
}

func monthName(month string) string {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return month
	}
	return t.Format("January 2006")
}

// WriteBook renders a filled-in record book as a PDF.
func WriteBook(p Book, out io.Writer) error {
	b := p.Book
	tpl := b.Template
	loc := p.Loc
	if loc == nil {
		loc = time.UTC
	}
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 14)
	pdf.SetTitle(tpl.Code+" - "+tpl.Title, true)
	pdf.SetCreator("Kishtin", true)
	pdf.AliasNbPages("{nb}")
	w := &writer{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor(""), loc: loc}

	name := p.RestaurantName
	if name == "" {
		name = "Kishtin"
	}
	month := monthName(b.Month)
	chain := "hash chain OK"
	if !p.Chain.OK {
		chain = "hash chain BROKEN: run kishtin haccp verify"
	}
	refs := "no records"
	if b.Records > 0 {
		refs = fmt.Sprintf("%d records, #%d to #%d", b.Records, b.FirstSeq, b.LastSeq)
	}
	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("Helvetica", "", 7)
		w.color(muted)
		pdf.CellFormat(0, 4, w.text(fmt.Sprintf("%s · %s · %s · filled by Kishtin from %s · %s, head %s · page %d of {nb}",
			name, tpl.Code, month, refs, chain, shortHash(p.Chain.HeadHash), pdf.PageNo())), "", 0, "C", false, 0, "")
	})

	for i, page := range b.Pages {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "B", 14)
		w.color(ink)
		pdf.CellFormat(0, 6.5, w.text(tpl.Code+" - "+tpl.Title), "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		head := name + " · Month: " + month
		if len(b.Pages) > 1 {
			head += fmt.Sprintf(" · sheet %d of %d", i+1, len(b.Pages))
		}
		pdf.CellFormat(0, 5, w.text(head), "", 1, "L", false, 0, "")
		pdf.Ln(1)

		widths := make([]float64, len(page.Columns))
		labels := make([]string, len(page.Columns))
		grouped := false
		for k, c := range page.Columns {
			widths[k], labels[k] = c.Width, c.Label
			grouped = grouped || c.Group != ""
		}
		header := func() {
			if grouped {
				w.groupRow(page.Columns)
			}
			fills := make([]*[3]int, len(labels))
			for k := range fills {
				fills[k] = &band
			}
			w.gridRow(widths, labels, fills, true, 7.5, nil)
		}
		header()
		for _, row := range page.Rows {
			cells := make([]string, len(row.Cells))
			fills := make([]*[3]int, len(row.Cells))
			for k, c := range row.Cells {
				cells[k], fills[k] = flagged(c)
			}
			w.gridRow(widths, cells, fills, false, 7.5, header)
		}
		if len(page.Rows) == 0 {
			w.para("No records this month.")
		}

		pdf.Ln(1.5)
		pdf.SetFont("Helvetica", "", 7.5)
		w.color(muted)
		for _, n := range tpl.Notes {
			pdf.MultiCell(0, 3.6, w.text(n), "", "L", false)
		}
		if tpl.ManagerCheck {
			pdf.Ln(1)
			pdf.SetFont("Helvetica", "", 9)
			w.color(ink)
			pdf.CellFormat(0, 5.5, w.text("Manager/Supervisor check on ____ / ____ / ________      Signed ______________________"), "", 1, "L", false, 0, "")
		}
	}

	pdf.Ln(1)
	pdf.SetFont("Helvetica", "", 7)
	w.color(ink)
	var about []string
	if len(b.Gaps) > 0 {
		about = append(about, fmt.Sprintf("%d checks were due and have no reading. They are marked \"missed\".", len(b.Gaps)))
	}
	if len(b.Unfilled) > 0 {
		about = append(about, "Kishtin does not record: "+strings.Join(b.Unfilled, ", ")+". These boxes are left blank.")
	}
	if b.Corrected > 0 {
		about = append(about, fmt.Sprintf("%d entries show a correction in place of the original record; the comments say why. Both records are kept.", b.Corrected))
	}
	verify := "Run \"kishtin haccp verify\" to check the records have not changed."
	if slices.ContainsFunc(tpl.Columns, func(c books.Column) bool { return c.Field == "ref" }) {
		verify = "Kishtin ref is the record number. " + verify
	}
	about = append(about, fmt.Sprintf("Layout from %s, %s %s edition, filled in by Kishtin on %s for %s. FAIL and CHECK mark readings outside the restaurant's limits. %s",
		b.PackName, tpl.Code, tpl.Source.Published, w.when(p.GeneratedAt), p.GeneratedBy, verify))
	for _, a := range about {
		pdf.MultiCell(0, 3.2, w.text(a), "", "L", false)
	}

	if err := pdf.Error(); err != nil {
		return fmt.Errorf("render %s: %w", tpl.Code, err)
	}
	if err := pdf.Output(out); err != nil {
		return fmt.Errorf("write %s: %w", tpl.Code, err)
	}
	return nil
}
