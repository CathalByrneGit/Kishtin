package haccp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFSAIDefaultsAreValid(t *testing.T) {
	g := FSAIGuidance()
	for _, lv := range FSAIDefaults() {
		t.Run(string(lv.Check), func(t *testing.T) {
			require.NoError(t, lv.ValidateShape())
			assert.Empty(t, lv.LooserThanFSAI())
			assert.Empty(t, lv.LooserThanGuidance(g))
			require.NoError(t, lv.ValidateNew(g))
		})
	}
}

func TestValidateShape(t *testing.T) {
	fridge := func(mut func(*LimitVersion)) LimitVersion {
		lv := FSAIDefault(CheckFridge)
		mut(&lv)
		return lv
	}
	def := Scope{Kind: ScopeDefault}
	bad := map[string]LimitVersion{
		"unknown check":         fridge(func(l *LimitVersion) { l.Check = "nope" }),
		"default with ref":      fridge(func(l *LimitVersion) { l.Scope.Ref = "x" }),
		"unit without ref":      fridge(func(l *LimitVersion) { l.Scope = Scope{Kind: ScopeUnit} }),
		"missing max":           fridge(func(l *LimitVersion) { l.MaxC = nil }),
		"extra min":             fridge(func(l *LimitVersion) { l.MinC = f(1) }),
		"non-food on default":   fridge(func(l *LimitVersion) { l.NonFood = true }),
		"unit limit on cooking": {Check: CheckCook, Scope: Scope{Kind: ScopeUnit, Ref: "Oven"}, Thresholds: Thresholds{Combos: []TimeTemp{{MinC: 75}}}},
		"borderline below max":  {Check: CheckChilledDelivery, Scope: def, Thresholds: Thresholds{MaxC: f(5), BorderlineMaxC: f(4)}},
		"negative hold":         {Check: CheckCook, Scope: def, Thresholds: Thresholds{Combos: []TimeTemp{{MinC: 75, HoldSeconds: -1}}}},
		"cooling cutoff early": {Check: CheckCooling, Scope: def, Thresholds: Thresholds{Cooling: &CoolingLimit{
			IntoFridgeWithinSeconds: 7200, CutoffAfterSeconds: 3600, CutoffMaxC: 10}}},
	}
	for name, lv := range bad {
		t.Run(name, func(t *testing.T) { assert.Error(t, lv.ValidateShape()) })
	}
}

func TestLooserNeedsReasonAndPlanRef(t *testing.T) {
	g := FSAIGuidance()
	stricter := FSAIDefault(CheckFridge)
	stricter.ID, stricter.Scope = "fish", Scope{Kind: ScopeUnit, Ref: "Fish fridge"}
	stricter.MaxC = f(2)
	stricter.Reason = "Fish kept at 0–2°C"
	assert.Empty(t, stricter.LooserThanFSAI())
	assert.NoError(t, stricter.ValidateNew(g))

	stricter.Reason = ""
	assert.Error(t, stricter.ValidateNew(g), "every change needs a reason")

	burger := LimitVersion{
		Check: CheckCook, Scope: Scope{Kind: ScopeDish, Ref: "Pink burger"},
		Thresholds: Thresholds{Combos: []TimeTemp{{MinC: 75}, {MinC: 60, HoldSeconds: 600}}},
		Reason:     "Pink burger exception",
	}
	assert.NotEmpty(t, burger.LooserThanFSAI())
	assert.ErrorContains(t, burger.ValidateNew(g), "below your reference guidance, FSAI")
	burger.PlanRef = "HACCP plan section 4.2, validation SYN-001"
	assert.NoError(t, burger.ValidateNew(g))

	drinks := FSAIDefault(CheckFridge)
	drinks.Scope, drinks.NonFood = Scope{Kind: ScopeUnit, Ref: "Drinks fridge"}, true
	drinks.MaxC, drinks.Reason = f(8), "Drinks only"
	assert.NotEmpty(t, drinks.LooserThanFSAI())
	assert.NoError(t, drinks.ValidateNew(g), "a non-food unit needs a reason but no plan reference")
}

func TestLimitMeasuredAgainstCustomGuidance(t *testing.T) {
	g := FSAIGuidance()
	g.Name = "Our HACCP plan"
	for k := range g.Checks {
		if g.Checks[k].Check == CheckCook {
			g.Checks[k].Combos = []TimeTemp{{MinC: 75}, {MinC: 70, HoldSeconds: 120}}
		}
	}
	lv := LimitVersion{
		Check: CheckCook, Scope: Scope{Kind: ScopeCategory, Ref: "poultry"},
		Thresholds: Thresholds{Combos: []TimeTemp{{MinC: 75}, {MinC: 70, HoldSeconds: 180}}},
		Reason:     "Equivalent",
	}
	assert.NotEmpty(t, lv.LooserThanFSAI(), "70°C is still below the FSAI constant")
	assert.Empty(t, lv.LooserThanGuidance(g), "70°C for 3m is covered by the guidance's 70°C for 2m")
	assert.NoError(t, lv.ValidateNew(g), "no plan reference needed once the guidance recognises it")

	lv.Combos = []TimeTemp{{MinC: 70, HoldSeconds: 60}}
	assert.ErrorContains(t, lv.ValidateNew(g), "Our HACCP plan")
}

func TestLooserThanEachField(t *testing.T) {
	cases := map[string]LimitVersion{
		"fridge max":     {Check: CheckFridge, Thresholds: Thresholds{MaxC: f(5.1)}},
		"borderline":     {Check: CheckChilledDelivery, Thresholds: Thresholds{MaxC: f(5), BorderlineMaxC: f(8.1)}},
		"hot hold min":   {Check: CheckHotHold, Thresholds: Thresholds{MinC: f(62.9)}},
		"reheat combo":   {Check: CheckReheat, Thresholds: Thresholds{Combos: []TimeTemp{{MinC: 69.9}}}},
		"cooling window": {Check: CheckCooling, Thresholds: Thresholds{Cooling: &CoolingLimit{IntoFridgeWithinSeconds: 7201, CutoffAfterSeconds: 9000, CutoffMaxC: 10}}},
		"cooling cutoff": {Check: CheckCooling, Thresholds: Thresholds{Cooling: &CoolingLimit{IntoFridgeWithinSeconds: 7200, CutoffAfterSeconds: 9001, CutoffMaxC: 10}}},
		"cooling temp":   {Check: CheckCooling, Thresholds: Thresholds{Cooling: &CoolingLimit{IntoFridgeWithinSeconds: 7200, CutoffAfterSeconds: 9000, CutoffMaxC: 10.1}}},
	}
	for name, lv := range cases {
		t.Run(name, func(t *testing.T) { assert.Len(t, lv.LooserThanFSAI(), 1) })
	}
	onLimit := LimitVersion{Check: CheckFridge, Thresholds: Thresholds{MaxC: f(5.0)}}
	assert.Empty(t, onLimit.LooserThanFSAI())
	hotterLonger := LimitVersion{Check: CheckCook, Thresholds: Thresholds{Combos: []TimeTemp{{MinC: 80, HoldSeconds: 30}}}}
	assert.Empty(t, hotterLonger.LooserThanFSAI(), "hotter and held longer is stricter")
}

func TestResolve(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	def := FSAIDefault(CheckFridge)
	fish := Scope{Kind: ScopeUnit, Ref: "Fish fridge"}
	unitV1 := LimitVersion{ID: "u1", Check: CheckFridge, Scope: fish, Thresholds: Thresholds{MaxC: f(3)}, CreatedAt: t0}
	unitV2 := LimitVersion{ID: "u2", Check: CheckFridge, Scope: fish, Thresholds: Thresholds{MaxC: f(2)}, CreatedAt: t0.Add(time.Hour), Supersedes: "u1"}
	cook := FSAIDefault(CheckCook)
	poultry := LimitVersion{ID: "p1", Check: CheckCook, Scope: Scope{Kind: ScopeCategory, Ref: "poultry"}, Thresholds: Thresholds{Combos: []TimeTemp{{MinC: 75}}}, CreatedAt: t0}
	dish := LimitVersion{ID: "d1", Check: CheckCook, Scope: Scope{Kind: ScopeDish, Ref: "Chicken wings"}, Thresholds: Thresholds{Combos: []TimeTemp{{MinC: 80}}}, CreatedAt: t0}
	history := []LimitVersion{def, unitV1, unitV2, cook, poultry, dish}

	assert.Equal(t, "u2", Resolve(history, CheckFridge, []Scope{fish}).ID, "the newest version of the unit limit applies")
	assert.Equal(t, def.ID, Resolve(history, CheckFridge, []Scope{{Kind: ScopeUnit, Ref: "Walk-in"}}).ID)

	wings := Resolve(history, CheckCook, Scopes(RecordCook, Reading{Dish: "Chicken wings", FoodCategory: "poultry"}))
	assert.Equal(t, "d1", wings.ID, "dish beats category")
	thighs := Resolve(history, CheckCook, Scopes(RecordCook, Reading{Dish: "Thighs", FoodCategory: "poultry"}))
	assert.Equal(t, "p1", thighs.ID, "category beats default")

	assert.Equal(t, FSAIDefaultID(CheckHotHold), Resolve(nil, CheckHotHold, nil).ID, "falls back to the FSAI constant")
}
