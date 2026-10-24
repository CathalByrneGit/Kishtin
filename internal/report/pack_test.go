package report

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

func TestWritePack(t *testing.T) {
	dublin, err := time.LoadLocation("Europe/Dublin")
	require.NoError(t, err)
	t0 := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	var recs []haccp.Record
	var prev *haccp.Record
	for k := range 60 {
		temp := 3.0 + float64(k%5)
		o := haccp.OutcomePass
		action := ""
		if temp > 5 {
			o, action = haccp.OutcomeFail, "Moved food to a working unit, called the engineer, re-checked after thirty minutes"
		}
		r := haccp.Seal(haccp.Record{
			ID: fmt.Sprintf("rec-%d", k), Type: haccp.RecordFridge, LoggedAt: t0.Add(time.Duration(k) * 5 * time.Hour),
			LoggedBy: "u1", LoggedByName: "Siobhán (synthetic)",
			Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: &temp}, Outcome: o,
			OutcomeReason: "reason", CorrectiveAction: action, LimitVersionID: haccp.FSAIDefaultID(haccp.CheckFridge),
		}, prev)
		recs = append(recs, r)
		prev = &recs[len(recs)-1]
	}
	var buf bytes.Buffer
	err = Write(Pack{
		RestaurantName: "The Synthetic Kitchen − Café",
		From:           t0, To: t0.AddDate(0, 0, 14),
		GeneratedAt: t0, GeneratedBy: "Máire Manager", Loc: dublin,
		Records: recs, Chain: haccp.VerifyChain(recs), FilesChecked: 2,
		Guidance: "FSAI", Limits: haccp.FSAIDefaults(),
	}, &buf)
	require.NoError(t, err)
	if out := os.Getenv("KISHTIN_SAMPLE_PDF"); out != "" {
		require.NoError(t, os.WriteFile(out, buf.Bytes(), 0o600))
	}
	assert.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")))
	assert.Greater(t, bytes.Count(buf.Bytes(), []byte("/Type /Page\n")), 2, "the records span several pages")

	buf.Reset()
	require.NoError(t, Write(Pack{GeneratedAt: t0}, &buf), "an empty pack still renders")
}

func TestClean(t *testing.T) {
	assert.Equal(t, "-18.0°C -> Siobhán's '?'", clean("−18.0°C → Siobhán’s ‘✓’"))
}
