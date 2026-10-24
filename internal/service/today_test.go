package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
)

func cleaning(task string) NewRecord {
	done := true
	return NewRecord{Type: haccp.RecordCleaning, Reading: haccp.Reading{Task: task, Done: &done}}
}

func TestToday(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// start is 09:00 UTC on 1 May: 10:00 in Dublin, so the AM check is due.
	_, err := f.svc.LogRecord(ctx, f.cook, fridge(3.4))
	require.NoError(t, err)
	bad := fridge(7)
	bad.Reading.Unit = "Prep fridge"
	bad.CorrectiveAction = "Moved food to a working unit"
	_, err = f.svc.LogRecord(ctx, f.cook, bad)
	require.NoError(t, err)
	_, err = f.svc.LogRecord(ctx, f.cook, cleaning("floors swept & mopped"))
	require.NoError(t, err)
	five := 5
	_, err = f.svc.LogRecord(ctx, f.cook, NewRecord{Type: haccp.RecordBatchLabel, Reading: haccp.Reading{
		Product: "Grape soda syrup", MadeOn: "2026-05-01", ShelfLifeDays: &five}})
	require.NoError(t, err)

	_, err = f.svc.System().Save(ctx, f.manager, SettingsChange[settings.System]{
		Value:  settings.System{RestaurantName: "The Synthetic Kitchen", BackupKeep: 2, MaxUploadMB: 1, PreviewRecords: 50},
		Reason: "Name the restaurant",
	})
	require.NoError(t, err)

	td, err := f.svc.Today(ctx, f.cook)
	require.NoError(t, err)
	assert.Equal(t, "The Synthetic Kitchen", td.RestaurantName)
	assert.Equal(t, "2026-05-01", td.Date)
	require.Len(t, td.Units, 3)
	assert.Equal(t, haccp.SlotDone, td.Units[0].Slots[0].Status)
	assert.Equal(t, haccp.SlotDue, td.Units[0].Slots[1].Status)
	assert.Equal(t, haccp.OutcomeFail, td.Units[1].Slots[0].Record.Outcome)
	assert.Equal(t, 3, td.Done)
	assert.Equal(t, 12, td.Total)
	assert.Equal(t, 1, td.OutOfLimit)
	assert.Equal(t, haccp.RingAttention, td.Ring)
	require.Len(t, td.Batches, 1)
	assert.Equal(t, "2026-05-06", td.Batches[0].UseBy)
	assert.NotEmpty(t, td.CorrectiveActions)

	f.clk.Advance(4 * time.Hour) // 14:00 in Dublin
	td, err = f.svc.Today(ctx, f.cook)
	require.NoError(t, err)
	assert.Equal(t, haccp.SlotMissed, td.Units[2].Slots[0].Status, "the freezer's AM check was missed")
}

func TestCleaningOncePerDay(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	first, err := f.svc.LogRecord(ctx, f.cook, cleaning("Bins emptied & cleaned"))
	require.NoError(t, err)

	_, err = f.svc.LogRecord(ctx, f.cook, cleaning("bins EMPTIED & cleaned"))
	require.Equal(t, KindConflict, KindOf(err))
	assert.Contains(t, err.Error(), "already done today at 10:00 by Liam Cook")

	fix := cleaning("Bins emptied & cleaned")
	fix.Corrects, fix.CorrectionReason = first.ID, "Ticked by mistake"
	_, err = f.svc.LogRecord(ctx, f.cook, fix)
	require.NoError(t, err, "a correction is allowed")

	f.clk.Advance(24 * time.Hour)
	_, err = f.svc.LogRecord(ctx, f.cook, cleaning("Bins emptied & cleaned"))
	require.NoError(t, err, "tomorrow is a new day")
}

func TestChangesArePublished(t *testing.T) {
	f := setup(t)
	hub := events.NewHub()
	f.svc.events = hub
	ch, stop := hub.Subscribe()
	defer stop()
	ctx := context.Background()

	rec, err := f.svc.LogRecord(ctx, f.cook, fridge(3))
	require.NoError(t, err)
	assert.Equal(t, events.Event{Kind: events.KindRecord, ID: rec.ID}, <-ch)

	_, err = f.svc.LogRecord(ctx, f.cook, fridge(30))
	require.Error(t, err)
	select {
	case e := <-ch:
		t.Fatalf("a refused record published %v", e)
	default:
	}

	_, err = f.svc.Rules().Save(ctx, f.manager, SettingsChange[haccp.Rules]{Value: haccp.Rules{BorderlineNeedsAction: true}, Reason: "Free names"})
	require.NoError(t, err)
	assert.Equal(t, events.Event{Kind: events.KindSettings, ID: "rules"}, <-ch)
}
