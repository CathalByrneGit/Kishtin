package books

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

// Cell flags. A flagged cell is shown with its flag as a word as well as a
// colour.
const (
	FlagFail       = "fail"
	FlagBorderline = "borderline"
	// FlagMissed is a check that was due and has no reading.
	FlagMissed = "missed"
)

// Cell is one filled-in box.
type Cell struct {
	Text string `json:"text"`
	Flag string `json:"flag,omitempty"`
}

// Row is one line of a booklet page.
type Row struct {
	Cells []Cell `json:"cells"`
}

// Page is one page of a booklet: its columns and rows. Long pages run on
// to the next sheet when printed.
type Page struct {
	Columns []Column `json:"columns"`
	Rows    []Row    `json:"rows"`
}

// Gap is a check that was due and has no reading.
type Gap struct {
	Date  string `json:"date"`
	Unit  string `json:"unit"`
	Check string `json:"check"`
}

// Book is a filled-in record book for one month.
type Book struct {
	// Template is the booklet as the restaurant uses it.
	Template   Template `json:"template"`
	BundleName string   `json:"bundle_name"`
	Issuer     string   `json:"issuer"`
	// Changed lists what the restaurant changed from the bundle's booklet.
	Changed []string `json:"changed"`
	// SettingsVersion is the record book settings version used.
	SettingsVersion string `json:"settings_version,omitempty"`
	// Month is YYYY-MM.
	Month string `json:"month"`
	Pages []Page `json:"pages"`
	// Gaps are checks that were due and have no reading.
	Gaps []Gap `json:"gaps"`
	// Unfilled are the columns Kishtin does not record, by label.
	Unfilled []string `json:"unfilled"`
	// Records is how many records the book shows, after corrections.
	Records int `json:"records"`
	// Corrected is how many of them are corrections of an earlier record.
	Corrected int `json:"corrected"`
	// SignOffs are the manager sign-offs whose period overlaps the month.
	SignOffs []SignOff `json:"sign_offs"`
	// Unsigned are the days of the month, up to yesterday, that no
	// sign-off covers.
	Unsigned []Span `json:"unsigned"`
	// FirstSeq and LastSeq are the lowest and highest record numbers shown.
	FirstSeq int64 `json:"first_seq,omitempty"`
	LastSeq  int64 `json:"last_seq,omitempty"`
}

// Input is what Build works from.
type Input struct {
	Template Template
	Bundle   Bundle
	Year     int
	Month    time.Month
	Loc      *time.Location
	Now      time.Time
	// Since is when record keeping started (the first record). No check
	// before it counts as missed.
	Since time.Time
	// Records are the records logged from the start of the month on,
	// including later corrections, in any order.
	Records []haccp.Record
	// Units are the fridges and freezers on the managed list.
	Units []haccp.Unit
}

// Build fills in a record book for a month.
func Build(in Input) (Book, error) {
	t := in.Template
	if !t.Supported() {
		return Book{}, fmt.Errorf("cannot fill %s yet: %s", t.Code, t.Missing)
	}
	if err := t.Validate(); err != nil {
		return Book{}, err
	}
	if in.Loc == nil {
		in.Loc = time.UTC
	}
	b := &builder{in: in, from: time.Date(in.Year, in.Month, 1, 0, 0, 0, 0, in.Loc)}
	b.to = b.from.AddDate(0, 1, 0)
	b.book = Book{
		Template: t, BundleName: in.Bundle.Name, Issuer: in.Bundle.Issuer, Changed: []string{},
		Month: b.from.Format("2006-01"), Pages: []Page{}, Gaps: []Gap{}, Unfilled: t.Unfilled(),
		SignOffs: []SignOff{}, Unsigned: []Span{},
	}
	if b.book.Unfilled == nil {
		b.book.Unfilled = []string{}
	}
	recs := slices.Clone(in.Records)
	slices.SortFunc(recs, func(a, b haccp.Record) int { return int(a.Seq - b.Seq) })
	for _, e := range haccp.ApplyCorrections(recs) {
		if !e.At.Before(b.from) && e.At.Before(b.to) {
			b.recs = append(b.recs, e)
		}
	}
	switch t.Builder {
	case BuilderUnitGrid:
		b.unitGrid()
	case BuilderCooking:
		b.cooking()
	case BuilderHotHold:
		b.hotHold()
	}
	b.signOffs()
	return b.book, nil
}

type builder struct {
	in       Input
	from, to time.Time
	recs     []haccp.Effective
	book     Book
	seen     map[string]bool
}

// use counts a record as shown in the book.
func (b *builder) use(e haccp.Effective) {
	if b.seen == nil {
		b.seen = map[string]bool{}
	}
	if b.seen[e.ID] {
		return
	}
	b.seen[e.ID] = true
	b.book.Records++
	if e.Corrects != "" {
		b.book.Corrected++
	}
	if b.book.FirstSeq == 0 || e.Seq < b.book.FirstSeq {
		b.book.FirstSeq = e.Seq
	}
	b.book.LastSeq = max(b.book.LastSeq, e.Seq)
}

func (b *builder) clock(t time.Time) string { return t.In(b.in.Loc).Format("15:04") }
func (b *builder) date(t time.Time) string  { return t.In(b.in.Loc).Format("02/01/06") }

func temp(c *float64) string {
	if c == nil {
		return ""
	}
	return strconv.FormatFloat(*c, 'f', 1, 64) + "°C"
}

func flag(o haccp.Outcome) string {
	switch o {
	case haccp.OutcomeFail:
		return FlagFail
	case haccp.OutcomeBorderline:
		return FlagBorderline
	}
	return ""
}

// firstName is how a booklet's "Signed" box shows who logged a record.
func firstName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return name
}

// note is the comment a record adds: why it failed and what was done, or
// why it was corrected.
func note(prefix string, e haccp.Effective) string {
	var parts []string
	if e.Outcome == haccp.OutcomeFail || e.Outcome == haccp.OutcomeBorderline {
		s := strings.TrimRight(e.OutcomeReason, ". ")
		if e.CorrectiveAction != "" {
			s += ". Action: " + e.CorrectiveAction
		}
		parts = append(parts, s)
	}
	if e.Corrects != "" {
		s := "Corrected"
		if e.CorrectionReason != "" {
			s += ": " + e.CorrectionReason
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return ""
	}
	if prefix != "" {
		prefix += ": "
	}
	return prefix + strings.Join(parts, ". ")
}

func addUnique(list []string, s string) []string {
	if s == "" || slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// --- Unit grid (SC2) ---

type gridUnit struct {
	unit   haccp.Unit
	listed bool
	checks []haccp.CheckTime
}

func ordinal(d int) string {
	suffix := "th"
	if d%100 < 11 || d%100 > 13 {
		switch d % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(d) + suffix
}

func (b *builder) unitGrid() {
	t := b.in.Template
	var units []gridUnit
	key := func(kind haccp.RecordType, name string) string { return string(kind) + "|" + strings.ToLower(name) }
	known := map[string]bool{}
	for _, u := range b.in.Units {
		units = append(units, gridUnit{unit: u, listed: true, checks: u.ChecksOrDefault()})
		known[key(u.Kind, u.Name)] = true
	}
	var extra []gridUnit
	for _, e := range b.recs {
		if e.Type != haccp.RecordFridge && e.Type != haccp.RecordFreezer {
			continue
		}
		if k := key(e.Type, e.Reading.Unit); !known[k] {
			known[k] = true
			extra = append(extra, gridUnit{unit: haccp.Unit{Name: e.Reading.Unit, Kind: e.Type}, checks: haccp.DefaultChecks()})
		}
	}
	slices.SortFunc(extra, func(a, b gridUnit) int {
		return strings.Compare(strings.ToLower(a.unit.Name), strings.ToLower(b.unit.Name))
	})
	units = append(units, extra...)

	if len(units) == 0 {
		b.book.Pages = append(b.book.Pages, b.gridPage(nil))
		return
	}
	for i := 0; i < len(units); i += t.UnitsPerPage {
		b.book.Pages = append(b.book.Pages, b.gridPage(units[i:min(i+t.UnitsPerPage, len(units))]))
	}
}

// slotOf finds which of a unit's checks a time of day falls in.
func slotOf(checks []haccp.CheckTime, t time.Time) int {
	m := t.Hour()*60 + t.Minute()
	idx := 0
	for i, c := range checks {
		if from, err := haccp.ParseClock(c.From); err == nil && from <= m {
			idx = i
		}
	}
	return idx
}

func (b *builder) gridPage(units []gridUnit) Page {
	t := b.in.Template
	loc := b.in.Loc
	slots := 0
	for _, u := range units {
		slots += len(u.checks)
	}
	// Columns: unit_checks becomes one column per unit check. Space the
	// page's checks as if the page were full, and give what is left to
	// the comments.
	var spare float64
	var cols []Column
	for _, c := range t.Columns {
		if c.Field != "unit_checks" {
			cols = append(cols, c)
			continue
		}
		per := c.Width / float64(max(slots, t.UnitsPerPage*2))
		per = math.Floor(per*10) / 10
		spare = c.Width - per*float64(slots)
		for _, u := range units {
			for _, ch := range u.checks {
				cols = append(cols, Column{Label: ch.Label, Group: u.unit.Name, Field: "unit_checks", Width: per})
			}
		}
	}
	for i := range cols {
		if cols[i].Field == "comments" {
			cols[i].Width += spare
			spare = 0
		}
	}

	type slotKey struct{ unit, day, slot int }
	latest := map[slotKey]haccp.Effective{}
	comments := map[int][]string{}
	signed := map[int][]string{}
	for _, e := range b.recs {
		for ui, u := range units {
			if e.Type != u.unit.Kind || !strings.EqualFold(e.Reading.Unit, u.unit.Name) {
				continue
			}
			at := e.At.In(loc)
			k := slotKey{ui, at.Day(), slotOf(u.checks, at)}
			if prev, ok := latest[k]; !ok || !e.At.Before(prev.At) {
				latest[k] = e
			}
			b.use(e)
			if n := note(u.unit.Name+" "+u.checks[k.slot].Label+" "+temp(e.Reading.TempC), e); n != "" {
				comments[k.day] = append(comments[k.day], n)
			}
			signed[k.day] = addUnique(signed[k.day], firstName(e.LoggedByName))
		}
	}

	days := b.to.AddDate(0, 0, -1).Day()
	page := Page{Columns: cols, Rows: []Row{}}
	for d := 1; d <= days; d++ {
		dayStart := time.Date(b.from.Year(), b.from.Month(), d, 0, 0, 0, 0, loc)
		var row Row
		for _, c := range t.Columns {
			switch c.Field {
			case "date":
				row.Cells = append(row.Cells, Cell{Text: ordinal(d)})
			case "comments":
				row.Cells = append(row.Cells, Cell{Text: strings.Join(comments[d], "\n")})
			case "signed":
				row.Cells = append(row.Cells, Cell{Text: strings.Join(signed[d], ", ")})
			case "unit_checks":
				for ui, u := range units {
					for si, ch := range u.checks {
						if e, ok := latest[slotKey{ui, d, si}]; ok {
							row.Cells = append(row.Cells, Cell{Text: temp(e.Reading.TempC), Flag: flag(e.Outcome)})
							continue
						}
						if u.listed && b.missed(dayStart, u.checks, si) {
							row.Cells = append(row.Cells, Cell{Text: "missed", Flag: FlagMissed})
							b.book.Gaps = append(b.book.Gaps, Gap{Date: dayStart.Format(haccp.DateLayout), Unit: u.unit.Name, Check: ch.Label})
							continue
						}
						row.Cells = append(row.Cells, Cell{})
					}
				}
			default:
				row.Cells = append(row.Cells, b.fixed(c, nil))
			}
		}
		page.Rows = append(page.Rows, row)
	}
	return page
}

// missed reports whether a check on a day is over with no reading: its
// time has passed (the next check has started, or the day has ended), and
// it ended after record keeping started.
func (b *builder) missed(dayStart time.Time, checks []haccp.CheckTime, i int) bool {
	end := dayStart.AddDate(0, 0, 1)
	if i+1 < len(checks) {
		if m, err := haccp.ParseClock(checks[i+1].From); err == nil {
			end = dayStart.Add(time.Duration(m) * time.Minute)
		}
	}
	if end.After(b.in.Now) {
		return false
	}
	return b.in.Since.IsZero() || end.After(b.in.Since)
}

// fixed fills a column with no field: its fixed text, when the row has a
// value in FixedWhen.
func (b *builder) fixed(c Column, values map[string]Cell) Cell {
	if c.Fixed == "" {
		return Cell{}
	}
	if c.FixedWhen != "" && values[c.FixedWhen].Text == "" {
		return Cell{}
	}
	return Cell{Text: c.Fixed}
}

// --- Log builders (SC3, SC4) ---

// logRow is a row of a log booklet, as field values.
type logRow struct {
	at     time.Time
	values map[string]Cell
	notes  []string
	signed []string
	refs   []string
}

func (b *builder) newRow(at time.Time, food string) *logRow {
	return &logRow{at: at, values: map[string]Cell{"date": {Text: b.date(at)}, "food": {Text: food}}}
}

func (b *builder) add(r *logRow, e haccp.Effective, prefix string) {
	b.use(e)
	r.refs = append(r.refs, "#"+strconv.FormatInt(e.Seq, 10))
	if n := note(prefix, e); n != "" {
		r.notes = append(r.notes, n)
	}
	r.signed = addUnique(r.signed, firstName(e.LoggedByName))
}

func (b *builder) emit(rows []*logRow) {
	slices.SortStableFunc(rows, func(x, y *logRow) int { return x.at.Compare(y.at) })
	page := Page{Columns: b.in.Template.Columns, Rows: []Row{}}
	for _, r := range rows {
		r.values["comments"] = Cell{Text: strings.Join(r.notes, "\n")}
		r.values["ref"] = Cell{Text: strings.Join(r.refs, ", ")}
		if _, ok := r.values["signed"]; !ok {
			r.values["signed"] = Cell{Text: strings.Join(r.signed, ", ")}
		}
		var row Row
		for _, c := range page.Columns {
			if c.Field != "" {
				row.Cells = append(row.Cells, r.values[c.Field])
			} else {
				row.Cells = append(row.Cells, b.fixed(c, r.values))
			}
		}
		page.Rows = append(page.Rows, row)
	}
	b.book.Pages = append(b.book.Pages, page)
}

func coreTemp(r haccp.Reading) string {
	s := temp(r.TempC)
	if r.HoldSeconds != nil && *r.HoldSeconds > 0 {
		if *r.HoldSeconds%60 == 0 {
			s += fmt.Sprintf(" for %d min", *r.HoldSeconds/60)
		} else {
			s += fmt.Sprintf(" for %d s", *r.HoldSeconds)
		}
	}
	return s
}

// coolMatch is how far apart a cooking record and a cooling record's
// "cooked at" time can be and still be the same batch.
const coolMatch = time.Hour

func (b *builder) cooking() {
	var rows []*logRow
	var cooks []*logRow
	cooled := map[*logRow]bool{}
	for _, e := range b.recs {
		if e.Type != haccp.RecordCook {
			continue
		}
		r := b.newRow(e.At, e.Reading.Food)
		r.values["cook_finished"] = Cell{Text: b.clock(e.At)}
		r.values["core_temp"] = Cell{Text: coreTemp(e.Reading), Flag: flag(e.Outcome)}
		r.values["cook_signed"] = Cell{Text: firstName(e.LoggedByName)}
		b.add(r, e, "")
		rows = append(rows, r)
		cooks = append(cooks, r)
	}
	for _, e := range b.recs {
		if e.Type != haccp.RecordCooling || e.Reading.CookedAt == nil {
			continue
		}
		cookedAt := *e.Reading.CookedAt
		var match *logRow
		for _, c := range cooks {
			if cooled[c] || !strings.EqualFold(c.values["food"].Text, e.Reading.Food) {
				continue
			}
			d := c.at.Sub(cookedAt).Abs()
			if d <= coolMatch && (match == nil || d < match.at.Sub(cookedAt).Abs()) {
				match = c
			}
		}
		if match == nil {
			match = b.newRow(cookedAt, e.Reading.Food)
			match.values["cook_finished"] = Cell{Text: b.clock(cookedAt)}
			rows = append(rows, match)
		}
		cooled[match] = true
		into := Cell{Flag: flag(e.Outcome)}
		if e.Reading.IntoFridgeAt != nil {
			into.Text = b.clock(*e.Reading.IntoFridgeAt)
		} else if into.Flag != "" {
			into.Text = "not in fridge"
		}
		if e.Reading.TempAtCutoffC != nil {
			if into.Text != "" {
				into.Text += ", "
			}
			into.Text += temp(e.Reading.TempAtCutoffC) + " at 2.5 h"
		}
		match.values["into_fridge"] = into
		match.values["cool_signed"] = Cell{Text: firstName(e.LoggedByName)}
		b.add(match, e, "Cooling")
	}
	for _, e := range b.recs {
		if e.Type != haccp.RecordReheat {
			continue
		}
		r := b.newRow(e.At, e.Reading.Food)
		r.values["reheat_temp"] = Cell{Text: coreTemp(e.Reading), Flag: flag(e.Outcome)}
		r.values["reheat_signed"] = Cell{Text: firstName(e.LoggedByName)}
		b.add(r, e, "")
		rows = append(rows, r)
	}
	b.emit(rows)
}

func (b *builder) hotHold() {
	var rows []*logRow
	open := map[string]*logRow{}
	count := map[*logRow]int{}
	for _, e := range b.recs {
		if e.Type != haccp.RecordHotHold {
			continue
		}
		k := b.date(e.At) + "|" + strings.ToLower(e.Reading.Food)
		r := open[k]
		if r == nil || count[r] == 3 {
			r = b.newRow(e.At, e.Reading.Food)
			open[k] = r
			rows = append(rows, r)
		}
		count[r]++
		n := count[r]
		r.values["check_"+strconv.Itoa(n)] = Cell{Text: temp(e.Reading.TempC) + " at " + b.clock(e.At), Flag: flag(e.Outcome)}
		b.add(r, e, ordinal(n)+" check")
	}
	b.emit(rows)
}
