package books

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

func signOff(r *recs, at time.Time, from, to, note string) {
	r.add(haccp.RecordSignOff, at, haccp.Reading{PeriodFrom: from, PeriodTo: to, Notes: note, Review: &haccp.Review{}}, haccp.OutcomeRecorded,
		func(x *haccp.Record) { x.LoggedByName = "Máire Manager" })
}

func TestReviewPeriod(t *testing.T) {
	units := []haccp.Unit{{Name: "Walk-in fridge", Kind: haccp.RecordFridge}}
	var r recs
	r.add(haccp.RecordFridge, on(1, 9, 0), haccp.Reading{Unit: "Walk-in fridge", TempC: c(3)}, haccp.OutcomePass)
	r.add(haccp.RecordFridge, on(1, 15, 0), haccp.Reading{Unit: "Walk-in fridge", TempC: c(7)}, haccp.OutcomeFail, func(x *haccp.Record) { x.CorrectiveAction = "Moved food" })
	r.add(haccp.RecordHotHold, on(2, 12, 0), haccp.Reading{Food: "Soup", TempC: c(60)}, haccp.OutcomeBorderline)
	r.add(haccp.RecordFridge, on(4, 9, 0), haccp.Reading{Unit: "Walk-in fridge", TempC: c(3)}, haccp.OutcomePass)
	signOff(&r, on(3, 10, 0), "2026-09-24", "2026-09-30", "")

	res := ReviewPeriod(ReviewInput{From: on(1, 0, 0), To: on(3, 0, 0), Loc: dublin, Now: on(10, 12, 0), Since: on(1, 9, 0), Records: r.list, Units: units})
	assert.Equal(t, haccp.Review{Records: 3, OutOfLimit: 2, WithoutAction: 1, Missed: 4}, res.Review, "the sign-off and the 4th are not counted")
	assert.Equal(t, "Soup", res.WithoutAction[0].Reading.Food)
	assert.Equal(t, []Gap{
		{Date: "2026-10-02", Unit: "Walk-in fridge", Check: "AM"},
		{Date: "2026-10-02", Unit: "Walk-in fridge", Check: "PM"},
		{Date: "2026-10-03", Unit: "Walk-in fridge", Check: "AM"},
		{Date: "2026-10-03", Unit: "Walk-in fridge", Check: "PM"},
	}, res.Missed)

	// Across a month end.
	across := ReviewPeriod(ReviewInput{From: time.Date(2026, 9, 29, 0, 0, 0, 0, dublin), To: on(1, 0, 0), Loc: dublin, Now: on(10, 12, 0),
		Since: time.Date(2026, 9, 1, 0, 0, 0, 0, dublin), Records: r.list, Units: units})
	assert.Equal(t, 4, across.Review.Missed, "29 and 30 September, both checks")
	assert.Equal(t, 2, across.Review.Records)
}

func TestBookSignOffs(t *testing.T) {
	tpl, bundle := book(t, "SC2")
	var r recs
	r.add(haccp.RecordFridge, on(1, 9, 0), haccp.Reading{Unit: "Walk-in fridge", TempC: c(3)}, haccp.OutcomePass)
	signOff(&r, on(8, 10, 0), "2026-10-01", "2026-10-07", "")
	signOff(&r, on(16, 10, 0), "2026-10-10", "2026-10-14", "Closed 8 and 9 October")
	signOff(&r, on(20, 10, 0), "2026-11-01", "2026-11-07", "") // another month
	b, err := Build(Input{Template: tpl, Bundle: bundle, Year: 2026, Month: time.October, Loc: dublin, Now: on(20, 12, 0), Since: on(1, 9, 0), Records: r.list})
	require.NoError(t, err)
	require.Len(t, b.SignOffs, 2)
	assert.Equal(t, SignOff{Seq: 2, Date: "2026-10-08", By: "Máire Manager", From: "2026-10-01", To: "2026-10-07"}, b.SignOffs[0])
	assert.Equal(t, "Closed 8 and 9 October", b.SignOffs[1].Note)
	assert.Equal(t, []Span{{From: "2026-10-08", To: "2026-10-09"}, {From: "2026-10-15", To: "2026-10-19"}}, b.Unsigned, "up to yesterday")
	assert.Equal(t, 1, b.Records, "sign-offs are not readings")
}
