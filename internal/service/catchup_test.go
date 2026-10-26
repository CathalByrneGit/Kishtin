package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

func setPermissions(t *testing.T, f *fixture, grants ...auth.Grant) {
	t.Helper()
	_, err := f.svc.Permissions().Save(context.Background(), f.manager, SettingsChange[auth.Permissions]{
		Value: auth.Permissions{Grants: grants}, Reason: "Synthetic delegation",
	})
	require.NoError(t, err)
}

func TestPermissions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	chef, err := f.svc.CreateUser(ctx, f.manager, NewUser{Name: "Aoife Chef", Role: auth.RoleHeadChef, PIN: "2222"})
	require.NoError(t, err)
	chefP := &auth.Principal{User: chef, Method: auth.MethodPIN}

	assert.Equal(t, AllowedNo, f.svc.Can(chefP)[auth.ActSignOff])
	_, err = f.svc.SignOff(ctx, chefP, SignOffInput{})
	assert.Equal(t, KindForbidden, KindOf(err))

	pv, err := f.svc.Permissions().Preview(ctx, f.manager, SettingsChange[auth.Permissions]{
		Value: auth.Permissions{Grants: []auth.Grant{{Action: auth.ActSignOff, Roles: []auth.Role{auth.RoleHeadChef}, PIN: true}}}, Reason: "Head chef signs off",
	})
	require.NoError(t, err)
	assert.Empty(t, pv.Problem)
	assert.Equal(t, []string{"Sign off records: managers, head chefs, PIN is enough"}, pv.Changes)

	// Only a manager with a password changes permissions.
	_, err = f.svc.Permissions().Save(ctx, chefP, SettingsChange[auth.Permissions]{Value: auth.Permissions{}, Reason: "x"})
	assert.Equal(t, KindForbidden, KindOf(err))

	setPermissions(t, f, auth.Grant{Action: auth.ActSignOff, Roles: []auth.Role{auth.RoleHeadChef}, PIN: true})
	assert.Equal(t, AllowedYes, f.svc.Can(chefP)[auth.ActSignOff])
	assert.Equal(t, AllowedNo, f.svc.Can(f.cook)[auth.ActSignOff])

	// A named member of staff, needing a password they do not have.
	pv, err = f.svc.Permissions().Preview(ctx, f.manager, SettingsChange[auth.Permissions]{
		Value: auth.Permissions{Grants: []auth.Grant{{Action: auth.ActInspect, Roles: []auth.Role{auth.RoleHeadChef}, Users: []string{f.cook.User.ID}}}}, Reason: "Liam does inspections",
	})
	require.NoError(t, err)
	assert.Contains(t, pv.Notes, "Liam Cook has no password, so cannot log hygiene inspections and training, import paper pages until a manager sets one in Setup › Staff, or PIN is made enough.")
	setPermissions(t, f, auth.Grant{Action: auth.ActInspect, Roles: []auth.Role{auth.RoleHeadChef}, Users: []string{f.cook.User.ID}, PIN: true})
	assert.Equal(t, AllowedYes, f.svc.Can(f.cook)[auth.ActInspect])
	assert.Equal(t, AllowedNo, f.svc.Can(chefP)[auth.ActSignOff], "saving a new version replaces the old grants")

	// Fixed actions and unknown staff are refused.
	pv, err = f.svc.Permissions().Preview(ctx, f.manager, SettingsChange[auth.Permissions]{
		Value: auth.Permissions{Grants: []auth.Grant{{Action: auth.ActManageUsers, Roles: []auth.Role{auth.RoleHeadChef}}}}, Reason: "x",
	})
	require.NoError(t, err)
	assert.Contains(t, pv.Problem, "always managers with a password")
	_, err = f.svc.Permissions().Preview(ctx, f.manager, SettingsChange[auth.Permissions]{
		Value: auth.Permissions{Grants: []auth.Grant{{Action: auth.ActInspect, Users: []string{"nobody"}}}}, Reason: "x",
	})
	assert.Equal(t, KindInvalid, KindOf(err))

	// A grant the same as the default is dropped.
	pv, err = f.svc.Permissions().Preview(ctx, f.manager, SettingsChange[auth.Permissions]{
		Value: auth.Permissions{Grants: []auth.Grant{{Action: auth.ActLogHACCP, Roles: []auth.Role{auth.RoleLineCook, auth.RoleHeadChef, auth.RoleFrontOfHouse}, PIN: true}}}, Reason: "x",
	})
	require.NoError(t, err)
	assert.Empty(t, pv.Proposed.Grants)

	// Permissions survive a restart.
	require.NoError(t, f.svc.Init(ctx))
	assert.Equal(t, AllowedYes, f.svc.Can(f.cook)[auth.ActInspect])
}

func TestBackdatedRecords(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.clk.Advance(48 * time.Hour) // 3 May, 09:00 UTC
	yesterday := time.Date(2026, 5, 2, 9, 30, 0, 0, time.UTC)

	r := fridge(3.4)
	r.Reading.TakenAt = &yesterday
	r.Reading.SignedBy = "Aoife"
	rec, err := f.svc.LogRecord(ctx, f.cook, r)
	require.NoError(t, err)
	assert.True(t, rec.TakenAt().Equal(yesterday))
	assert.Equal(t, f.clk.Now(), rec.LoggedAt)

	day, err := f.svc.ListRecords(ctx, f.manager, RecordQuery{From: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	require.Len(t, day, 1, "records are listed by when the check was done")
	today, err := f.svc.Today(ctx, f.cook)
	require.NoError(t, err)
	assert.Equal(t, 0, today.Done, "a check done yesterday does not count today")

	future := f.clk.Now().Add(time.Hour)
	r.Reading.TakenAt = &future
	_, err = f.svc.LogRecord(ctx, f.cook, r)
	assert.ErrorContains(t, err, "cannot be in the future")

	// A correction keeps the original's time.
	c := fridge(3.8)
	c.Corrects, c.CorrectionReason = rec.ID, "Misread"
	cr, err := f.svc.LogRecord(ctx, f.cook, c)
	require.NoError(t, err)
	assert.True(t, cr.TakenAt().Equal(yesterday))

	// The restaurant can keep backdating to managers.
	setPermissions(t, f, auth.Grant{Action: auth.ActBackdate})
	r.Reading.TakenAt = &yesterday
	_, err = f.svc.LogRecord(ctx, f.cook, r)
	assert.Equal(t, KindForbidden, KindOf(err))
	recent := f.clk.Now().Add(-10 * time.Minute)
	r.Reading.TakenAt = &recent
	_, err = f.svc.LogRecord(ctx, f.cook, r)
	require.NoError(t, err, "a few minutes ago is not logging later")

	// Signed-off periods.
	setPermissions(t, f)
	_, err = f.svc.SignOff(ctx, f.manager, SignOffInput{From: "2026-05-01", To: "2026-05-02", Note: "Synthetic"})
	require.NoError(t, err)
	r.Reading.TakenAt = &yesterday
	_, err = f.svc.LogRecord(ctx, f.cook, r)
	assert.Equal(t, KindForbidden, KindOf(err))
	assert.ErrorContains(t, err, "is in a period Máire Manager signed off (2026-05-01 to 2026-05-02)")
	setPermissions(t, f, auth.Grant{Action: auth.ActLogSignedOff, Roles: []auth.Role{auth.RoleLineCook}, PIN: true})
	_, err = f.svc.LogRecord(ctx, f.cook, r)
	require.NoError(t, err)
}

func TestCatchUp(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.svc.LogRecord(ctx, f.cook, fridge(3)) // records start 1 May, 10:00 Dublin
	require.NoError(t, err)
	f.clk.Advance(48 * time.Hour)

	cu, err := f.svc.CatchUpGaps(ctx, f.cook, "", "")
	require.NoError(t, err)
	assert.Equal(t, "2026-05-01", cu.Since)
	var walkIn2PM *CatchUpGap
	cleaning := 0
	for i, g := range cu.Gaps {
		if g.Date == "2026-05-02" && g.Unit == "Walk-in fridge" && g.Check == "PM" {
			walkIn2PM = &cu.Gaps[i]
		}
		if g.Type == haccp.RecordCleaning {
			cleaning++
		}
	}
	require.NotNil(t, walkIn2PM)
	assert.Equal(t, "12:00", walkIn2PM.Time)
	assert.Equal(t, haccp.RecordFridge, walkIn2PM.Type)
	assert.Equal(t, 12, cleaning, "6 tasks on each of 1 and 2 May; today is not over")

	at := time.Date(2026, 5, 2, 14, 0, 0, 0, time.UTC)
	done := true
	rows := []NewRecord{
		{Type: haccp.RecordFridge, Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: fp(4), TakenAt: &at}},
		{Type: haccp.RecordCleaning, Reading: haccp.Reading{Task: "Floors swept & mopped", Done: &done, TakenAt: &at}},
		{Type: haccp.RecordFridge, Reading: haccp.Reading{Unit: "Prep fridge", TempC: fp(9), TakenAt: &at}},
	}
	checks, err := f.svc.PreviewCatchUp(ctx, f.cook, CatchUpInput{Rows: rows})
	require.NoError(t, err)
	assert.Empty(t, checks[0].Problem)
	assert.Contains(t, checks[2].Problem, "corrective action")
	_, err = f.svc.SaveCatchUp(ctx, f.cook, CatchUpInput{Rows: rows})
	assert.ErrorContains(t, err, "row 3")
	rows[2].CorrectiveAction = "Moved food to a working unit"
	saved, err := f.svc.SaveCatchUp(ctx, f.cook, CatchUpInput{Rows: rows})
	require.NoError(t, err)
	require.Len(t, saved, 3)

	after, err := f.svc.CatchUpGaps(ctx, f.cook, "", "")
	require.NoError(t, err)
	assert.Len(t, after.Gaps, len(cu.Gaps)-3)

	_, err = f.svc.SaveCatchUp(ctx, f.cook, CatchUpInput{Rows: []NewRecord{{Type: haccp.RecordInspection, Reading: haccp.Reading{TakenAt: &at}}}})
	assert.ErrorContains(t, err, "cannot be caught up here")
}
