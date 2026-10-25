package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/speech"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
)

func TestVoiceKitchenCommands(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	o, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: "4", Items: []NewItem{{DishID: k.burger.ID, Quantity: 2}, {DishID: k.salad.ID}}})
	require.NoError(t, err)
	say := func(_ any, text string) VoiceReply {
		t.Helper()
		r, err := k.svc.VoiceCommand(ctx, k.chef, text)
		require.NoError(t, err)
		return r
	}

	r := say(nil, "Fire table 4 mains")
	assert.Equal(t, voice.OutcomeDone, r.Outcome)
	assert.Equal(t, "Table 4 mains fired, due 10:11.", r.Say)
	assert.Equal(t, "Fire table 4 mains", r.Heard)

	r = say(nil, "what's next on grill")
	assert.Equal(t, voice.OutcomeAnswered, r.Outcome)
	assert.Equal(t, "Grill. Start now: 2 Synthetic burger for Table 4.", r.Say)

	r = say(nil, "burgers on table 4")
	assert.Equal(t, "2 Synthetic burger on for Table 4.", r.Say)
	r = say(nil, "burgers on table 4")
	assert.Equal(t, voice.OutcomeRefused, r.Outcome)
	assert.Equal(t, "No Synthetic burger waiting to start for Table 4.", r.Say)

	r = say(nil, "table 4 ready")
	assert.Equal(t, "Table 4 mains ready at the pass.", r.Say)
	o, err = k.svc.GetOrder(ctx, k.chef, o.ID)
	require.NoError(t, err)
	for _, it := range o.ItemsIn(menu.CourseMain) {
		assert.Equal(t, orders.StateReady, it.State)
	}

	r = say(nil, "fire table 4")
	assert.Equal(t, "Table 4 starters fired, due 10:03.", r.Say, "the next held course")
	r = say(nil, "hold table 4")
	assert.Equal(t, "Table 4 starters held again.", r.Say, "the fired course not yet started")

	r = say(nil, "86 burger")
	assert.Equal(t, "Synthetic burger is 86.", r.Say)
	r = say(nil, "burger back on")
	assert.Equal(t, "Synthetic burger is back on.", r.Say)

	// Front of house cannot fire: the same permissions as the screens.
	r, err = k.svc.VoiceCommand(ctx, k.foh, "hold table 4 mains")
	require.NoError(t, err)
	assert.Equal(t, voice.OutcomeRefused, r.Outcome)

	r = say(nil, "lovely weather")
	assert.Equal(t, voice.OutcomeNotUnderstood, r.Outcome)
	r = say(nil, "confirm")
	assert.Equal(t, "Nothing is waiting for an answer.", r.Say)

	log, err := k.svc.VoiceLog(ctx, k.chef, 0)
	require.NoError(t, err)
	require.Len(t, log, 12)
	assert.Equal(t, "confirm", log[0].Transcript)
	assert.Equal(t, "Fire table 4 mains", log[11].Transcript)
	assert.Equal(t, voice.IntentFire, log[11].Intent)
	assert.Equal(t, o.ID, log[11].Ref)
	assert.Equal(t, "Aoife Chef", log[11].UserName)
	assert.Equal(t, voice.SourceTyped, log[11].Source)
	assert.Equal(t, "Sean Floor", log[2].UserName)
	assert.Equal(t, voice.OutcomeRefused, log[2].Outcome)
}

func TestVoiceAsksRatherThanGuesses(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	for _, table := range []string{"4", "14"} {
		_, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: table, Items: []NewItem{{DishID: k.burger.ID}}})
		require.NoError(t, err)
	}
	r, err := k.svc.VoiceCommand(ctx, k.chef, "fire table 41 mains")
	require.NoError(t, err)
	assert.Equal(t, voice.OutcomeAsked, r.Outcome)
	assert.Equal(t, "Table 4 or Table 14?", r.Say)
	assert.Equal(t, []string{"Table 4", "Table 14"}, r.Options)
	assert.True(t, r.Waiting)

	st, err := k.svc.VoiceStatus(ctx, k.chef)
	require.NoError(t, err)
	assert.Equal(t, "Table 4 or Table 14?", st.Waiting)
	assert.False(t, st.Listen)

	r, err = k.svc.VoiceCommand(ctx, k.chef, "the weather")
	require.NoError(t, err)
	assert.Equal(t, "Sorry, I did not catch that. Table 4 or Table 14?", r.Say)
	r, err = k.svc.VoiceCommand(ctx, k.chef, "fourteen")
	require.NoError(t, err)
	assert.Equal(t, "Table 14 mains fired, due 10:11.", r.Say)

	// A question expires after a minute.
	_, err = k.svc.VoiceCommand(ctx, k.chef, "fire table 41")
	require.NoError(t, err)
	k.clk.Advance(2 * time.Minute)
	r, err = k.svc.VoiceCommand(ctx, k.chef, "4")
	require.NoError(t, err)
	assert.Equal(t, voice.OutcomeNotUnderstood, r.Outcome)

	// Each person has their own question.
	_, err = k.svc.VoiceCommand(ctx, k.chef, "fire table 41")
	require.NoError(t, err)
	r, err = k.svc.VoiceCommand(ctx, k.manager, "4")
	require.NoError(t, err)
	assert.Equal(t, voice.OutcomeNotUnderstood, r.Outcome)
	r, err = k.svc.VoiceCommand(ctx, k.chef, "cancel")
	require.NoError(t, err)
	assert.Equal(t, voice.OutcomeCancelled, r.Outcome)
}

func TestVoiceTemperatures(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	records := func() []haccp.Record {
		t.Helper()
		recs, err := k.svc.st.HACCP().ListRecords(ctx, store.RecordFilter{})
		require.NoError(t, err)
		return recs
	}

	r, err := k.svc.VoiceCommand(ctx, k.cook, "walk-in fridge 3.4 degrees")
	require.NoError(t, err)
	assert.Equal(t, "Walk-in fridge 3.4 degrees. Pass. Saved.", r.Say)
	require.Len(t, records(), 1)
	assert.Equal(t, r.Ref, records()[0].ID)

	// A fail asks what was done, then needs a spoken confirmation.
	r, err = k.svc.VoiceCommand(ctx, k.cook, "walk in fridge eight degrees")
	require.NoError(t, err)
	assert.Equal(t, voice.OutcomeAsked, r.Outcome)
	assert.Contains(t, r.Say, "Walk-in fridge 8 degrees: Fail.")
	assert.Contains(t, r.Say, "What did you do about it? Or say cancel.")
	r, err = k.svc.VoiceCommand(ctx, k.cook, "confirm")
	require.NoError(t, err)
	assert.Equal(t, "Say what you did about it first, or say cancel.", r.Say)
	r, err = k.svc.VoiceCommand(ctx, k.cook, "moved the stock to the cold room")
	require.NoError(t, err)
	assert.Equal(t, "Action: Moved the stock to the cold room. Say confirm to save, or cancel.", r.Say)
	assert.True(t, r.Confirm)
	assert.Len(t, records(), 1, "nothing saved before confirm")
	r, err = k.svc.VoiceCommand(ctx, k.cook, "fire table 4")
	require.NoError(t, err)
	assert.Equal(t, "Say confirm to save, or cancel.", r.Say)
	r, err = k.svc.VoiceCommand(ctx, k.cook, "confirm")
	require.NoError(t, err)
	assert.Equal(t, voice.OutcomeDone, r.Outcome)
	assert.Contains(t, r.Say, "Walk-in fridge 8 degrees. Fail:")
	recs := records()
	require.Len(t, recs, 2)
	assert.Equal(t, haccp.OutcomeFail, recs[1].Outcome)
	assert.Equal(t, "Moved the stock to the cold room", recs[1].CorrectiveAction)
	assert.Equal(t, "Liam Cook", recs[1].LoggedByName)

	// Cancel saves nothing.
	_, err = k.svc.VoiceCommand(ctx, k.cook, "chest freezer minus 5")
	require.NoError(t, err)
	r, err = k.svc.VoiceCommand(ctx, k.cook, "cancel")
	require.NoError(t, err)
	assert.Equal(t, "Cancelled. Nothing was saved.", r.Say)
	assert.Len(t, records(), 2)

	r, err = k.svc.VoiceCommand(ctx, k.cook, "core temp burger 78")
	require.NoError(t, err)
	assert.Equal(t, "Synthetic burger 78 degrees. Pass. Saved.", r.Say)
	recs = records()
	assert.Equal(t, haccp.RecordCook, recs[2].Type)
	assert.Equal(t, "Synthetic burger", recs[2].Reading.Dish)

	r, err = k.svc.VoiceCommand(ctx, k.cook, "hot hold curry 65")
	require.NoError(t, err)
	assert.Equal(t, "Curry 65 degrees. Pass. Saved.", r.Say)
}

func TestVoiceAudio(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	_, err := k.svc.VoiceAudio(ctx, k.cook, []byte("synthetic"), "audio/webm", false)
	assert.Equal(t, KindConflict, KindOf(err))
	assert.ErrorContains(t, err, "Speech-to-text is not set up")

	var gotPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPrompt = r.FormValue("prompt")
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "Walk-in fridge 2 degrees."})
	}))
	defer srv.Close()
	k.svc.speech, err = speech.New(srv.URL, "", time.Second)
	require.NoError(t, err)

	r, err := k.svc.VoiceAudio(ctx, k.cook, []byte("synthetic"), "audio/webm", false)
	require.NoError(t, err)
	assert.Equal(t, "Walk-in fridge 2 degrees.", r.Heard)
	assert.Equal(t, "Walk-in fridge 2 degrees. Pass. Saved.", r.Say)
	assert.Contains(t, gotPrompt, "Synthetic burger")
	assert.Contains(t, gotPrompt, "Walk-in fridge")
	log, err := k.svc.VoiceLog(ctx, k.cook, 1)
	require.NoError(t, err)
	assert.Equal(t, voice.SourceSpoken, log[0].Source)

	_, err = k.svc.VoiceAudio(ctx, k.cook, nil, "audio/webm", false)
	assert.ErrorContains(t, err, "No audio was recorded")
	_, err = k.svc.VoiceSpeak(ctx, k.cook, "hello")
	assert.ErrorContains(t, err, "Text-to-speech is not set up")
}

func TestVoiceHandsFree(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	heard := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": heard})
	}))
	defer srv.Close()
	var err error
	k.svc.speech, err = speech.New(srv.URL, "", time.Second)
	require.NoError(t, err)
	hear := func(text string) VoiceReply {
		t.Helper()
		heard = text
		r, err := k.svc.VoiceAudio(ctx, k.cook, []byte("synthetic"), "audio/wav", true)
		require.NoError(t, err)
		return r
	}
	logged := func() int {
		t.Helper()
		l, err := k.svc.VoiceLog(ctx, k.cook, 0)
		require.NoError(t, err)
		return len(l)
	}

	// Kitchen talk without the wake word is ignored and not kept.
	r := hear("walk in fridge 3 degrees, can you check it")
	assert.Equal(t, voice.OutcomeIgnored, r.Outcome)
	assert.Empty(t, r.Heard, "what was said is not sent back")
	assert.Zero(t, logged())

	r = hear("Kishtin, walk-in fridge 3 degrees")
	assert.Equal(t, "Walk-in fridge 3 degrees. Pass. Saved.", r.Say)
	assert.Equal(t, 1, logged())

	// The wake word alone opens a short window for the command.
	r = hear("Kishtin.")
	assert.Equal(t, "Yes?", r.Say)
	r = hear("prep fridge 4")
	assert.Equal(t, "Prep fridge 4 degrees. Pass. Saved.", r.Say)
	r = hear("prep fridge 4")
	assert.Equal(t, voice.OutcomeIgnored, r.Outcome, "the window is used once")
	hear("Kishtin")
	k.clk.Advance(10 * time.Second)
	r = hear("prep fridge 4")
	assert.Equal(t, voice.OutcomeIgnored, r.Outcome, "the window closes")

	// Answers need the wake word too.
	r = hear("Kishtin, walk-in fridge 9 degrees")
	assert.True(t, r.Waiting)
	r = hear("moved the stock")
	assert.Equal(t, voice.OutcomeIgnored, r.Outcome)
	r = hear("Kishtin, moved the stock")
	assert.Contains(t, r.Say, "Say confirm to save")
	r = hear("confirm")
	assert.Equal(t, voice.OutcomeIgnored, r.Outcome, "a stray confirm does nothing")
	r = hear("Kishtin confirm")
	assert.Equal(t, voice.OutcomeDone, r.Outcome)

	st, err := k.svc.VoiceStatus(ctx, k.cook)
	require.NoError(t, err)
	assert.Equal(t, []string{"kishtin"}, st.WakeWords)
}
