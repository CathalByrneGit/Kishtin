// Package books fills in official HACCP record books, such as the FSAI
// Safe Catering pack, from Kishtin's records. A record book is a template:
// data that says which columns a booklet has and which record field goes
// in each. Templates for other countries, or a new edition of a booklet,
// are new data, not new code. The package does no I/O beyond reading its
// built-in templates.
package books

import (
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
)

// fields lists the record fields each builder can put in a column.
var fields = map[string][]string{
	BuilderUnitGrid: {"date", "unit_checks", "comments", "signed"},
	BuilderCooking: {
		"date", "food", "cook_finished", "core_temp", "cook_signed", "into_fridge",
		"cool_signed", "reheat_temp", "reheat_signed", "comments", "ref",
	},
	BuilderHotHold: {"date", "food", "check_1", "check_2", "check_3", "comments", "signed", "ref"},
}

// Pack is a set of record books from one issuer.
type Pack struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Issuer       string     `json:"issuer"`
	Jurisdiction string     `json:"jurisdiction"`
	URL          string     `json:"url"`
	Templates    []Template `json:"templates"`
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
	// Width is in millimetres on a landscape A4 page.
	Width float64 `json:"width"`
}

// Template is one record book.
type Template struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Title  string `json:"title"`
	Source Source `json:"source"`
	// Builder is empty when Kishtin cannot fill this booklet yet, and
	// Missing says why.
	Builder string `json:"builder,omitempty"`
	Missing string `json:"missing,omitempty"`
	// UnitsPerPage is how many fridges and freezers fit across one page of
	// a unit grid.
	UnitsPerPage int      `json:"units_per_page,omitempty"`
	Columns      []Column `json:"columns,omitempty"`
	// Notes are printed under the table, as the booklet prints them.
	Notes []string `json:"notes,omitempty"`
	// ManagerCheck adds the booklet's manager or supervisor check line.
	ManagerCheck bool `json:"manager_check,omitempty"`
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

// Validate checks a template's columns against its builder.
func (t Template) Validate() error {
	if t.ID == "" || t.Code == "" || t.Title == "" {
		return errors.New("a record book needs an id, code and title")
	}
	if t.Builder == "" {
		if t.Missing == "" {
			return fmt.Errorf("%s: say why Kishtin cannot fill it (missing)", t.ID)
		}
		return nil
	}
	known, ok := fields[t.Builder]
	if !ok {
		return fmt.Errorf("%s: unknown builder %q", t.ID, t.Builder)
	}
	if len(t.Columns) == 0 {
		return fmt.Errorf("%s: no columns", t.ID)
	}
	for _, c := range t.Columns {
		if c.Width <= 0 {
			return fmt.Errorf("%s: column %q needs a width", t.ID, c.Label)
		}
		for _, f := range []string{c.Field, c.FixedWhen} {
			if f != "" && !slices.Contains(known, f) {
				return fmt.Errorf("%s: column %q uses field %q, which the %s builder does not have (it has %s)",
					t.ID, c.Label, f, t.Builder, strings.Join(known, ", "))
			}
		}
	}
	if t.Builder == BuilderUnitGrid {
		if t.UnitsPerPage < 1 {
			return fmt.Errorf("%s: units_per_page must be at least 1", t.ID)
		}
		if !slices.ContainsFunc(t.Columns, func(c Column) bool { return c.Field == "unit_checks" }) {
			return fmt.Errorf("%s: a unit grid needs a unit_checks column", t.ID)
		}
	}
	return nil
}

//go:embed packs/*.json
var packFiles embed.FS

// Builtin returns the record book packs that ship with Kishtin.
func Builtin() ([]Pack, error) {
	names, err := fs.Glob(packFiles, "packs/*.json")
	if err != nil {
		return nil, err
	}
	var out []Pack
	for _, n := range names {
		b, err := packFiles.ReadFile(n)
		if err != nil {
			return nil, err
		}
		p, err := ParsePack(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		out = append(out, p)
	}
	return out, nil
}

// ParsePack reads and checks a pack.
func ParsePack(b []byte) (Pack, error) {
	var p Pack
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, fmt.Errorf("read record book pack: %w", err)
	}
	if p.ID == "" || p.Name == "" {
		return p, errors.New("a record book pack needs an id and a name")
	}
	seen := map[string]bool{}
	for _, t := range p.Templates {
		if err := t.Validate(); err != nil {
			return p, err
		}
		if seen[t.ID] {
			return p, fmt.Errorf("record book %s appears twice", t.ID)
		}
		seen[t.ID] = true
	}
	return p, nil
}

// Find returns a template by ID or code, and the pack it is in. Codes are
// matched without regard to case ("sc2").
func Find(packs []Pack, ref string) (Template, Pack, bool) {
	for _, p := range packs {
		for _, t := range p.Templates {
			if t.ID == ref || strings.EqualFold(t.Code, ref) {
				return t, p, true
			}
		}
	}
	return Template{}, Pack{}, false
}
