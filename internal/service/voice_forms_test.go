package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
)

func TestVoiceVoid(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	o, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: "4", Items: []NewItem{{DishID: k.burger.ID, Quantity: 3}}})
	require.NoError(t, err)
	o, err = k.svc.FireCourse(ctx, k.chef, o.ID, menu.CourseMain)
	require.NoError(t, err)
	_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: []string{o.Items[0].ID}, From: orders.StateFired})
	require.NoError(t, err)
	say := func(text string) VoiceReply {
		t.Helper()
		r, err := k.svc.VoiceCommand(ctx, k.chef, text)
		require.NoError(t, err)
		return r
	}

	// A cook cannot void, by voice or by touch.
	r, err := k.svc.VoiceCommand(ctx, k.cook, "void burger table 4")
	require.NoError(t, err)
	assert.Equal(t, voice.OutcomeRefused, r.Outcome)

	r = say("void burger table 4")
	assert.Equal(t, "How many: there are 3 Synthetic burger for Table 4. Say a number, or all.", r.Say)
	r = say("one")
	assert.Equal(t, "Which: 1 Synthetic burger, cooking or 1 Synthetic burger, waiting to start?", r.Say)
	assert.Equal(t, []string{"1 Synthetic burger, cooking", "1 Synthetic burger, waiting to start"}, r.Options)
	r = say("the first one")
	assert.Equal(t, "Why is it voided?", r.Say)
	r = say("dropped on the floor")
	assert.Equal(t, "Void 1 Synthetic burger (cooking) for Table 4: Dropped on the floor. Say confirm void, or cancel.", r.Say)
	assert.True(t, r.Confirm)
	r = say("yes please")
	assert.Equal(t, "Say confirm void, or cancel.", r.Say, "only a confirm word voids")
	o, err = k.svc.GetOrder(ctx, k.chef, o.ID)
	require.NoError(t, err)
	assert.Equal(t, orders.StateCooking, o.Items[0].State, "nothing voided yet")
	r = say("confirm void")
	assert.Equal(t, voice.OutcomeDone, r.Outcome)
	assert.Equal(t, "Voided 1 Synthetic burger (cooking) for Table 4.", r.Say)
	o, err = k.svc.GetOrder(ctx, k.chef, o.ID)
	require.NoError(t, err)
	assert.Equal(t, orders.StateVoided, o.Items[0].State)
	assert.Equal(t, "Dropped on the floor (by voice)", o.Items[0].VoidReason)

	// Count and reason said up front; cancel voids nothing.
	r = say("void 2 burgers table 4 because table changed their mind")
	assert.Equal(t, "Void 2 Synthetic burger (waiting to start) for Table 4: Table changed their mind. Say confirm void, or cancel.", r.Say)
	r = say("cancel")
	assert.Equal(t, voice.OutcomeCancelled, r.Outcome)
	o, err = k.svc.GetOrder(ctx, k.chef, o.ID)
	require.NoError(t, err)
	assert.Equal(t, orders.StateFired, o.Items[1].State)

	r = say("void salad table 4")
	assert.Equal(t, "No Synthetic salad to void for Table 4.", r.Say)
}

func TestVoiceRecordsInSteps(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	say := func(text string) VoiceReply {
		t.Helper()
		r, err := k.svc.VoiceCommand(ctx, k.cook, text)
		require.NoError(t, err)
		return r
	}
	last := func() haccp.Record {
		t.Helper()
		recs, err := k.svc.st.HACCP().ListRecords(ctx, store.RecordFilter{Newest: true, Limit: 1})
		require.NoError(t, err)
		require.Len(t, recs, 1)
		return recs[0]
	}

	// A delivery said in full is saved straight away.
	r := say("delivery chilled chicken from Synthetic Farm 3 degrees packaging fine in date")
	assert.Equal(t, "Delivery of Chicken from Synthetic Farm, 3 degrees. Pass. Saved.", r.Say)
	rec := last()
	assert.Equal(t, haccp.RecordDelivery, rec.Type)
	assert.Equal(t, "Synthetic Farm", rec.Reading.Supplier)

	// Or step by step.
	r = say("delivery")
	assert.Equal(t, "Who is the delivery from?", r.Say)
	r = say("synthetic foods")
	assert.Equal(t, "What was delivered?", r.Say)
	r = say("frozen peas")
	assert.Equal(t, "Chilled, frozen or ambient?", r.Say)
	r = say("maybe")
	assert.Equal(t, "Sorry, I did not catch that. Chilled, frozen or ambient?", r.Say)
	r = say("frozen")
	assert.Equal(t, "What temperature is it?", r.Say)
	r = say("minus 12")
	assert.Equal(t, "Is the packaging intact? Yes or no.", r.Say)
	r = say("yes")
	assert.Equal(t, "Is it all in date? Yes or no.", r.Say)
	r = say("yes")
	assert.Contains(t, r.Say, "Delivery of Frozen peas from Synthetic foods, -12 degrees: Fail.")
	assert.Contains(t, r.Say, "What did you do about it?")
	r = say("rejected the delivery")
	r = say("confirm")
	assert.Equal(t, voice.OutcomeDone, r.Outcome)
	rec = last()
	assert.Equal(t, haccp.OutcomeFail, rec.Outcome)
	assert.Equal(t, "Rejected the delivery", rec.CorrectiveAction)

	// Cooling: cooked 90 minutes ago, into the fridge now: a pass.
	r = say("cooling rice cooked 90 minutes ago")
	assert.Equal(t, voice.OutcomeDone, r.Outcome)
	rec = last()
	assert.Equal(t, haccp.RecordCooling, rec.Type)
	assert.True(t, k.clk.Now().Add(-90*time.Minute).Equal(*rec.Reading.CookedAt))
	assert.Contains(t, r.Say, "Rice into the fridge, cooked at")
	r = say("cooling")
	assert.Equal(t, "What is going in to cool?", r.Say)
	r = say("chicken stock")
	assert.Equal(t, "When was it cooked? Say a time like 14:30, or how long ago.", r.Say)
	now := k.clk.Now().In(clock.Dublin)
	r = say(now.Add(-3 * time.Hour).Format("15:04"))
	assert.Equal(t, voice.OutcomeAsked, r.Outcome, "more than 2 hours: needs an action")
	r = say("cancel")
	assert.Equal(t, voice.OutcomeCancelled, r.Outcome)

	// Batch label.
	r = say("batch grape soda syrup")
	assert.Equal(t, "How many days does it keep?", r.Say)
	r = say("five days")
	assert.Equal(t, voice.OutcomeDone, r.Outcome)
	rec = last()
	assert.Equal(t, haccp.RecordBatchLabel, rec.Type)
	assert.Equal(t, now.AddDate(0, 0, 5).Format(haccp.DateLayout), rec.Reading.UseBy)
	assert.Equal(t, "Grape soda syrup, use by "+now.AddDate(0, 0, 5).Format("Monday 2 January")+". Saved.", r.Say)

	// Cleaning.
	r = say("floors done")
	assert.Equal(t, "Floors swept & mopped: done. Saved.", r.Say)
	r = say("cleaning")
	assert.Equal(t, "Which cleaning task?", r.Say)
	assert.Len(t, r.Options, 6)
	r = say("bins")
	assert.Equal(t, "Bins emptied & cleaned: done. Saved.", r.Say)
}

func TestCookedAt(t *testing.T) {
	now := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC) // 09:00 Dublin (winter time)
	at, err := cookedAt(voice.Cooked{Clock: "08:15"}, now)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 10, 25, 8, 15, 0, 0, time.UTC), at.UTC())
	at, err = cookedAt(voice.Cooked{Clock: "23:30"}, now)
	require.NoError(t, err)
	// Yesterday, 24 October, was still summer time: 23:30 Dublin is 22:30 UTC.
	assert.Equal(t, time.Date(2026, 10, 24, 22, 30, 0, 0, time.UTC), at.UTC(), "later than now means yesterday")
	at, err = cookedAt(voice.Cooked{Ago: time.Hour}, now)
	require.NoError(t, err)
	assert.Equal(t, now.Add(-time.Hour), at)
}
