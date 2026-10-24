package haccp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFSAIDefaultsAreValid(t *testing.T) {
	for _, lv := range FSAIDefaults() {
		t.Run(string(lv.Check), func(t *testing.T) {
			require.NoError(t, lv.ValidateShape())
			assert.Empty(t, lv.LooserThanFSAI())
			require.NoError(t, lv.ValidateNew())
		})
	}
}

func TestValidateShape(t *testing.T) {
	fridge := func(mut func(*LimitVersion)) LimitVersion {
		lv := FSAIDefault(CheckFridge)
		mut(&lv)
		return lv
	}
	bad := map[string]LimitVersion{
		"unknown check":         fridge(func(l *LimitVersion) { l.Check = "nope" }),
		"default with ref":      fridge(func(l *LimitVersion) { l.Scope.Ref = "x" }),
		"unit without ref":      fridge(func(l *LimitVersion) { l.Scope = Scope{Kind: ScopeUnit} }),
		"missing max":           fridge(func(l *LimitVersion) { l.MaxC = nil }),
		"extra min":             fridge(func(l *LimitVersion) { l.MinC = f(1) }),
		"non-food on default":   fridge(func(l *LimitVersion) { l.NonFood = true }),
		"unit limit on cooking": {Check: CheckCook, Scope: Scope{Kind: ScopeUnit, Ref: "Oven"}, Combos: []TimeTemp{{MinC: 75}}},
		"borderline below max":  {Check: CheckChilledDelivery, Scope: Scope{Kind: ScopeDefault}, MaxC: f(5), BorderlineMaxC: f(4)},
		"negative hold":         {Check: CheckCook, Scope: Scope{Kind: ScopeDefault}, Combos: []TimeTemp{{MinC: 75, HoldSeconds: -1}}},
		"cooling cutoff early": {Check: CheckCooling, Scope: Scope{Kind: ScopeDefault}, Cooling: &CoolingLimit{
			IntoFridgeWithinSeconds: 7200, CutoffAfterSeconds: 3600, CutoffMaxC: 10}},
	}
	for name, lv := range bad {
		t.Run(name, func(t *testing.T) { assert.Error(t, lv.ValidateShape()) })
	}
}

func TestLooserNeedsReasonAndPlanRef(t *testing.T) {
	stricter := FSAIDefault(CheckFridge)
	stricter.ID, stricter.Scope = "fish", Scope{Kind: ScopeUnit, Ref: "Fish fridge"}
	stricter.MaxC = f(2)
	stricter.Reason = "Fish kept at 0–2°C"
	assert.Empty(t, stricter.LooserThanFSAI())
	assert.NoError(t, stricter.ValidateNew())

	stricter.Reason = ""
	assert.Error(t, stricter.ValidateNew(), "every change needs a reason")

	burger := LimitVersion{
		Check: CheckCook, Scope: Scope{Kind: ScopeDish, Ref: "Pink burger"},
		Combos: []TimeTemp{{MinC: 75}, {MinC: 60, HoldSeconds: 600}},
		Reason: "Pink burger exception",
	}
	assert.NotEmpty(t, burger.LooserThanFSAI())
	assert.ErrorContains(t, burger.ValidateNew(), "below FSAI guidance")
	burger.PlanRef = "HACCP plan section 4.2, validation SYN-001"
	assert.NoError(t, burger.ValidateNew())

	drinks := FSAIDefault(CheckFridge)
	drinks.Scope, drinks.NonFood = Scope{Kind: ScopeUnit, Ref: "Drinks fridge"}, true
	drinks.MaxC, drinks.Reason = f(8), "Drinks only"
	assert.NotEmpty(t, drinks.LooserThanFSAI())
	assert.NoError(t, drinks.ValidateNew(), "a non-food unit needs a reason but no plan reference")
}

func TestLooserThanFSAIEachField(t *testing.T) {
	cases := map[string]LimitVersion{
		"fridge max":     {Check: CheckFridge, MaxC: f(5.1)},
		"borderline":     {Check: CheckChilledDelivery, MaxC: f(5), BorderlineMaxC: f(8.1)},
		"hot hold min":   {Check: CheckHotHold, MinC: f(62.9)},
		"reheat combo":   {Check: CheckReheat, Combos: []TimeTemp{{MinC: 69.9}}},
		"cooling window": {Check: CheckCooling, Cooling: &CoolingLimit{IntoFridgeWithinSeconds: 7201, CutoffAfterSeconds: 9000, CutoffMaxC: 10}},
		"cooling cutoff": {Check: CheckCooling, Cooling: &CoolingLimit{IntoFridgeWithinSeconds: 7200, CutoffAfterSeconds: 9001, CutoffMaxC: 10}},
		"cooling temp":   {Check: CheckCooling, Cooling: &CoolingLimit{IntoFridgeWithinSeconds: 7200, CutoffAfterSeconds: 9000, CutoffMaxC: 10.1}},
	}
	for name, lv := range cases {
		t.Run(name, func(t *testing.T) { assert.Len(t, lv.LooserThanFSAI(), 1) })
	}
	onLimit := LimitVersion{Check: CheckFridge, MaxC: f(5.0)}
	assert.Empty(t, onLimit.LooserThanFSAI())
}

func TestResolve(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	def := FSAIDefault(CheckFridge)
	unitV1 := LimitVersion{ID: "u1", Check: CheckFridge, Scope: Scope{Kind: ScopeUnit, Ref: "Fish fridge"}, MaxC: f(3), CreatedAt: t0}
	unitV2 := LimitVersion{ID: "u2", Check: CheckFridge, Scope: Scope{Kind: ScopeUnit, Ref: "Fish fridge"}, MaxC: f(2), CreatedAt: t0.Add(time.Hour), Supersedes: "u1"}
	cook := FSAIDefault(CheckCook)
	poultry := LimitVersion{ID: "p1", Check: CheckCook, Scope: Scope{Kind: ScopeCategory, Ref: "poultry"}, Combos: []TimeTemp{{MinC: 75}}, CreatedAt: t0}
	dish := LimitVersion{ID: "d1", Check: CheckCook, Scope: Scope{Kind: ScopeDish, Ref: "Chicken wings"}, Combos: []TimeTemp{{MinC: 80}}, CreatedAt: t0}
	history := []LimitVersion{def, unitV1, unitV2, cook, poultry, dish}

	fish := Resolve(history, CheckFridge, []Scope{{Kind: ScopeUnit, Ref: "Fish fridge"}})
	assert.Equal(t, "u2", fish.ID, "the newest version of the unit limit applies")

	walkIn := Resolve(history, CheckFridge, []Scope{{Kind: ScopeUnit, Ref: "Walk-in"}})
	assert.Equal(t, def.ID, walkIn.ID)

	wings := Resolve(history, CheckCook, Scopes(RecordCook, Reading{Dish: "Chicken wings", FoodCategory: "poultry"}))
	assert.Equal(t, "d1", wings.ID, "dish beats category")
	thighs := Resolve(history, CheckCook, Scopes(RecordCook, Reading{Dish: "Thighs", FoodCategory: "poultry"}))
	assert.Equal(t, "p1", thighs.ID, "category beats default")

	empty := Resolve(nil, CheckHotHold, nil)
	assert.Equal(t, FSAIDefaultID(CheckHotHold), empty.ID, "falls back to the FSAI constant")
}
