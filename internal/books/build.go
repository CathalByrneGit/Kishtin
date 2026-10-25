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
	// Date is the day the row is about, as YYYY-MM-DD.
	Date string `json:"date,omitempty"`
	// Heading marks a section heading across the table, such as "Pest
	// Control". Its text is the first cell.
	Heading bool   `json:"heading,omitempty"`
	Cells   []Cell `json:"cells"`
}

// Page is one page of a booklet: its columns and rows. Long pages run on
// to the next sheet when printed.
type Page struct {
	// Day is set on a daily record: the day the page belongs to. Pages
	// of the same day print on one sheet.
	Day string `json:"day,omitempty"`
	// Sheet groups pages that print on one sheet (a day, an inspection, a
	// member of staff). Empty means the page is a sheet of its own.
	Sheet string `json:"sheet,omitempty"`
	// SheetLabel is the line under the booklet's title on that sheet, such
	// as "Date: 25/10/26" or "Name: Liam Cook".
	SheetLabel string `json:"sheet_label,omitempty"`
	// Title is a section heading, such as "Food Delivery Records" on a
	// daily record.
	Title   string   `json:"title,omitempty"`
	Columns []Column `json:"columns"`
	Rows    []Row    `json:"rows"`
	// Notes are a section's own notes, on a daily record.
	Notes []string `json:"notes,omitempty"`
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
	// Sections are the booklets a daily record is made of, as the
	// restaurant uses them, in the template's order.
	Sections []Template
	// Roles are staff members' roles by name, for a training record's
	// "Position".
	Roles map[string]string
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
		if !e.At.Before(b.to) {
			continue
		}
		b.upTo = append(b.upTo, e)
		if !e.At.Before(b.from) {
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
	case BuilderDelivery:
		b.deliveries()
	case BuilderTransport:
		b.transport()
	case BuilderFitness:
		b.fitness()
	case BuilderChecklist:
		b.checklist()
	case BuilderTraining:
		b.training()
	case BuilderDaily:
		if err := b.daily(); err != nil {
			return Book{}, err
		}
	}
	b.signOffs()
	return b.book, nil
}

type builder struct {
	in       Input
	from, to time.Time
	recs     []haccp.Effective
	// upTo are every record before the end of the month, for cumulative
	// booklets.
	upTo []haccp.Effective
	book Book
	seen map[string]bool
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
		row := Row{Date: dayStart.Format(haccp.DateLayout)}
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
		row := Row{Date: r.at.In(b.in.Loc).Format(haccp.DateLayout)}
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

// shortDay turns 2026-10-31 into 31/10/26, as the booklets write dates.
func shortDay(d string) string {
	t, err := time.Parse(haccp.DateLayout, d)
	if err != nil {
		return d
	}
	return t.Format("02/01/06")
}

func (b *builder) deliveries() {
	var rows []*logRow
	// A failed check shows only its flag, which prints as FAIL.
	vehicle := func(p *bool) Cell {
		switch {
		case p == nil:
			return Cell{}
		case *p:
			return Cell{Text: "OK"}
		}
		return Cell{Flag: FlagFail}
	}
	for _, e := range b.recs {
		if e.Type != haccp.RecordDelivery {
			continue
		}
		rd := e.Reading
		r := b.newRow(e.At, rd.Product)
		r.values["batch_code"] = Cell{Text: rd.BatchCode}
		r.values["supplier"] = Cell{Text: rd.Supplier}
		if rd.UseBy != "" {
			r.values["use_by"] = Cell{Text: shortDay(rd.UseBy)}
		}
		t := Cell{Text: temp(rd.TempC)}
		if rd.TempC == nil {
			t.Text = "ambient"
		}
		// The temperature is out when the result's reason says so; a
		// delivery can also fail on packaging, date or vehicle alone.
		if f := flag(e.Outcome); f != "" && strings.Contains(e.OutcomeReason, "°C is above") {
			t.Flag = f
		}
		r.values["temp"] = t
		r.values["vehicle"] = vehicle(rd.VehicleOK)
		r.values["delivery_kind"] = Cell{Text: rd.DeliveryKind}
		b.add(r, e, "")
		rows = append(rows, r)
	}
	b.emit(rows)
}

// daily builds a sheet per day from the section booklets: every day of
// the month from when record keeping started up to today.
func (b *builder) daily() error {
	type section struct {
		tpl  Template
		book Book
	}
	var secs []section
	unfilled := map[string]bool{}
	for _, t := range b.in.Sections {
		in := b.in
		in.Template, in.Sections = t, nil
		sub, err := Build(in)
		if err != nil {
			return fmt.Errorf("section %s: %w", t.Code, err)
		}
		secs = append(secs, section{t, sub})
		b.book.Gaps = append(b.book.Gaps, sub.Gaps...)
		for _, u := range sub.Unfilled {
			if !unfilled[u] {
				unfilled[u] = true
				b.book.Unfilled = append(b.book.Unfilled, u)
			}
		}
		b.book.Records += sub.Records
		b.book.Corrected += sub.Corrected
		if sub.FirstSeq != 0 && (b.book.FirstSeq == 0 || sub.FirstSeq < b.book.FirstSeq) {
			b.book.FirstSeq = sub.FirstSeq
		}
		b.book.LastSeq = max(b.book.LastSeq, sub.LastSeq)
	}
	slices.SortStableFunc(b.book.Gaps, func(x, y Gap) int { return strings.Compare(x.Date, y.Date) })
	if b.in.Since.IsZero() {
		return nil
	}
	first := b.in.Since.In(b.in.Loc).Format(haccp.DateLayout)
	today := b.in.Now.In(b.in.Loc).Format(haccp.DateLayout)
	for d := b.from; d.Before(b.to); d = d.AddDate(0, 0, 1) {
		day := d.Format(haccp.DateLayout)
		if day < first || day > today {
			continue
		}
		for _, s := range secs {
			for _, pg := range s.book.Pages {
				// The sheet carries the date, so the sections drop their
				// date column.
				var keep []int
				var cols []Column
				for i, c := range pg.Columns {
					if c.Field != "date" {
						keep = append(keep, i)
						cols = append(cols, c)
					}
				}
				out := Page{Day: day, Sheet: day, SheetLabel: "Date: " + shortDay(day), Title: s.tpl.Title, Columns: cols, Rows: []Row{}, Notes: s.tpl.Notes}
				for _, r := range pg.Rows {
					if r.Date != day {
						continue
					}
					row := Row{Date: day}
					for _, i := range keep {
						row.Cells = append(row.Cells, r.Cells[i])
					}
					out.Rows = append(out.Rows, row)
				}
				b.book.Pages = append(b.book.Pages, out)
			}
		}
	}
	return nil
}

// onDate is the day a training or fitness record is about: its on_date, or
// the day it was logged.
func (b *builder) onDate(e haccp.Effective) string {
	if e.Reading.OnDate != "" {
		return e.Reading.OnDate
	}
	return e.At.In(b.in.Loc).Format(haccp.DateLayout)
}

func yesNo(p *bool) Cell {
	switch {
	case p == nil:
		return Cell{}
	case *p:
		return Cell{Text: "Yes"}
	}
	return Cell{Text: "No", Flag: FlagFail}
}

func (b *builder) transport() {
	var rows []*logRow
	for _, e := range b.recs {
		if e.Type != haccp.RecordTransport {
			continue
		}
		rd := e.Reading
		r := b.newRow(e.At, rd.Product)
		bu := rd.BatchCode
		if rd.UseBy != "" {
			if bu != "" {
				bu += " / "
			}
			bu += shortDay(rd.UseBy)
		}
		r.values["batch_use_by"] = Cell{Text: bu}
		r.values["customer"] = Cell{Text: rd.Customer}
		r.values["separated"] = yesNo(rd.Separated)
		t := Cell{Text: temp(rd.TempC)}
		if rd.TempC == nil {
			t.Text = "ambient"
		}
		if f := flag(e.Outcome); f != "" && strings.Contains(e.OutcomeReason, "°C is") {
			t.Flag = f
		}
		r.values["temp"] = t
		b.add(r, e, "")
		rows = append(rows, r)
	}
	b.emit(rows)
}

var fitnessReasons = map[string]string{
	haccp.FitnessExisting:      "Existing food handler",
	haccp.FitnessPreEmployment: "Pre-employment",
	haccp.FitnessReturn:        "Return to work after illness",
}

func (b *builder) fitness() {
	var rows []*logRow
	for _, e := range b.recs {
		if e.Type != haccp.RecordFitness {
			continue
		}
		rd := e.Reading
		day := b.onDate(e)
		r := b.newRow(e.At, "")
		r.values["date"] = Cell{Text: shortDay(day)}
		r.values["staff"] = Cell{Text: rd.Staff}
		r.values["reason"] = Cell{Text: fitnessReasons[rd.FitnessReason]}
		fit := Cell{Text: "Fit"}
		if rd.Fit != nil && !*rd.Fit {
			fit = Cell{Text: "Not fit", Flag: FlagFail}
		}
		r.values["fit"] = fit
		r.values["action"] = Cell{Text: e.CorrectiveAction}
		r.values["manager"] = Cell{Text: e.LoggedByName}
		b.add(r, e, "")
		rows = append(rows, r)
	}
	b.emit(rows)
}

// checklist prints each inspection as it was answered: the questions are
// stored in the record, so changing the checklist later does not change
// past inspections.
func (b *builder) checklist() {
	t := b.in.Template
	for _, e := range b.recs {
		if e.Type != haccp.RecordInspection {
			continue
		}
		b.use(e)
		day := e.At.In(b.in.Loc).Format(haccp.DateLayout)
		page := Page{
			Day: day, Sheet: e.ID,
			SheetLabel: fmt.Sprintf("Date: %s · checked by %s · Kishtin ref #%d", shortDay(day), e.LoggedByName, e.Seq),
			Columns:    t.Columns, Rows: []Row{},
		}
		section := ""
		for _, a := range e.Reading.Answers {
			if a.Section != "" && a.Section != section {
				section = a.Section
				page.Rows = append(page.Rows, Row{Date: day, Heading: true, Cells: []Cell{{Text: section}}})
			}
			values := map[string]Cell{"item": {Text: a.Item}, "action": {Text: a.Action}}
			if a.OK != nil && *a.OK {
				values["yes"] = Cell{Text: "Yes"}
			}
			if a.OK != nil && !*a.OK {
				values["no"] = Cell{Text: "No", Flag: FlagFail}
			}
			row := Row{Date: day}
			for _, c := range t.Columns {
				row.Cells = append(row.Cells, values[c.Field])
			}
			page.Rows = append(page.Rows, row)
		}
		if e.Reading.Notes != "" {
			page.Notes = []string{e.Reading.Notes}
		}
		b.book.Pages = append(b.book.Pages, page)
	}
}

// training prints a sheet per member of staff with every training record
// up to the end of the month: the template's topics with the dates and
// trainers, then further training.
func (b *builder) training() {
	t := b.in.Template
	type person struct {
		name    string
		inHouse map[string][]haccp.Effective
		other   []string
		further []haccp.Effective
	}
	var people []*person
	byName := map[string]*person{}
	listed := map[string]bool{}
	for _, sec := range t.Checklist {
		for _, it := range sec.Items {
			listed[strings.ToLower(it)] = true
		}
	}
	for _, e := range b.upTo {
		if e.Type != haccp.RecordTraining {
			continue
		}
		key := strings.ToLower(e.Reading.Staff)
		p := byName[key]
		if p == nil {
			p = &person{name: e.Reading.Staff, inHouse: map[string][]haccp.Effective{}}
			byName[key] = p
			people = append(people, p)
		}
		b.use(e)
		if e.Reading.TrainingKind == haccp.TrainingFurther {
			p.further = append(p.further, e)
			continue
		}
		for _, topic := range e.Reading.Topics {
			k := strings.ToLower(topic)
			if !listed[k] && len(p.inHouse[k]) == 0 {
				p.other = append(p.other, topic)
			}
			p.inHouse[k] = append(p.inHouse[k], e)
		}
	}
	slices.SortFunc(people, func(x, y *person) int { return strings.Compare(strings.ToLower(x.name), strings.ToLower(y.name)) })
	for _, p := range people {
		label := "Name: " + p.name
		if role := b.in.Roles[p.name]; role != "" {
			label += " · Position: " + role
		}
		topicRow := func(topic string) Row {
			var dates, trainers []string
			for _, e := range p.inHouse[strings.ToLower(topic)] {
				dates = addUnique(dates, shortDay(b.onDate(e)))
				trainers = addUnique(trainers, e.Reading.Trainer)
			}
			values := map[string]Cell{"topic": {Text: topic}, "dates": {Text: strings.Join(dates, ", ")}, "trainer": {Text: strings.Join(trainers, ", ")}}
			row := Row{}
			for _, c := range t.Columns {
				row.Cells = append(row.Cells, values[c.Field])
			}
			return row
		}
		page := Page{Sheet: "staff:" + strings.ToLower(p.name), SheetLabel: label, Title: "In-house training", Columns: t.Columns, Rows: []Row{}}
		for _, sec := range t.Checklist {
			if sec.Title != "" {
				page.Rows = append(page.Rows, Row{Heading: true, Cells: []Cell{{Text: sec.Title}}})
			}
			for _, it := range sec.Items {
				page.Rows = append(page.Rows, topicRow(it))
			}
		}
		if len(p.other) > 0 {
			page.Rows = append(page.Rows, Row{Heading: true, Cells: []Cell{{Text: "Other"}}})
			for _, it := range p.other {
				page.Rows = append(page.Rows, topicRow(it))
			}
		}
		b.book.Pages = append(b.book.Pages, page)
		if len(t.FurtherColumns) == 0 {
			continue
		}
		further := Page{Sheet: page.Sheet, SheetLabel: label, Title: "Further training", Columns: t.FurtherColumns, Rows: []Row{}}
		for _, e := range p.further {
			values := map[string]Cell{
				"nature": {Text: strings.Join(e.Reading.Topics, ", ")}, "provider": {Text: e.Reading.Provider},
				"completed": {Text: shortDay(b.onDate(e))},
			}
			row := Row{Date: b.onDate(e)}
			for _, c := range t.FurtherColumns {
				row.Cells = append(row.Cells, values[c.Field])
			}
			further.Rows = append(further.Rows, row)
		}
		b.book.Pages = append(b.book.Pages, further)
	}
}
