package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite"
)

func TestSetupProgress(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	done := func() map[string]bool {
		t.Helper()
		p, err := f.svc.SetupProgress(ctx, f.manager)
		require.NoError(t, err)
		out := map[string]bool{}
		for _, s := range p.Steps {
			out[s.ID] = s.Done
		}
		return out
	}
	_, err := f.svc.SetupProgress(ctx, f.cook)
	assert.Equal(t, KindForbidden, KindOf(err), "managers only")

	got := done()
	// The fixture was set up from the command line, with a manager, a cook
	// and a tablet's device key.
	assert.Equal(t, map[string]bool{
		"name": false, "staff": true, "second_manager": false, "lists": false, "tablet": true,
		"first_check": false, "startup": true, "menu": false, "allergens": false,
	}, got)

	_, err = f.svc.System().Save(ctx, f.manager, SettingsChange[settings.System]{
		Value: settings.System{RestaurantName: "The Synthetic Kitchen", BackupKeep: 2, MaxUploadMB: 1, PreviewRecords: 50}, Reason: "Name"})
	require.NoError(t, err)
	_, err = f.svc.CreateUser(ctx, f.manager, NewUser{Name: "Aoife Chef", Role: auth.RoleManager, Password: "synthetic-password"})
	require.NoError(t, err)
	lists, err := f.svc.Lists().Current(ctx, f.manager)
	require.NoError(t, err)
	next := lists.Value
	next.CleaningTasks = append(next.CleaningTasks, haccp.CleaningTask{Task: "Clean extractor filters", EveryDays: 28})
	_, err = f.svc.Lists().Save(ctx, f.manager, SettingsChange[haccp.Lists]{Value: next, Reason: "Our own tasks"})
	require.NoError(t, err)
	_, err = f.svc.LogRecord(ctx, f.cook, fridge(3.4))
	require.NoError(t, err)
	st, err := f.svc.CreateStation(ctx, f.manager, StationInput{Name: "Grill"})
	require.NoError(t, err)
	_, err = f.svc.CreateDish(ctx, f.manager, DishInput{Name: "Burger", Course: menu.CourseMain, StationID: st.ID, CookSeconds: 480,
		Allergens: &[]string{"gluten"}, AllergensChecked: new(true)})
	require.NoError(t, err)

	for id, ok := range done() {
		assert.True(t, ok, id)
	}
	p, err := f.svc.SetupProgress(ctx, f.manager)
	require.NoError(t, err)
	assert.Equal(t, p.Total, p.Done)
}

func TestSetupProgressAfterSetupFromTheScreens(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "kishtin.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	svc := New(Options{Store: st, Clock: clock.NewFixed(start)})
	require.NoError(t, svc.Init(ctx))
	res, err := svc.SetupFromScreen(ctx, FirstRun{Restaurant: "The Synthetic Kitchen", Name: "Máire Manager", Password: "synthetic-password", Device: "Office computer"})
	require.NoError(t, err)
	mgr := &auth.Principal{User: res.User, Method: auth.MethodPassword}

	tablet := func() bool {
		t.Helper()
		p, err := svc.SetupProgress(ctx, mgr)
		require.NoError(t, err)
		for _, s := range p.Steps {
			if s.ID == "tablet" {
				return s.Done
			}
		}
		t.Fatal("no tablet step")
		return false
	}
	assert.False(t, tablet(), "the computer that did the setup is not a tablet")
	_, err = svc.CreateKey(ctx, mgr, auth.KeyDevice, "Pass tablet", "")
	require.NoError(t, err)
	assert.True(t, tablet())
}
