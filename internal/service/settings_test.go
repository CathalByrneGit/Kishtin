package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
)

func TestBuiltinSettings(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	g, err := f.svc.Guidance().Current(ctx, f.cook)
	require.NoError(t, err)
	assert.True(t, g.Builtin)
	assert.Equal(t, haccp.BuiltinGuidanceID, g.Version.ID)
	assert.Equal(t, "FSAI", g.Value.Name)

	r, err := f.svc.Rules().Current(ctx, f.cook)
	require.NoError(t, err)
	assert.Equal(t, haccp.DefaultRules(), r.Value)

	sys, err := f.svc.System().Current(ctx, f.cook)
	require.NoError(t, err)
	assert.True(t, sys.Builtin)
	assert.Equal(t, 2, sys.Value.BackupKeep, "system settings start from kishtin.toml")

	hist, err := f.svc.Lists().History(ctx, f.cook)
	require.NoError(t, err)
	require.Len(t, hist, 1, "the built-in lists are stored once")
}

func TestSettingsNeedManagerPassword(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ch := SettingsChange[haccp.Rules]{Value: haccp.Rules{BorderlineNeedsAction: true}, Reason: "x"}
	_, err := f.svc.Rules().Save(ctx, f.cook, ch)
	assert.Equal(t, KindForbidden, KindOf(err))
	pinManager := &auth.Principal{User: f.manager.User, Method: auth.MethodPIN}
	_, err = f.svc.Rules().Preview(ctx, pinManager, ch)
	var se *Error
	require.ErrorAs(t, err, &se)
	assert.True(t, se.NeedPassword)
	_, err = f.svc.System().Save(ctx, pinManager, SettingsChange[settings.System]{})
	require.ErrorAs(t, err, &se)
	assert.True(t, se.NeedPassword)
}

func TestRulesBorderline(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	delivery := NewRecord{Type: haccp.RecordDelivery, Reading: haccp.Reading{
		Supplier: "Synthetic Dairy Ltd", Product: "Cream", DeliveryKind: haccp.DeliveryChilled, TempC: fp(6.5),
	}}
	_, err := f.svc.LogRecord(ctx, f.cook, delivery)
	assert.Equal(t, KindInvalid, KindOf(err), "borderline needs an action by default")

	off := SettingsChange[haccp.Rules]{Value: haccp.Rules{BorderlineNeedsAction: false, ListedNamesOnly: true}, Reason: "Plan allows it"}
	pv, err := f.svc.Rules().Preview(ctx, f.manager, off)
	require.NoError(t, err)
	assert.Equal(t, []string{"borderline_needs_action: true → false"}, pv.Changes)
	assert.NotEmpty(t, pv.LessStrict)
	assert.Contains(t, pv.Problem, "less strict")
	_, err = f.svc.Rules().Save(ctx, f.manager, off)
	assert.Equal(t, KindInvalid, KindOf(err))

	off.PlanRef = "HACCP plan 5.2 (synthetic)"
	saved, err := f.svc.Rules().Save(ctx, f.manager, off)
	require.NoError(t, err)
	assert.Equal(t, haccp.BuiltinRulesID, saved.Version.Supersedes)

	rec, err := f.svc.LogRecord(ctx, f.cook, delivery)
	require.NoError(t, err)
	assert.Equal(t, haccp.OutcomeBorderline, rec.Outcome)
	assert.Equal(t, saved.Version.ID, rec.RulesVersionID, "the record names the rules that let it through")

	fail := delivery
	fail.Reading.TempC = fp(9)
	_, err = f.svc.LogRecord(ctx, f.cook, fail)
	assert.Equal(t, KindInvalid, KindOf(err), "a fail still needs an action")

	_, err = f.svc.Rules().Save(ctx, f.manager, off)
	assert.Equal(t, KindInvalid, KindOf(err), "saving the same value is refused")

	hist, err := f.svc.Rules().History(ctx, f.manager)
	require.NoError(t, err)
	assert.Len(t, hist, 2)
}

func TestListsAndListedNames(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.svc.LogRecord(ctx, f.cook, fridge(3.0)) // "Walk-in"
	require.NoError(t, err)
	walkin := fridge(3.0)
	walkin.Reading.Unit = "Walkin"
	_, err = f.svc.LogRecord(ctx, f.cook, walkin)
	require.NoError(t, err, "any name is accepted while the list is empty")

	lists := haccp.Lists{
		Units:          []haccp.Unit{{Name: "Walk-in", Kind: haccp.RecordFridge}, {Name: "Fish fridge", Kind: haccp.RecordFridge}},
		FoodCategories: []string{"poultry"},
	}
	pv, err := f.svc.Lists().Preview(ctx, f.manager, SettingsChange[haccp.Lists]{Value: lists, Reason: "Name our fridges"})
	require.NoError(t, err)
	assert.Empty(t, pv.Problem)
	assert.Contains(t, pv.Changes, `units[Fish fridge].name: added "Fish fridge"`)
	assert.Contains(t, pv.Notes[len(pv.Notes)-1], "Walkin", "the preview warns about names in use that are not listed")

	_, err = f.svc.Lists().Save(ctx, f.manager, SettingsChange[haccp.Lists]{Value: lists, Reason: "Name our fridges"})
	require.NoError(t, err)

	_, err = f.svc.LogRecord(ctx, f.cook, walkin)
	require.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "not on the fridge list")

	lower := fridge(3.0)
	lower.Reading.Unit = "walk-IN"
	rec, err := f.svc.LogRecord(ctx, f.cook, lower)
	require.NoError(t, err)
	assert.Equal(t, "Walk-in", rec.Reading.Unit, "the listed spelling is saved")

	_, err = f.svc.CreateLimit(ctx, f.manager, NewLimit{Check: haccp.CheckFridge, Scope: haccp.Scope{Kind: haccp.ScopeUnit, Ref: "Bar fridge"}, MaxC: fp(4), Reason: "x"})
	assert.Equal(t, KindInvalid, KindOf(err), "limit scopes use listed names too")
	lv, err := f.svc.CreateLimit(ctx, f.manager, NewLimit{Check: haccp.CheckFridge, Scope: haccp.Scope{Kind: haccp.ScopeUnit, Ref: "fish FRIDGE"}, MaxC: fp(2), Reason: "Fish"})
	require.NoError(t, err)
	assert.Equal(t, "Fish fridge", lv.Scope.Ref)

	bad := SettingsChange[haccp.Lists]{Value: haccp.Lists{FoodCategories: []string{"fish", "Fish"}}, Reason: "x"}
	_, err = f.svc.Lists().Save(ctx, f.manager, bad)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestCustomGuidance(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	limit := NewLimit{
		Check: haccp.CheckCook, Scope: haccp.Scope{Kind: haccp.ScopeCategory, Ref: "poultry"},
		Combos: []haccp.TimeTemp{{MinC: 75}, {MinC: 70, HoldSeconds: 120}}, Reason: "Equivalent",
	}
	_, err := f.svc.CreateLimit(ctx, f.manager, limit)
	assert.Equal(t, KindInvalid, KindOf(err), "70°C for 2m is below the FSAI guidance")

	g := haccp.FSAIGuidance()
	g.Name = "Our HACCP plan (synthetic)"
	for k := range g.Checks {
		if g.Checks[k].Check == haccp.CheckCook {
			g.Checks[k].Combos = append(g.Checks[k].Combos, haccp.TimeTemp{MinC: 70, HoldSeconds: 120})
		}
	}
	ch := SettingsChange[haccp.Guidance]{Value: g, Reason: "Recognise 70°C for 2 minutes"}
	pv, err := f.svc.Guidance().Preview(ctx, f.manager, ch)
	require.NoError(t, err)
	assert.Equal(t, []string{"cook: core 70.0°C for 2m is not one of the FSAI combinations"}, pv.LessStrict)
	assert.Contains(t, pv.Problem, "plan section")
	assert.Contains(t, pv.Changes, `name: "FSAI" → "Our HACCP plan (synthetic)"`)

	ch.PlanRef = "HACCP plan 4.1 (synthetic)"
	gv, err := f.svc.Guidance().Save(ctx, f.manager, ch)
	require.NoError(t, err)

	lpv, err := f.svc.PreviewLimit(ctx, f.manager, limit)
	require.NoError(t, err)
	assert.Empty(t, lpv.Problem)
	assert.Empty(t, lpv.BelowGuidance)
	assert.NotEmpty(t, lpv.BelowFSAI, "the preview still says it is below FSAI")
	assert.Equal(t, "Our HACCP plan (synthetic)", lpv.Guidance)

	lv, err := f.svc.CreateLimit(ctx, f.manager, limit)
	require.NoError(t, err)
	assert.Equal(t, gv.Version.ID, lv.GuidanceVersionID)

	// Tightening the guidance again flags the limit, but never changes it.
	pv, err = f.svc.Guidance().Preview(ctx, f.manager, SettingsChange[haccp.Guidance]{Value: haccp.FSAIGuidance(), Reason: "Back to FSAI"})
	require.NoError(t, err)
	assert.Contains(t, pv.Notes, "The cook limit (category:poultry) would be below the new guidance: core 70.0°C for 2m is not one of the FSAI combinations.")

	bad := haccp.FSAIGuidance()
	bad.Checks = bad.Checks[:3]
	_, err = f.svc.Guidance().Save(ctx, f.manager, SettingsChange[haccp.Guidance]{Value: bad, Reason: "x"})
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestSystemSettings(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sys := settings.System{BackupIntervalMinutes: 60, BackupKeep: 1, MaxUploadMB: 1, PreviewRecords: 10}
	pv, err := f.svc.System().Preview(ctx, f.manager, SettingsChange[settings.System]{Value: sys, Reason: "Hourly backups"})
	require.NoError(t, err)
	assert.Contains(t, pv.Changes, "backup_interval_minutes: 0 → 60")
	saved, err := f.svc.System().Save(ctx, f.manager, SettingsChange[settings.System]{Value: sys, Reason: "Hourly backups"})
	require.NoError(t, err)
	assert.Empty(t, saved.Version.Supersedes, "the kishtin.toml default is not a stored version")

	for range 2 {
		_, err := f.svc.BackupNow(ctx, f.manager)
		require.NoError(t, err)
		f.clk.Advance(time.Hour)
	}
	list, err := f.svc.ListBackups(ctx, f.manager)
	require.NoError(t, err)
	assert.Len(t, list, 1, "backup_keep from the saved settings applies")

	sys.PreviewRecords = 0
	_, err = f.svc.System().Save(ctx, f.manager, SettingsChange[settings.System]{Value: sys, Reason: "x"})
	assert.Equal(t, KindInvalid, KindOf(err))
}
