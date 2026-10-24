package haccp

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dublin = func() *time.Location {
	l, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		panic(err)
	}
	return l
}()

// at returns 1 May 2026 at hh:mm Dublin time (UTC+1).
func at(hh, mm int) time.Time { return time.Date(2026, 5, 1, hh, mm, 0, 0, dublin) }

type builder struct {
	recs []Record
	seq  int64
}

func (b *builder) add(r Record) Record {
	b.seq++
	r.Seq = b.seq
	if r.ID == "" {
		r.ID = fmt.Sprintf("r%d", b.seq)
	}
	b.recs = append(b.recs, r)
	return r
}

func temp(unit string, kind RecordType, when time.Time, c float64, o Outcome) Record {
	return Record{Type: kind, LoggedAt: when, Reading: Reading{Unit: unit, TempC: &c}, Outcome: o}
}

func TestBuildTodaySlots(t *testing.T) {
	var b builder
	b.add(temp("Walk-in fridge", RecordFridge, at(8, 0), 6.0, OutcomeFail))
	fixed := b.add(temp("walk-in FRIDGE", RecordFridge, at(9, 0), 3.4, OutcomePass))
	b.add(temp("Chest freezer", RecordFreezer, at(13, 0), -19, OutcomePass))
	b.add(temp("Walk-in fridge", RecordFreezer, at(13, 0), -19, OutcomePass)) // wrong kind: ignored

	in := TodayInput{Now: at(14, 0), Loc: dublin, Records: b.recs, Lists: DefaultLists(), Limits: FSAIDefaults()}
	in.Records[0].CorrectiveAction = "Adjusted thermostat, recheck in 30 min"
	td := BuildToday(in)

	assert.Equal(t, "2026-05-01", td.Date)
	require.Len(t, td.Units, 3)
	walkin := td.Units[0]
	assert.Equal(t, 5.0, *walkin.Limit.MaxC)
	require.Len(t, walkin.Slots, 2)
	assert.Equal(t, SlotDone, walkin.Slots[0].Status)
	assert.Equal(t, fixed.ID, walkin.Slots[0].Record.ID, "the latest reading in the slot counts")
	assert.Equal(t, SlotDue, walkin.Slots[1].Status, "the last check of the day is never missed")

	prep := td.Units[1]
	assert.Equal(t, SlotMissed, prep.Slots[0].Status, "AM is missed after 12:00")
	freezer := td.Units[2]
	assert.Equal(t, SlotMissed, freezer.Slots[0].Status)
	assert.Equal(t, "12:00", freezer.Slots[0].Until)
	assert.Equal(t, SlotDone, freezer.Slots[1].Status)

	assert.Equal(t, 2, td.Done)
	assert.Equal(t, 3*2+6, td.Total, "2 per unit plus 1 per cleaning task")
	assert.Equal(t, 16, td.Percent)
	assert.Equal(t, 1, td.OutOfLimit)
	assert.Equal(t, 0, td.WithoutAction)
	assert.Equal(t, RingAttention, td.Ring, "red while any reading today was out of limit")
	require.Len(t, td.NeedsAttention, 1)
}

func TestBuildTodayCustomChecksAndRing(t *testing.T) {
	lists := Lists{Units: []Unit{{Name: "Bar fridge", Kind: RecordFridge, Checks: []CheckTime{
		{Label: "Opening", From: "08:00"}, {Label: "Mid-service", From: "14:00"}, {Label: "Close", From: "21:00"},
	}}}}
	var b builder
	b.add(temp("Bar fridge", RecordFridge, at(6, 30), 3, OutcomePass)) // before opening: counts as Opening
	in := TodayInput{Now: at(15, 0), Loc: dublin, Records: b.recs, Lists: lists, Limits: FSAIDefaults()}
	td := BuildToday(in)
	slots := td.Units[0].Slots
	assert.Equal(t, []string{SlotDone, SlotDue, SlotDue}, []string{slots[0].Status, slots[1].Status, slots[2].Status})
	assert.Equal(t, RingInProgress, td.Ring)

	b.add(temp("Bar fridge", RecordFridge, at(14, 30), 3, OutcomePass))
	b.add(temp("Bar fridge", RecordFridge, at(21, 30), 3, OutcomePass))
	in.Records, in.Now = b.recs, at(22, 0)
	td = BuildToday(in)
	assert.Equal(t, 100, td.Percent)
	assert.Equal(t, RingComplete, td.Ring)
	assert.Equal(t, 0, td.OutOfLimit)
}

func TestBuildTodayCorrections(t *testing.T) {
	var b builder
	orig := b.add(temp("Walk-in fridge", RecordFridge, at(9, 0), 7.0, OutcomeFail))
	b.recs[0].CorrectiveAction = "Moved food"
	fix := temp("Walk-in fridge", RecordFridge, at(15, 0), 3.0, OutcomePass)
	fix.Corrects, fix.CorrectionReason = orig.ID, "Misread the probe"
	fixed := b.add(fix)
	other := temp("Prep fridge", RecordFridge, at(15, 0), 3.0, OutcomePass)
	other.Corrects = "from-yesterday"
	b.add(other)

	td := BuildToday(TodayInput{Now: at(16, 0), Loc: dublin, Records: b.recs, Lists: DefaultLists(), Limits: FSAIDefaults()})
	am := td.Units[0].Slots[0]
	assert.Equal(t, fixed.ID, am.Record.ID, "the correction fills the original's AM slot")
	assert.Equal(t, SlotDue, td.Units[0].Slots[1].Status, "and not the PM slot it was logged in")
	assert.Equal(t, 0, td.OutOfLimit, "the corrected fail no longer counts")
	assert.Equal(t, SlotMissed, td.Units[1].Slots[0].Status, "a correction of another day's record is left out")
}

func TestBuildTodayCleaningAndBatches(t *testing.T) {
	var b builder
	done := true
	b.add(Record{Type: RecordCleaning, LoggedAt: at(10, 0), Reading: Reading{Task: "floors swept & mopped", Done: &done}, Outcome: OutcomePass})
	in := TodayInput{Now: at(11, 0), Loc: dublin, Records: b.recs, Lists: DefaultLists(), Limits: FSAIDefaults()}

	batch := func(id string, seq int64, useBy string) Record {
		return Record{ID: id, Seq: seq, Type: RecordBatchLabel, Reading: Reading{Product: "Syrup " + id, MadeOn: "2026-04-20", UseBy: useBy}, Outcome: OutcomeRecorded}
	}
	in.BatchLabels = []Record{
		batch("old", 1, "2026-04-27"), batch("expired", 2, "2026-04-29"),
		batch("today", 3, "2026-05-01"), batch("ok", 4, "2026-05-05"),
	}
	for k := range 20 {
		in.BatchLabels = append(in.BatchLabels, batch(fmt.Sprintf("x%d", k), int64(10+k), "2026-05-03"))
	}
	td := BuildToday(in)

	require.NotNil(t, td.Cleaning[3].Record)
	assert.Equal(t, "Floors swept & mopped", td.Cleaning[3].Task)
	assert.Nil(t, td.Cleaning[0].Record)
	assert.Equal(t, 1, td.Done)
	assert.NotNil(t, DoneToday(b.recs, "FLOORS SWEPT & MOPPED"))
	assert.Nil(t, DoneToday(b.recs, "Bins emptied & cleaned"))

	assert.Len(t, td.Batches, 12, "at most 12 batches")
	assert.Equal(t, "x19", td.Batches[0].Record.ID, "newest first")

	in.BatchLabels = in.BatchLabels[:4]
	td = BuildToday(in)
	require.Len(t, td.Batches, 3, "use-by more than 3 days ago is left out")
	assert.Equal(t, BatchOK, td.Batches[0].Status)
	assert.Equal(t, BatchUseToday, td.Batches[1].Status)
	assert.Equal(t, BatchExpired, td.Batches[2].Status)
}
