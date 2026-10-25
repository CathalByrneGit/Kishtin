// Package books fills in HACCP record books, such as the FSAI Safe
// Catering booklets, from Kishtin's records.
//
// A bundle is one country's (or one issuer's) set of record books. Each
// book is a template: data that says which columns a booklet has and which
// record field goes in each. Developers add a bundle when a country needs
// one; a new edition with renamed or reordered columns is new data, not
// new code. A restaurant then picks its bundle and changes what is
// relevant to it (see RecordBookSettings). The package does no I/O beyond reading
// its built-in bundles.
package books

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// Builders say how a booklet's rows are worked out from records.
const (
	// BuilderUnitGrid is a month grid: a row per day, and a column per
	// fridge or freezer check (SC2).
	BuilderUnitGrid = "unit_grid"
	// BuilderCooking is a row per cooking, with its cooling, and a row
	// per reheat (SC3).
	BuilderCooking = "cooking"
	// BuilderHotHold is a row per food held hot, with up to three checks
	// (SC4).
	BuilderHotHold = "hot_hold"
	// BuilderDelivery is a row per delivery (SC1).
	BuilderDelivery = "delivery"
	// BuilderDaily is a sheet per day made of other booklets of the same
	// bundle, one section each (SC8). It has sections instead of columns.
	BuilderDaily = "daily"
	// BuilderChecklist is a sheet per hygiene inspection, with its
	// questions and answers (SC5). The questions come from the template.
	BuilderChecklist = "checklist"
	// BuilderTraining is a sheet per member of staff with the training
	// they have had, up to the end of the month (SC6). The topics come
	// from the template.
	BuilderTraining = "training"
	// BuilderFitness is a row per fitness to work assessment (SC7).
	BuilderFitness = "fitness"
	// BuilderTransport is a row per delivery to a customer (SC9).
	BuilderTransport = "transport"
)

// Field is a record field a builder can put in a column.
type Field struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Builder is a kind of booklet row, with the fields it can show.
type Builder struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Fields []Field `json:"fields"`
}

// Builders lists every builder, for setting up a booklet.
var Builders = []Builder{
	{ID: BuilderUnitGrid, Label: "Fridge and freezer grid: a row per day, a column per check", Fields: []Field{
		{"date", "Day of the month"},
		{"unit_checks", "Readings: a column for each check on each fridge and freezer"},
		{"comments", "Comments and actions"},
		{"signed", "Who logged"},
	}},
	{ID: BuilderCooking, Label: "Cooking, cooling and reheating: a row per cooking or reheat", Fields: []Field{
		{"date", "Date"},
		{"food", "Food"},
		{"cook_finished", "Time cooking finished"},
		{"core_temp", "Core temperature after cooking"},
		{"cook_signed", "Who logged the cooking"},
		{"into_fridge", "Time into the fridge or blast chiller"},
		{"cool_signed", "Who logged the cooling"},
		{"reheat_temp", "Core temperature after reheating"},
		{"reheat_signed", "Who logged the reheat"},
		{"comments", "Comments and actions"},
		{"ref", "Kishtin record number"},
	}},
	{ID: BuilderDelivery, Label: "Deliveries: a row per delivery", Fields: []Field{
		{"date", "Date"},
		{"food", "Food item"},
		{"batch_code", "Batch code"},
		{"supplier", "Supplied by"},
		{"use_by", "Use-by date"},
		{"temp", "Temperature (or ambient)"},
		{"vehicle", "Delivery vehicle check"},
		{"delivery_kind", "Chilled, frozen or ambient"},
		{"comments", "Comments and actions"},
		{"signed", "Who logged"},
		{"ref", "Kishtin record number"},
	}},
	{ID: BuilderTransport, Label: "Deliveries to customers: a row per delivery", Fields: []Field{
		{"date", "Date"},
		{"food", "Food item"},
		{"batch_use_by", "Batch code and use-by date"},
		{"customer", "Customer"},
		{"separated", "Raw and ready-to-eat food kept apart"},
		{"temp", "Temperature (or ambient)"},
		{"comments", "Comments and actions"},
		{"signed", "Who logged"},
		{"ref", "Kishtin record number"},
	}},
	{ID: BuilderChecklist, Label: "Inspection checklist: a sheet per inspection, with your questions", Fields: []Field{
		{"item", "Question"},
		{"yes", "Satisfactory: yes"},
		{"no", "Satisfactory: no"},
		{"action", "Details of action taken"},
	}},
	{ID: BuilderTraining, Label: "Training record: a sheet per member of staff, with your topics", Fields: []Field{
		{"topic", "Topic"},
		{"dates", "Dates trained"},
		{"trainer", "Trainer"},
		{"nature", "Further training: what it was"},
		{"provider", "Further training: course provider"},
		{"completed", "Further training: date completed"},
		{"signature", "Employee signature (blank, to sign by hand)"},
	}},
	{ID: BuilderFitness, Label: "Fitness to work: a row per assessment, outcome only", Fields: []Field{
		{"date", "Date of assessment"},
		{"staff", "Employee"},
		{"reason", "Reason for assessment"},
		{"fit", "Fit to work"},
		{"action", "Action taken"},
		{"manager", "Owner or manager"},
		{"ref", "Kishtin record number"},
	}},
	{ID: BuilderHotHold, Label: "Hot holding: a row per food per day, up to three checks", Fields: []Field{
		{"date", "Date"},
		{"food", "Food"},
		{"check_1", "1st check: temperature and time"},
		{"check_2", "2nd check: temperature and time"},
		{"check_3", "3rd check: temperature and time"},
		{"comments", "Comments and actions"},
		{"signed", "Who logged"},
		{"ref", "Kishtin record number"},
	}},
}

func findBuilder(id string) (Builder, bool) {
	for _, b := range Builders {
		if b.ID == id {
			return b, true
		}
	}
	return Builder{}, false
}

// PageWidth is the printable width of a landscape A4 page, in mm. Column
// widths are scaled to fill it.
const PageWidth = 273.0

// Bundle is a set of record books from one country or issuer.
type Bundle struct {
	ID string `json:"id"`
	// Name is the bundle's name, such as "FSAI Safe Catering record books".
	Name string `json:"name"`
	// Country is who the bundle is for, in words: "Ireland", "Any country".
	Country string `json:"country"`
	Issuer  string `json:"issuer"`
	// Jurisdiction is the ISO 3166 country code, or empty for any.
	Jurisdiction string `json:"jurisdiction,omitempty"`
	// Language is the language of the booklets, as a BCP 47 tag.
	Language  string     `json:"language"`
	URL       string     `json:"url,omitempty"`
	Templates []Template `json:"templates"`
}

// Source is the official booklet a template copies.
type Source struct {
	URL string `json:"url"`
	// Published is the edition, as YYYY-MM.
	Published string `json:"published"`
	// SHA256 is the hash of the PDF the template was made from, so a new
	// edition can be spotted.
	SHA256 string `json:"sha256"`
}

// Column is one column of a booklet.
type Column struct {
	Label string `json:"label"`
	// Group is a heading over several columns, such as "Cooking".
	Group string `json:"group,omitempty"`
	// Field is the record field shown. Empty means Kishtin does not record
	// what this column asks for, and it prints blank unless Fixed is set.
	Field string `json:"field,omitempty"`
	// Fixed is text printed in every row, when the row has a value in
	// FixedWhen (or always, when FixedWhen is empty).
	Fixed     string `json:"fixed,omitempty"`
	FixedWhen string `json:"fixed_when,omitempty"`
	// Width is the column's share of the page width. Widths are scaled
	// so the columns fill a landscape A4 page.
	Width float64 `json:"width"`
}

// Template is one record book.
type Template struct {
	ID    string `json:"id"`
	Code  string `json:"code"`
	Title string `json:"title"`
	// Source is the official booklet this copies, if there is one.
	Source *Source `json:"source,omitempty"`
	// Builder is empty when Kishtin cannot fill this booklet yet, and
	// Missing says why.
	Builder string `json:"builder,omitempty"`
	Missing string `json:"missing,omitempty"`
	// UnitsPerPage is how many fridges and freezers fit across one page of
	// a unit grid.
	UnitsPerPage int      `json:"units_per_page,omitempty"`
	Columns      []Column `json:"columns,omitempty"`
	// Notes are printed under the table.
	Notes []string `json:"notes,omitempty"`
	// ManagerCheck adds a manager or supervisor check line.
	ManagerCheck bool `json:"manager_check,omitempty"`
	// Sections are the IDs of the booklets a daily record is made of, in
	// order. Only for the daily builder.
	Sections []string `json:"sections,omitempty"`
	// Checklist is the questions of an inspection checklist, or the
	// topics of a training record, in sections.
	Checklist []ChecklistSection `json:"checklist,omitempty"`
	// FurtherColumns are a training record's second table, for further
	// training such as external courses.
	FurtherColumns []Column `json:"further_columns,omitempty"`
}

// ChecklistSection is a heading and its questions or topics.
type ChecklistSection struct {
	Title string   `json:"title,omitempty"`
	Items []string `json:"items"`
}

// Supported reports whether Kishtin can fill this booklet.
func (t Template) Supported() bool { return t.Builder != "" }

// Unfilled lists the columns Kishtin does not record, by label.
func (t Template) Unfilled() []string {
	var out []string
	for _, c := range t.Columns {
		if c.Field == "" && c.Fixed == "" {
			out = append(out, c.Label)
		}
	}
	return out
}

// MaxColumns is the most columns a booklet can have.
const MaxColumns = 16

// Validate checks a template's columns against its builder.
func (t Template) Validate() error {
	if t.ID == "" || strings.TrimSpace(t.Code) == "" || strings.TrimSpace(t.Title) == "" {
		return errors.New("a record book needs an id, code and title")
	}
	if len([]rune(t.Code)) > 12 || len([]rune(t.Title)) > 120 {
		return fmt.Errorf("%s: the code is at most 12 characters and the title at most 120", t.Code)
	}
	if t.Builder == "" {
		if t.Missing == "" {
			return fmt.Errorf("%s: say why Kishtin cannot fill it (missing)", t.Code)
		}
		return nil
	}
	if t.Builder == BuilderDaily {
		if len(t.Sections) == 0 {
			return fmt.Errorf("%s: a daily record needs its sections", t.Code)
		}
		if len(t.Columns) > 0 {
			return fmt.Errorf("%s: a daily record takes its columns from its sections", t.Code)
		}
		return nil
	}
	if len(t.Sections) > 0 {
		return fmt.Errorf("%s: only a daily record has sections", t.Code)
	}
	b, ok := findBuilder(t.Builder)
	if !ok {
		return fmt.Errorf("%s: unknown builder %q", t.Code, t.Builder)
	}
	if len(t.Columns) == 0 || len(t.Columns) > MaxColumns {
		return fmt.Errorf("%s: a booklet has 1 to %d columns", t.Code, MaxColumns)
	}
	known := func(f string) bool {
		return slices.ContainsFunc(b.Fields, func(x Field) bool { return x.Key == f })
	}
	grid := 0
	for _, c := range t.Columns {
		if strings.TrimSpace(c.Label) == "" {
			return fmt.Errorf("%s: every column needs a label", t.Code)
		}
		if c.Width <= 0 {
			return fmt.Errorf("%s: column %q needs a width", t.Code, c.Label)
		}
		for _, f := range []string{c.Field, c.FixedWhen} {
			if f != "" && !known(f) {
				var names []string
				for _, x := range b.Fields {
					names = append(names, x.Key)
				}
				return fmt.Errorf("%s: column %q uses field %q, which this kind of booklet does not have (it has %s)",
					t.Code, c.Label, f, strings.Join(names, ", "))
			}
		}
		if c.Field == "unit_checks" {
			grid++
		}
	}
	for _, c := range t.FurtherColumns {
		if strings.TrimSpace(c.Label) == "" || c.Width <= 0 {
			return fmt.Errorf("%s: every further training column needs a label and a width", t.Code)
		}
		if c.Field != "" && !known(c.Field) {
			return fmt.Errorf("%s: further training column %q uses field %q, which this kind of booklet does not have", t.Code, c.Label, c.Field)
		}
	}
	if t.Builder == BuilderChecklist || t.Builder == BuilderTraining {
		n := 0
		for _, sec := range t.Checklist {
			for _, it := range sec.Items {
				if strings.TrimSpace(it) == "" {
					return fmt.Errorf("%s: a question or topic is empty", t.Code)
				}
				n++
			}
		}
		if n == 0 && t.Builder == BuilderChecklist {
			return fmt.Errorf("%s: an inspection checklist needs its questions", t.Code)
		}
	} else if len(t.Checklist) > 0 {
		return fmt.Errorf("%s: only an inspection checklist or training record has a checklist", t.Code)
	}
	if t.Builder == BuilderUnitGrid {
		if t.UnitsPerPage < 1 || t.UnitsPerPage > 12 {
			return fmt.Errorf("%s: fridges and freezers per sheet must be 1 to 12", t.Code)
		}
		if grid != 1 {
			return fmt.Errorf("%s: a fridge and freezer grid needs exactly one readings column (unit_checks)", t.Code)
		}
	}
	return nil
}

//go:embed bundles/*.json
var bundleFiles embed.FS

// Builtin returns the bundles that ship with Kishtin.
func Builtin() ([]Bundle, error) {
	names, err := fs.Glob(bundleFiles, "bundles/*.json")
	if err != nil {
		return nil, err
	}
	var out []Bundle
	for _, n := range names {
		b, err := bundleFiles.ReadFile(n)
		if err != nil {
			return nil, err
		}
		p, err := ParseBundle(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		out = append(out, p)
	}
	return out, nil
}

// ParseBundle reads and checks a bundle.
func ParseBundle(b []byte) (Bundle, error) {
	var p Bundle
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, fmt.Errorf("read record book bundle: %w", err)
	}
	if p.ID == "" || p.Name == "" || p.Country == "" {
		return p, errors.New("a record book bundle needs an id, a name and a country")
	}
	seen := map[string]bool{}
	for _, t := range p.Templates {
		if err := t.Validate(); err != nil {
			return p, err
		}
		if t.Source != nil && len(t.Source.SHA256) != 64 {
			return p, fmt.Errorf("%s: the source needs the SHA-256 of the official PDF", t.Code)
		}
		if seen[t.ID] || seen["code:"+strings.ToLower(t.Code)] {
			return p, fmt.Errorf("record book %s appears twice", t.Code)
		}
		seen[t.ID], seen["code:"+strings.ToLower(t.Code)] = true, true
	}
	for _, t := range p.Templates {
		for _, id := range t.Sections {
			sec, ok := findTemplate(p.Templates, id)
			if !ok || !sec.Supported() || sec.Builder == BuilderDaily {
				return p, fmt.Errorf("%s: section %q must be another booklet in the bundle that Kishtin can fill", t.Code, id)
			}
		}
	}
	return p, nil
}

// FindBundle returns a bundle by ID.
func FindBundle(bundles []Bundle, id string) (Bundle, bool) {
	for _, b := range bundles {
		if b.ID == id {
			return b, true
		}
	}
	return Bundle{}, false
}

func findTemplate(list []Template, id string) (Template, bool) {
	for _, t := range list {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

// Cumulative reports whether a booklet shows everything up to the end of
// the month, not only the month's records (a training record).
func (t Template) Cumulative() bool { return t.Builder == BuilderTraining }
