package report

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/books"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

// TestWriteBook renders every booklet Kishtin can fill, for a month with a
// reading, a failed reading, missed checks and a closed day.
func TestWriteBook(t *testing.T) {
	bundles, err := books.Builtin()
	require.NoError(t, err)
	_, booklets, err := books.Resolve(bundles, books.DefaultSettings())
	require.NoError(t, err)
	bundle, _ := books.FindBundle(bundles, books.DefaultBundle)
	loc := time.UTC
	at := func(day, hour int) time.Time { return time.Date(2026, 10, day, hour, 0, 0, 0, loc) }
	temp := func(v float64) *float64 { return &v }
	recs := []haccp.Record{
		{ID: "r1", Seq: 1, Type: haccp.RecordFridge, Outcome: haccp.OutcomePass, LoggedAt: at(1, 9), LoggedByName: "Liam Cook", Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: temp(3.4)}},
		{ID: "r2", Seq: 2, Type: haccp.RecordFridge, Outcome: haccp.OutcomeFail, CorrectiveAction: "Moved food to the prep fridge", LoggedAt: at(2, 9), LoggedByName: "Liam Cook", Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: temp(9.1)}},
		{ID: "r3", Seq: 3, Type: haccp.RecordCook, Outcome: haccp.OutcomePass, LoggedAt: at(2, 13), LoggedByName: "Aoife Chef", Reading: haccp.Reading{Food: "Synthetic chicken", TempC: temp(78)}},
	}
	units := haccp.DefaultLists().Units
	closed := haccp.ClosedDays{Weekdays: []string{"mon"}}

	rendered := 0
	for _, bk := range booklets {
		tpl := bk.Template
		if !tpl.Supported() {
			continue
		}
		var sections []books.Template
		for _, id := range tpl.Sections {
			if sec, ok := books.Find(booklets, id); ok {
				sections = append(sections, sec.Template)
			}
		}
		b, err := books.Build(books.Input{Template: tpl, Bundle: bundle, Year: 2026, Month: time.October, Loc: loc, Now: at(20, 12), Records: recs, Units: units, Sections: sections, Closed: closed})
		require.NoError(t, err, tpl.Code)
		var out bytes.Buffer
		err = WriteBook(Book{Book: b, RestaurantName: "The Synthetic Kitchen", GeneratedAt: at(20, 12), GeneratedBy: "Máire Manager", Loc: loc, Chain: haccp.ChainReport{OK: true, Checked: 3, HeadHash: "abc123"}}, &out)
		require.NoError(t, err, tpl.Code)
		assert.True(t, bytes.HasPrefix(out.Bytes(), []byte("%PDF-")), tpl.Code)
		assert.Greater(t, out.Len(), 1500, tpl.Code)
		rendered++
	}
	assert.GreaterOrEqual(t, rendered, 5, "the built-in bundle's booklets render")
}

func TestBookHelpers(t *testing.T) {
	assert.Equal(t, "October 2026", monthName("2026-10"))
	assert.Equal(t, "soon", monthName("soon"))
	text, tint := flagged(books.Cell{Text: "missed", Flag: books.FlagMissed})
	assert.NotEmpty(t, text)
	assert.NotNil(t, tint)
	_, tint = flagged(books.Cell{Text: "3.4"})
	assert.Nil(t, tint)
}

func TestWriteLabel(t *testing.T) {
	rec := haccp.Record{ID: "r1", Seq: 42, Type: haccp.RecordBatchLabel, Outcome: haccp.OutcomeRecorded, LoggedByName: "Aoife Chef",
		LoggedAt: time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC),
		Reading:  haccp.Reading{Product: "Grape soda syrup with a very long name that must shrink to fit", MadeOn: "2026-11-10", UseBy: "2026-11-15"}}
	for _, sz := range LabelSizes {
		var out bytes.Buffer
		require.NoError(t, WriteLabel(Label{Record: rec, Size: sz}, &out), sz.ID)
		assert.True(t, bytes.HasPrefix(out.Bytes(), []byte("%PDF-")), sz.ID)
		if sz.ID != "a4" {
			// The page is the label, wider than tall: 1 mm is 72/25.4 points.
			box := fmt.Sprintf("/MediaBox [0 0 %.2f %.2f]", sz.W*72/25.4, sz.H*72/25.4)
			assert.Contains(t, out.String(), box, sz.ID)
		}
	}
	def, ok := FindLabelSize("")
	require.True(t, ok)
	assert.Equal(t, "62x29", def.ID)
	_, ok = FindLabelSize("huge")
	assert.False(t, ok)
	assert.Equal(t, "Sun 15 Nov 2026", labelDate("2026-11-15"))

	notLabel := rec
	notLabel.Type = haccp.RecordFridge
	assert.Error(t, WriteLabel(Label{Record: notLabel, Size: def}, &bytes.Buffer{}))
}
