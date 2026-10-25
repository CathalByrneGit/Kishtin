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

	packs, err := f.svc.RecordBookPacks(ctx, f.manager)
	require.NoError(t, err)
	assert.Len(t, packs[0].Templates, 9)

	for _, code := range []string{"SC2", "SC3", "SC4"} {
		var buf bytes.Buffer
		_, sum, err := f.svc.RecordBookPDF(ctx, f.manager, code, "2026-05", &buf)
		require.NoError(t, err, code)
		assert.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF")), code)
		assert.True(t, sum.Chain.OK)
	}
}
