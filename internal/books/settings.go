package books

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// BuiltinSettingsID is the version ID of the default record book settings.
const BuiltinSettingsID = "builtin-record-books-v1"

// DefaultBundle is the bundle a new install uses.
const DefaultBundle = "fsai-safe-catering"

// CustomPrefix starts the ID of every booklet a restaurant adds.
const CustomPrefix = "custom-"

// RecordBookSettings is how a restaurant uses record books: the bundle it follows,
// what it changes in the bundle's booklets, and booklets it adds. Records
// are never changed by these settings; only how they are printed.
type RecordBookSettings struct {
	// Bundle is the ID of the bundle in use.
	Bundle string `json:"bundle"`
	// Books are changes to booklets, by template ID. A booklet with no
	// entry is used as the bundle has it.
	Books []BookSetting `json:"books"`
	// Custom are booklets the restaurant added. Their IDs start with
	// "custom-".
	Custom []Template `json:"custom"`
}

// BookSetting is what a restaurant changes in one booklet.
type BookSetting struct {
	Template string `json:"template"`
	// Off hides the booklet from Record books.
	Off   bool   `json:"off,omitempty"`
	Title string `json:"title,omitempty"`
	// Columns replace the booklet's columns when set.
	Columns []Column `json:"columns,omitempty"`
	// Notes are printed under the booklet's own notes.
	Notes        []string `json:"notes,omitempty"`
	UnitsPerPage int      `json:"units_per_page,omitempty"`
	ManagerCheck *bool    `json:"manager_check,omitempty"`
	// CopyFile is the SHA-256 of the restaurant's copy of the official
	// booklet, uploaded so its edition can be checked.
	CopyFile string `json:"copy_file,omitempty"`
}

func (b BookSetting) empty() bool {
	return !b.Off && b.Title == "" && len(b.Columns) == 0 && len(b.Notes) == 0 &&
		b.UnitsPerPage == 0 && b.ManagerCheck == nil && b.CopyFile == ""
}

// DefaultSettings uses the Irish bundle with no changes.
func DefaultSettings() RecordBookSettings {
	return RecordBookSettings{Bundle: DefaultBundle, Books: []BookSetting{}, Custom: []Template{}}
}

func trimColumns(cols []Column) []Column {
	out := make([]Column, 0, len(cols))
	for _, c := range cols {
		c.Label = strings.TrimSpace(c.Label)
		c.Group = strings.TrimSpace(c.Group)
		c.Fixed = strings.TrimSpace(c.Fixed)
		out = append(out, c)
	}
	return out
}

func trimNotes(notes []string) []string {
	var out []string
	for _, n := range notes {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// Normalize trims text and drops booklet entries that change nothing.
func (s RecordBookSettings) Normalize() RecordBookSettings {
	s.Bundle = strings.TrimSpace(s.Bundle)
	books := []BookSetting{}
	for _, b := range s.Books {
		b.Template = strings.TrimSpace(b.Template)
		b.Title = strings.TrimSpace(b.Title)
		b.CopyFile = strings.ToLower(strings.TrimSpace(b.CopyFile))
		b.Notes = trimNotes(b.Notes)
		if len(b.Columns) > 0 {
			b.Columns = trimColumns(b.Columns)
		}
		if !b.empty() {
			books = append(books, b)
		}
	}
	s.Books = books
	custom := []Template{}
	for _, t := range s.Custom {
		t.ID = strings.TrimSpace(t.ID)
		t.Code = strings.TrimSpace(t.Code)
		t.Title = strings.TrimSpace(t.Title)
		t.Columns = trimColumns(t.Columns)
		t.Notes = trimNotes(t.Notes)
		t.Source, t.Missing = nil, ""
		custom = append(custom, t)
	}
	s.Custom = custom
	return s
}

// Validate checks the settings against the bundles Kishtin has.
func (s RecordBookSettings) Validate(bundles []Bundle) error {
	bundle, ok := FindBundle(bundles, s.Bundle)
	if !ok {
		var ids []string
		for _, b := range bundles {
			ids = append(ids, b.ID)
		}
		return fmt.Errorf("there is no record book bundle %q (there are %s)", s.Bundle, strings.Join(ids, ", "))
	}
	byID := map[string]Template{}
	codes := map[string]bool{}
	for _, t := range bundle.Templates {
		byID[t.ID] = t
		codes[strings.ToLower(t.Code)] = true
	}
	var errs []error
	for _, t := range s.Custom {
		if !strings.HasPrefix(t.ID, CustomPrefix) {
			errs = append(errs, fmt.Errorf("an added booklet's id must start with %q", CustomPrefix))
			continue
		}
		if _, dup := byID[t.ID]; dup || codes[strings.ToLower(t.Code)] {
			errs = append(errs, fmt.Errorf("the code %s is already used: give the added booklet another", t.Code))
			continue
		}
		if t.Builder == "" {
			errs = append(errs, fmt.Errorf("%s: choose what kind of booklet it is", t.Code))
			continue
		}
		if err := t.Validate(); err != nil {
			errs = append(errs, err)
			continue
		}
		byID[t.ID] = t
		codes[strings.ToLower(t.Code)] = true
	}
	seen := map[string]bool{}
	for _, b := range s.Books {
		base, ok := byID[b.Template]
		if !ok {
			errs = append(errs, fmt.Errorf("there is no booklet %q in the %s or among the added booklets", b.Template, bundle.Name))
			continue
		}
		if seen[b.Template] {
			errs = append(errs, fmt.Errorf("%s has two sets of changes: keep one", base.Code))
			continue
		}
		seen[b.Template] = true
		if b.CopyFile != "" && len(b.CopyFile) != 64 {
			errs = append(errs, fmt.Errorf("%s: the uploaded copy must be named by its SHA-256", base.Code))
		}
		if !base.Supported() && (b.Title != "" || len(b.Columns) > 0 || len(b.Notes) > 0 || b.UnitsPerPage != 0 || b.ManagerCheck != nil) {
			errs = append(errs, fmt.Errorf("%s cannot be filled in yet, so only turning it off or uploading a copy can be changed", base.Code))
			continue
		}
		if _, err := apply(base, b); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// apply makes a booklet's changes to its template.
func apply(t Template, b BookSetting) (Template, error) {
	if b.Title != "" {
		t.Title = b.Title
	}
	if len(b.Columns) > 0 {
		t.Columns = slices.Clone(b.Columns)
	}
	if len(b.Notes) > 0 {
		t.Notes = append(slices.Clone(t.Notes), b.Notes...)
	}
	if b.UnitsPerPage != 0 {
		t.UnitsPerPage = b.UnitsPerPage
	}
	if b.ManagerCheck != nil {
		t.ManagerCheck = *b.ManagerCheck
	}
	if err := t.Validate(); err != nil {
		return t, err
	}
	return t, nil
}

// fitWidths scales column widths to fill the page.
func fitWidths(cols []Column) []Column {
	total := 0.0
	for _, c := range cols {
		total += c.Width
	}
	if total <= 0 {
		return cols
	}
	out := slices.Clone(cols)
	for i := range out {
		out[i].Width = out[i].Width * PageWidth / total
	}
	return out
}

// CopyCheck says whether a restaurant's uploaded copy of a booklet is the
// edition the template was made from.
type CopyCheck struct {
	// File is the SHA-256 of the uploaded copy.
	File string `json:"file"`
	// Known is set when the template records the edition it was made
	// from, so the copy can be compared.
	Known bool `json:"known"`
	// Matches is set when the copy is that edition, byte for byte.
	Matches bool `json:"matches"`
}

// Booklet is a booklet as the restaurant uses it.
type Booklet struct {
	// Template is the booklet after the restaurant's changes.
	Template Template `json:"template"`
	// Base is the bundle's own booklet, when the restaurant changed it.
	Base *Template `json:"base,omitempty"`
	// Changed lists what the restaurant changed: "title", "columns",
	// "notes", "sheet layout", "manager check".
	Changed []string `json:"changed"`
	// Custom is set for a booklet the restaurant added.
	Custom bool `json:"custom,omitempty"`
	// Off is set when the booklet is turned off.
	Off  bool       `json:"off,omitempty"`
	Copy *CopyCheck `json:"copy,omitempty"`
}

// Resolve applies the settings to their bundle: the bundle's booklets in
// order, then the added ones, each with the restaurant's changes and
// column widths scaled to the page.
func Resolve(bundles []Bundle, s RecordBookSettings) (Bundle, []Booklet, error) {
	if err := s.Validate(bundles); err != nil {
		return Bundle{}, nil, err
	}
	bundle, _ := FindBundle(bundles, s.Bundle)
	changes := map[string]BookSetting{}
	for _, b := range s.Books {
		changes[b.Template] = b
	}
	out := []Booklet{}
	add := func(base Template, custom bool) {
		ch := changes[base.ID]
		t, _ := apply(base, ch)
		t.Columns = fitWidths(t.Columns)
		bk := Booklet{Template: t, Changed: []string{}, Custom: custom, Off: ch.Off}
		for _, c := range []struct {
			name string
			on   bool
		}{
			{"title", ch.Title != ""},
			{"columns", len(ch.Columns) > 0},
			{"notes", len(ch.Notes) > 0},
			{"sheet layout", ch.UnitsPerPage != 0},
			{"manager check", ch.ManagerCheck != nil},
		} {
			if c.on {
				bk.Changed = append(bk.Changed, c.name)
			}
		}
		if len(bk.Changed) > 0 && !custom {
			b := base
			bk.Base = &b
		}
		if ch.CopyFile != "" {
			bk.Copy = &CopyCheck{File: ch.CopyFile}
			if base.Source != nil {
				bk.Copy.Known = true
				bk.Copy.Matches = base.Source.SHA256 == ch.CopyFile
			}
		}
		out = append(out, bk)
	}
	for _, t := range bundle.Templates {
		add(t, false)
	}
	for _, t := range s.Custom {
		add(t, true)
	}
	return bundle, out, nil
}

// Find returns a booklet by ID or code. Codes are matched without regard
// to case ("sc2").
func Find(booklets []Booklet, ref string) (Booklet, bool) {
	for _, b := range booklets {
		if b.Template.ID == ref || strings.EqualFold(b.Template.Code, ref) {
			return b, true
		}
	}
	return Booklet{}, false
}

// Summarize describes a settings change in words, one line per booklet.
func Summarize(bundles []Bundle, cur, next RecordBookSettings) []string {
	var out []string
	name := func(id string) string {
		if b, ok := FindBundle(bundles, id); ok {
			return b.Name + " (" + b.Country + ")"
		}
		return id
	}
	if cur.Bundle != next.Bundle {
		out = append(out, "Bundle: "+name(cur.Bundle)+" → "+name(next.Bundle))
	}
	codes := map[string]string{}
	for _, b := range bundles {
		for _, t := range b.Templates {
			codes[t.ID] = t.Code
		}
	}
	oldCustom := map[string]Template{}
	for _, t := range cur.Custom {
		oldCustom[t.ID] = t
		codes[t.ID] = t.Code
	}
	newCustom := map[string]bool{}
	for _, t := range next.Custom {
		newCustom[t.ID] = true
		codes[t.ID] = t.Code
		was, ok := oldCustom[t.ID]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s %s added", t.Code, t.Title))
		case !sameJSON(was, t):
			out = append(out, fmt.Sprintf("%s changed", t.Code))
		}
	}
	for _, t := range cur.Custom {
		if !newCustom[t.ID] {
			out = append(out, fmt.Sprintf("%s %s removed", t.Code, t.Title))
		}
	}
	settingFor := func(s RecordBookSettings, id string) BookSetting {
		for _, b := range s.Books {
			if b.Template == id {
				return b
			}
		}
		return BookSetting{Template: id}
	}
	var ids []string
	for _, s := range []RecordBookSettings{cur, next} {
		for _, b := range s.Books {
			if !slices.Contains(ids, b.Template) {
				ids = append(ids, b.Template)
			}
		}
	}
	for _, id := range ids {
		a, b := settingFor(cur, id), settingFor(next, id)
		code := codes[id]
		if code == "" {
			code = id
		}
		if cur.Bundle != next.Bundle && !newCustom[id] {
			continue // the old bundle's changes go with it
		}
		var parts []string
		if a.Off != b.Off {
			if b.Off {
				parts = append(parts, "turned off")
			} else {
				parts = append(parts, "turned back on")
			}
		}
		changed := func(what string, same bool) {
			if !same {
				parts = append(parts, what+" changed")
			}
		}
		changed("title", a.Title == b.Title)
		changed("columns", sameJSON(a.Columns, b.Columns))
		changed("notes", slices.Equal(a.Notes, b.Notes))
		changed("sheet layout", a.UnitsPerPage == b.UnitsPerPage)
		changed("manager check", sameJSON(a.ManagerCheck, b.ManagerCheck))
		switch {
		case a.CopyFile == b.CopyFile:
		case b.CopyFile == "":
			parts = append(parts, "uploaded copy removed")
		case a.CopyFile == "":
			parts = append(parts, "copy uploaded")
		default:
			parts = append(parts, "copy replaced")
		}
		if len(parts) > 0 {
			out = append(out, code+": "+strings.Join(parts, ", "))
		}
	}
	return out
}

func sameJSON(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(x) == string(y)
}
