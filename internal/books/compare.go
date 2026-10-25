package books

import (
	"strings"
	"unicode"
)

// LabelCheck is one column heading or question, and whether a copy of the
// booklet has its words.
type LabelCheck struct {
	Label string `json:"label"`
	Found bool   `json:"found"`
}

// CopyReport compares a booklet with the text of a copy of the official
// PDF, such as a new edition. A column or question counts as found when
// every word of it appears in the copy; the text of a PDF table is in
// reading order, so the words of a heading are not always side by side.
type CopyReport struct {
	Pages     int          `json:"pages"`
	Columns   []LabelCheck `json:"columns"`
	Questions []LabelCheck `json:"questions"`
	// Found and Total count the columns and questions together.
	Found int `json:"found"`
	Total int `json:"total"`
	// Excerpt is the start of the copy's text, for a person to read.
	Excerpt string `json:"excerpt"`
}

// stopWords are left out when matching words.
var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true, "by": true, "e": true, "g": true,
	"for": true, "if": true, "in": true, "is": true, "it": true, "of": true, "on": true, "or": true, "the": true, "to": true,
}

// words splits text into lower-case words, without punctuation or stop
// words. "Temp." and "temp" are the same word; "°C" and "OC" both become
// "c" and "oc" and are dropped as too short or kept as written.
func words(s string) []string {
	f := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	out := f[:0]
	for _, w := range f {
		if len(w) > 1 && !stopWords[w] {
			out = append(out, w)
		}
	}
	return out
}

// CompareCopy checks which of a booklet's column headings, further
// training headings and checklist items appear in a copy's pages.
func CompareCopy(t Template, pages []string) CopyReport {
	have := map[string]bool{}
	for _, p := range pages {
		for _, w := range words(p) {
			have[w] = true
		}
	}
	check := func(label string) LabelCheck {
		ws := words(label)
		found := len(ws) > 0
		for _, w := range ws {
			if !have[w] {
				found = false
				break
			}
		}
		return LabelCheck{Label: label, Found: found}
	}
	r := CopyReport{Pages: len(pages), Columns: []LabelCheck{}, Questions: []LabelCheck{}}
	seen := map[string]bool{}
	for _, c := range append(append([]Column{}, t.Columns...), t.FurtherColumns...) {
		// The record number column is Kishtin's own, not the booklet's.
		if seen[c.Label] || c.Field == "ref" {
			continue
		}
		seen[c.Label] = true
		r.Columns = append(r.Columns, check(c.Label))
	}
	for _, sec := range t.Checklist {
		for _, it := range sec.Items {
			r.Questions = append(r.Questions, check(it))
		}
	}
	for _, l := range append(append([]LabelCheck{}, r.Columns...), r.Questions...) {
		r.Total++
		if l.Found {
			r.Found++
		}
	}
	text := strings.Join(strings.Fields(strings.Join(pages, "\n")), " ")
	if len([]rune(text)) > 1500 {
		text = string([]rune(text)[:1500]) + "…"
	}
	r.Excerpt = text
	return r
}
