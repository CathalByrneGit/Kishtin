package books

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareCopy(t *testing.T) {
	sc3, _ := book(t, "SC3")
	// The text of the 2020 SC3 page, as PDFium reads it: table headings
	// broken across lines.
	page := `SC3 - Cooking/Cooling/Reheating Records
Cooking* Cooling* Reheating* Comments/Action
Date Food
Time
Started
Cooking**
Time
Finished
Cooking
Core Temp. Signed
Time Into
Fridge/Blast
Chiller
Signed Core Temp. Signed`
	r := CompareCopy(sc3, []string{page})
	assert.Equal(t, 1, r.Pages)
	assert.Equal(t, r.Total, r.Found, "every SC3 heading is in its own booklet: %+v", r.Columns)
	missing := func(r CopyReport) []string {
		var out []string
		for _, l := range append(r.Columns, r.Questions...) {
			if !l.Found {
				out = append(out, l.Label)
			}
		}
		return out
	}
	// A new edition that renamed "Time Into Fridge/Blast Chiller".
	r = CompareCopy(sc3, []string{`Date Food Time Started Cooking Time Finished Cooking Core Temp Signed Time Chilled Signed Core Temp Signed Comments/Action`})
	assert.Equal(t, []string{"Time into fridge/blast chiller"}, missing(r), "Kishtin's own record number column is not looked for")

	sc5, _ := book(t, "SC5")
	r = CompareCopy(sc5, []string{"Question Satisfactory Yes No Details Of Action Taken Pest Control Are premises pest proofed and free from any signs of pests?"})
	assert.Equal(t, 4+45, r.Total, "4 columns and the 45 FSAI questions")
	assert.Contains(t, r.Questions, LabelCheck{Label: "Are premises pest proofed and free from any signs of pests?", Found: true})
	assert.Contains(t, r.Questions, LabelCheck{Label: "Are freezers working properly?", Found: false})
	assert.LessOrEqual(t, len([]rune(r.Excerpt)), 1501)
	assert.Equal(t, []string{"temp", "c", "oc"}[:1], words("Temp. °C")[:1])
}
