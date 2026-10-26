package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/scheduler"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
)

func TestPlanSnapshots(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()

	kept, err := k.svc.RecordPlan(ctx, "start")
	require.NoError(t, err)
	assert.True(t, kept, "the first plan is kept, even an empty one")
	kept, err = k.svc.RecordPlan(ctx, "timer")
	require.NoError(t, err)
	assert.False(t, kept, "an unchanged plan is not kept again")

	// Four burgers on a three-slot flat-top: the fourth table waits.
	for i := range 4 {
		o, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: string(rune('1' + i)), Items: []NewItem{{DishID: k.burger.ID}}})
		require.NoError(t, err)
		_, err = k.svc.FireCourse(ctx, k.chef, o.ID, menu.CourseMain)
		require.NoError(t, err)
	}
	kept, err = k.svc.RecordPlan(ctx, "order")
	require.NoError(t, err)
	assert.True(t, kept)

	// Time passes without anyone starting: courses go late.
	k.clk.Advance(5 * time.Minute)
	kept, err = k.svc.RecordPlan(ctx, "timer")
	require.NoError(t, err)
	assert.True(t, kept, "a change of status is kept")

	snaps, err := k.svc.PlanSnapshots(ctx, k.chef, time.Time{}, time.Time{}, 0)
	require.NoError(t, err)
	require.Len(t, snaps, 3)
	assert.Equal(t, "start", snaps[0].Trigger)
	assert.Empty(t, snaps[0].Plan.Courses)
	last := snaps[2].Plan
	require.Len(t, last.Courses, 4)
	causes := map[string]int{}
	for _, c := range last.Courses {
		causes[c.Cause]++
	}
	assert.Positive(t, causes[scheduler.CauseStartedLate]+causes[scheduler.CauseEquipmentFull], "late courses say why: %v", causes)
	require.NotEmpty(t, last.Stations)
	assert.NotEmpty(t, last.Stations[0].Actions[0].Reason)

	_, err = k.svc.PlanSnapshots(ctx, k.foh, time.Time{}, time.Time{}, 0)
	assert.Equal(t, KindForbidden, KindOf(err))
}

func TestActivity(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	dev := &auth.Principal{User: k.chef.User, Method: auth.MethodPIN, Device: "key-pass", DeviceName: "Pass tablet"}

	// Dishes sold out and back on, once each way.
	_, err := k.svc.SetAvailability(ctx, dev, k.burger.ID, false)
	require.NoError(t, err)
	_, err = k.svc.SetAvailability(ctx, dev, k.burger.ID, false)
	require.NoError(t, err)
	_, err = k.svc.SetAvailability(ctx, dev, k.burger.ID, true)
	require.NoError(t, err)

	// Order events name the device.
	o, err := k.svc.CreateOrder(ctx, dev, NewOrder{Kind: orders.KindTable, Table: "4", Items: []NewItem{{DishID: k.burger.ID}}})
	require.NoError(t, err)
	evs, err := k.svc.OrderEvents(ctx, k.chef, o.ID)
	require.NoError(t, err)
	assert.Equal(t, "key-pass", evs[0].Device)

	// How HACCP records were entered.
	_, err = k.svc.LogRecord(ctx, dev, fridge(3))
	require.NoError(t, err)
	_, err = k.svc.LogRecord(ctx, k.manager, fridge(3.5))
	require.NoError(t, err)
	at := k.clk.Now().Add(-time.Hour)
	_, err = k.svc.SaveCatchUp(ctx, dev, CatchUpInput{Rows: []NewRecord{{Type: haccp.RecordFridge, Reading: haccp.Reading{Unit: "Prep fridge", TempC: fp(4), TakenAt: &at}}}})
	require.NoError(t, err)

	all, err := k.svc.Activity(ctx, k.chef, ActivityQuery{})
	require.NoError(t, err)
	var kinds []string
	var sources []string
	for _, a := range all {
		kinds = append(kinds, a.Kind)
		if a.Kind == ActivityHACCPEntered {
			sources = append(sources, a.Detail["source"])
		}
	}
	assert.Equal(t, []string{ActivitySoldOut, ActivityBackOn, ActivityHACCPEntered, ActivityHACCPEntered, ActivityHACCPEntered}, kinds)
	assert.Equal(t, []string{SourceForm, SourceAPI, SourceCatchUp}, sources)
	assert.Equal(t, "Pass tablet", all[0].DeviceName)
	assert.Equal(t, "Aoife Chef", all[0].UserName)

	soldOut, err := k.svc.Activity(ctx, k.chef, ActivityQuery{Kinds: []string{ActivitySoldOut}})
	require.NoError(t, err)
	assert.Len(t, soldOut, 1)
	_, err = k.svc.Activity(ctx, k.chef, ActivityQuery{Kinds: []string{"nope"}})
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = k.svc.Activity(ctx, k.cook, ActivityQuery{})
	assert.Equal(t, KindForbidden, KindOf(err))

	// Logins and logouts.
	login, err := k.svc.LoginPIN(ctx, Caller{Key: auth.APIKey{ID: "key-pass", Name: "Pass tablet", Kind: auth.KeyDevice}}, k.chef.User.ID, "2222")
	require.NoError(t, err)
	c, err := k.svc.Authenticate(ctx, k.device, login.Token)
	require.NoError(t, err)
	require.NoError(t, k.svc.Logout(ctx, c, login.Token))
	ins, err := k.svc.Activity(ctx, k.manager, ActivityQuery{Kinds: []string{ActivityLogin, ActivityLogout}})
	require.NoError(t, err)
	require.Len(t, ins, 2)
	assert.Equal(t, "Pass tablet", ins[0].DeviceName)
	assert.Equal(t, "Pass tablet", ins[1].DeviceName, "the device comes from the key the request used")
}

func TestVoiceRecordings(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	dir := t.TempDir()
	f.svc.voiceAudioDir = dir
	v, err := f.svc.vocab(ctx)
	require.NoError(t, err)

	// Off by default: nothing is kept.
	_, err = f.svc.voiceCommand(ctx, f.cook, "walk in fridge 3.4", voice.SourceSpoken, v, []byte("synthetic audio"), "audio/webm")
	require.NoError(t, err)
	files, _ := os.ReadDir(dir)
	assert.Empty(t, files)

	sys, err := f.svc.System().current(ctx, f.svc.st)
	require.NoError(t, err)
	next := sys.Value
	next.VoiceAudioDays = 7
	_, err = f.svc.System().Save(ctx, f.manager, SettingsChange[settings.System]{Value: next, Reason: "Test speech models on our own kitchen"})
	require.NoError(t, err)

	_, err = f.svc.voiceCommand(ctx, f.cook, "walk in fridge 3.6", voice.SourceSpoken, v, []byte("synthetic audio"), "audio/webm;codecs=opus")
	require.NoError(t, err)
	log, err := f.svc.VoiceLog(ctx, f.manager, 1)
	require.NoError(t, err)
	require.NotEmpty(t, log[0].AudioFile)
	assert.Equal(t, ".webm", filepath.Ext(log[0].AudioFile))
	b, ct, err := f.svc.VoiceRecording(ctx, f.manager, log[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "synthetic audio", string(b))
	assert.Equal(t, "audio/webm", ct)
	_, _, err = f.svc.VoiceRecording(ctx, f.cook, log[0].ID)
	assert.Equal(t, KindForbidden, KindOf(err), "recordings are staff voices: managers only")

	// Kept for the days set, then deleted.
	n, err := f.svc.PurgeVoiceAudio(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
	old := f.clk.Now().AddDate(0, 0, -8)
	require.NoError(t, os.Chtimes(filepath.Join(dir, log[0].AudioFile), old, old))
	n, err = f.svc.PurgeVoiceAudio(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	_, _, err = f.svc.VoiceRecording(ctx, f.manager, log[0].ID)
	assert.ErrorContains(t, err, "deleted after the days set")
}

func TestAnonymise(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	o, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: "4", Items: []NewItem{{DishID: k.salad.ID}}})
	require.NoError(t, err)
	rec, err := k.svc.LogRecord(ctx, k.foh, fridge(3))
	require.NoError(t, err)
	key, err := k.svc.CreateKey(ctx, k.manager, auth.KeyUser, "Sean's laptop", k.foh.User.ID)
	require.NoError(t, err)

	pv, err := k.svc.PreviewAnonymise(ctx, k.manager, k.foh.User.ID)
	require.NoError(t, err)
	assert.Equal(t, "deactivate them first: only a leaver's details are anonymised", pv.Problem)
	_, err = k.svc.Anonymise(ctx, k.manager, k.foh.User.ID)
	assert.Equal(t, KindConflict, KindOf(err))

	off := false
	_, err = k.svc.UpdateUser(ctx, k.manager, k.foh.User.ID, UserChange{Active: &off})
	require.NoError(t, err)
	pv, err = k.svc.PreviewAnonymise(ctx, k.manager, k.foh.User.ID)
	require.NoError(t, err)
	assert.Empty(t, pv.Problem)
	assert.Equal(t, "Former staff 1", pv.Name)
	assert.Equal(t, store.UserMentions{OrderEvents: 1, Activity: 1, HACCPRecords: 1}, pv.Mentions)

	_, err = k.svc.Anonymise(ctx, k.chef, k.foh.User.ID)
	assert.Equal(t, KindForbidden, KindOf(err))
	res, err := k.svc.Anonymise(ctx, k.manager, k.foh.User.ID)
	require.NoError(t, err)
	assert.Equal(t, "Former staff 1", res.User.Name)
	require.NotNil(t, res.User.AnonymisedAt)

	evs, err := k.svc.OrderEvents(ctx, k.chef, o.ID)
	require.NoError(t, err)
	assert.Equal(t, "Former staff 1", evs[0].ByName)
	kept, err := k.svc.GetRecord(ctx, k.chef, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, "Sean Floor", kept.LoggedByName, "HACCP records keep the name they were logged under")
	rep, err := k.svc.Verify(ctx, k.manager)
	require.NoError(t, err)
	assert.True(t, rep.OK, "the hash chain still verifies")
	_, err = k.svc.Authenticate(ctx, key.Secret, "")
	assert.Equal(t, KindUnauthenticated, KindOf(err), "their keys are revoked")
	u, err := k.svc.st.Users().GetUser(ctx, k.foh.User.ID)
	require.NoError(t, err)
	assert.Empty(t, u.PINHash)

	pv, err = k.svc.PreviewAnonymise(ctx, k.manager, k.foh.User.ID)
	require.NoError(t, err)
	assert.Equal(t, "already anonymised", pv.Problem)
}
