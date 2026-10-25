package service

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/books"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

func TestRecordBook(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// 1 May, 10:00 Dublin: the walk-in AM check.
	_, err := f.svc.LogRecord(ctx, f.cook, fridge(3.4))
	require.NoError(t, err)
	f.clk.Advance(5 * time.Hour)
	bad := fridge(7.5)
	bad.CorrectiveAction = "Moved food to the prep fridge"
	_, err = f.svc.LogRecord(ctx, f.cook, bad)
	require.NoError(t, err)
	_, err = f.svc.LogRecord(ctx, f.cook, NewRecord{Type: haccp.RecordHotHold, Reading: haccp.Reading{Food: "Soup", TempC: fp(70)}})
	require.NoError(t, err)
	// 2 May, 13:00 Dublin.
	f.clk.Set(start.Add(27 * time.Hour))

	_, err = f.svc.RecordBook(ctx, f.cook, "SC2", "2026-05")
	assert.Equal(t, KindForbidden, KindOf(err), "line cooks cannot export")

	b, err := f.svc.RecordBook(ctx, f.manager, "sc2", "")
	require.NoError(t, err)
	assert.Equal(t, "2026-05", b.Month, "this month by default")
	require.Len(t, b.Pages, 1)
	day1 := b.Pages[0].Rows[0].Cells
	assert.Equal(t, "3.4°C", day1[1].Text)
	assert.Equal(t, books.Cell{Text: "7.5°C", Flag: books.FlagFail}, day1[2])
	assert.Equal(t, books.Cell{Text: "missed", Flag: books.FlagMissed}, day1[3], "prep fridge AM")
	// Prep and chest AM and PM on the 1st, all three AM on the 2nd.
	assert.Len(t, b.Gaps, 7)
	assert.Equal(t, 2, b.Records)

	sc4, err := f.svc.RecordBook(ctx, f.manager, "SC4", "2026-05")
	require.NoError(t, err)
	require.Len(t, sc4.Pages[0].Rows, 1)
	assert.Equal(t, "Soup", sc4.Pages[0].Rows[0].Cells[1].Text)

	empty, err := f.svc.RecordBook(ctx, f.manager, "SC2", "2026-04")
	require.NoError(t, err)
	assert.Empty(t, empty.Gaps, "nothing is missed before the first record")

	_, err = f.svc.RecordBook(ctx, f.manager, "SC2", "2026-06")
	assert.Equal(t, KindInvalid, KindOf(err), "a future month")
	_, err = f.svc.RecordBook(ctx, f.manager, "SC2", "May")
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = f.svc.RecordBook(ctx, f.manager, "SC7", "")
	assert.Equal(t, KindInvalid, KindOf(err), "not supported yet")
	_, err = f.svc.RecordBook(ctx, f.manager, "SC99", "")
	assert.Equal(t, KindNotFound, KindOf(err))

	rb, err := f.svc.RecordBooks(ctx, f.manager)
	require.NoError(t, err)
	assert.Equal(t, "Ireland", rb.Bundle.Country)
	assert.Len(t, rb.Booklets, 9)
	assert.Len(t, rb.Bundles, 2)
	assert.Equal(t, books.BuiltinSettingsID, rb.SettingsVersion)

	for _, code := range []string{"SC2", "SC3", "SC4"} {
		var buf bytes.Buffer
		_, sum, err := f.svc.RecordBookPDF(ctx, f.manager, code, "2026-05", &buf)
		require.NoError(t, err, code)
		assert.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF")), code)
		assert.True(t, sum.Chain.OK)
	}
}

func TestRecordBookSettings(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.svc.LogRecord(ctx, f.cook, NewRecord{Type: haccp.RecordHotHold, Reading: haccp.Reading{Food: "Soup", TempC: fp(70)}})
	require.NoError(t, err)
	g := f.svc.RecordBookSettings()

	cur, err := g.Current(ctx, f.cook)
	require.NoError(t, err)
	assert.True(t, cur.Builtin)
	assert.Equal(t, books.DefaultBundle, cur.Value.Bundle)

	next := cur.Value
	next.Books = []books.BookSetting{
		{Template: "fsai-sc4", Title: "Soup and stew holding", Columns: []books.Column{
			{Label: "Date", Field: "date", Width: 1}, {Label: "Food", Field: "food", Width: 2}, {Label: "1st check", Field: "check_1", Width: 1},
		}},
		{Template: "fsai-sc9", Off: true},
	}
	next.Custom = []books.Template{{
		ID: "custom-t9", Code: "T9", Title: "Second hot hold log", Builder: books.BuilderHotHold,
		Columns: []books.Column{{Label: "Food", Field: "food", Width: 1}, {Label: "Temp", Field: "check_1", Width: 1}},
	}}
	ch := SettingsChange[books.RecordBookSettings]{Value: next, Reason: "Match our HACCP plan"}

	_, err = g.Save(ctx, f.cook, ch)
	assert.Equal(t, KindForbidden, KindOf(err), "line cooks cannot change record books")

	pv, err := g.Preview(ctx, f.manager, ch)
	require.NoError(t, err)
	assert.Empty(t, pv.Problem)
	assert.Empty(t, pv.LessStrict, "printing changes never make food safety less strict")
	assert.Equal(t, []string{
		"T9 Second hot hold log added",
		"SC4: title changed, columns changed",
		"SC9: turned off",
	}, pv.Changes, "the change in words, not field by field")
	assert.Equal(t, []string{
		"SC4 will no longer show: Core temp (2nd check), Core temp (3rd check), Comments/Action, Signed, Kishtin ref.",
		"Saved records do not change. Only how record books are printed changes.",
	}, pv.Notes)

	bad := next
	bad.Bundle = "fr-ddpp"
	pv, err = g.Preview(ctx, f.manager, SettingsChange[books.RecordBookSettings]{Value: bad, Reason: "x"})
	require.NoError(t, err)
	assert.Contains(t, pv.Problem, `no record book bundle "fr-ddpp"`)

	_, err = g.Save(ctx, f.manager, ch)
	require.NoError(t, err)

	rb, err := f.svc.RecordBooks(ctx, f.manager)
	require.NoError(t, err)
	assert.NotEqual(t, books.BuiltinSettingsID, rb.SettingsVersion)
	assert.Len(t, rb.Booklets, 10)

	b, err := f.svc.RecordBook(ctx, f.manager, "SC4", "")
	require.NoError(t, err)
	assert.Equal(t, "Soup and stew holding", b.Template.Title)
	assert.Equal(t, []string{"title", "columns"}, b.Changed)
	assert.Equal(t, rb.SettingsVersion, b.SettingsVersion)
	require.Len(t, b.Pages[0].Rows, 1)
	assert.Len(t, b.Pages[0].Rows[0].Cells, 3)

	t9, err := f.svc.RecordBook(ctx, f.manager, "t9", "")
	require.NoError(t, err)
	assert.Equal(t, "Soup", t9.Pages[0].Rows[0].Cells[0].Text)

	var buf bytes.Buffer
	_, _, err = f.svc.RecordBookPDF(ctx, f.manager, "SC4", "", &buf)
	require.NoError(t, err)

	// Switch to the general bundle: changes to FSAI booklets must go.
	gen := books.RecordBookSettings{Bundle: "general", Custom: next.Custom}
	pv, err = g.Preview(ctx, f.manager, SettingsChange[books.RecordBookSettings]{Value: gen, Reason: "Move to the general books"})
	require.NoError(t, err)
	assert.Empty(t, pv.Problem)
	assert.Equal(t, "Record books will use the Kishtin general record books (Any country).", pv.Notes[0])
	assert.Equal(t, []string{"Bundle: FSAI Safe Catering record books (Ireland) → Kishtin general record books (Any country)"}, pv.Changes,
		"changes to the old bundle's booklets go with it")
	_, err = f.svc.RecordBook(ctx, f.manager, "SC4", "")
	require.NoError(t, err, "not saved yet")
}
