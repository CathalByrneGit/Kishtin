package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

// LabelSize is a label a batch label can print on.
type LabelSize struct {
	ID string `json:"id"`
	// Name says what it is for.
	Name string `json:"name"`
	// W and H are the label's width and height in millimetres.
	W float64 `json:"w_mm"`
	H float64 `json:"h_mm"`
}

// LabelSizes are the sizes offered. The first is the default. "a4" prints
// one label at the top of an A4 page, with a line to cut along, for a
// kitchen with an ordinary printer.
var LabelSizes = []LabelSize{
	{ID: "62x29", Name: "Label printer, 62 × 29 mm", W: 62, H: 29},
	{ID: "100x50", Name: "Label printer, 100 × 50 mm", W: 100, H: 50},
	{ID: "a4", Name: "A4 page, to cut out", W: 100, H: 50},
}

// FindLabelSize returns the size with an ID; the default for "".
func FindLabelSize(id string) (LabelSize, bool) {
	if id == "" {
		return LabelSizes[0], true
	}
	for _, s := range LabelSizes {
		if s.ID == id {
			return s, true
		}
	}
	return LabelSize{}, false
}

// Label is a batch label to print.
type Label struct {
	Record haccp.Record
	Size   LabelSize
	Loc    *time.Location
}

func labelDate(day string) string {
	t, err := time.Parse(haccp.DateLayout, day)
	if err != nil {
		return day
	}
	return t.Format("Mon 2 Jan 2006")
}

// WriteLabel renders a batch label record as a PDF the size of the label:
// the product, when it was made, its use-by date in large type, who made
// it and the record's number, so the tub can be traced back to the record.
func WriteLabel(l Label, out io.Writer) error {
	r := l.Record
	if r.Type != haccp.RecordBatchLabel {
		return fmt.Errorf("record %d is a %s record, not a batch label", r.Seq, r.Type)
	}
	loc := l.Loc
	if loc == nil {
		loc = time.UTC
	}
	sz := l.Size
	a4 := sz.ID == "a4"
	var pdf *fpdf.Fpdf
	x0, y0 := 0.0, 0.0
	if a4 {
		pdf = fpdf.New("P", "mm", "A4", "")
		x0, y0 = 15, 15
	} else {
		// fpdf takes a page's size as if portrait and turns it for landscape.
		pdf = fpdf.NewCustom(&fpdf.InitType{OrientationStr: "L", UnitStr: "mm", Size: fpdf.SizeType{Wd: min(sz.W, sz.H), Ht: max(sz.W, sz.H)}})
	}
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetTitle(clean("Batch label: "+r.Reading.Product), false)
	pdf.SetCreator("Kishtin", false)
	pdf.AddPage()
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	text := func(s string) string { return tr(clean(s)) }

	if a4 {
		pdf.SetDrawColor(120, 120, 120)
		pdf.SetDashPattern([]float64{1.5, 1.5}, 0)
		pdf.Rect(x0, y0, sz.W, sz.H, "D")
		pdf.SetDashPattern(nil, 0)
	}
	// Type scales with the label's height.
	k := sz.H / 29
	pad := 2.0 * k
	w := sz.W - 2*pad
	pdf.SetTextColor(0, 0, 0)

	// The product, shrunk to fit one line.
	size := 13 * k
	pdf.SetFont("Helvetica", "B", size)
	name := text(strings.TrimSpace(r.Reading.Product))
	for size > 6 && pdf.GetStringWidth(name) > w {
		size -= 0.5
		pdf.SetFont("Helvetica", "B", size)
	}
	pdf.SetXY(x0+pad, y0+pad)
	pdf.CellFormat(w, 6*k, name, "", 0, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 7.5*k)
	pdf.SetXY(x0+pad, y0+pad+6.5*k)
	pdf.CellFormat(w, 3.5*k, text("Made "+labelDate(r.Reading.MadeOn)), "", 0, "L", false, 0, "")

	pdf.SetFont("Helvetica", "B", 7.5*k)
	pdf.SetXY(x0+pad, y0+pad+11*k)
	pdf.CellFormat(w, 3.5*k, "USE BY", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", 15*k)
	pdf.SetXY(x0+pad, y0+pad+14*k)
	pdf.CellFormat(w, 7*k, text(labelDate(r.Reading.UseBy)), "", 0, "L", false, 0, "")

	by := r.Reading.SignedBy
	if by == "" {
		by = r.LoggedByName
	}
	pdf.SetFont("Helvetica", "", 6*k)
	pdf.SetXY(x0+pad, y0+sz.H-pad-2.6*k)
	pdf.CellFormat(w, 2.6*k, text(fmt.Sprintf("%s · record %d · %s", by, r.Seq, r.TakenAt().In(loc).Format("15:04"))), "", 0, "L", false, 0, "")

	if err := pdf.Output(out); err != nil {
		return fmt.Errorf("write label: %w", err)
	}
	return nil
}
