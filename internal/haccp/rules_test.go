package haccp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuidance(t *testing.T) {
	g := FSAIGuidance()
	require.NoError(t, g.Validate())
	assert.Empty(t, g.LooserThanFSAI())
	assert.Equal(t, FSAIThresholds(CheckFridge), g.For(CheckFridge))

	missing := FSAIGuidance()
	missing.Checks = missing.Checks[1:]
	assert.ErrorContains(t, missing.Validate(), "missing")

	twice := FSAIGuidance()
	twice.Checks = append(twice.Checks, twice.Checks[0])
	assert.ErrorContains(t, twice.Validate(), "twice")

	unnamed := FSAIGuidance()
	unnamed.Name = ""
	assert.Error(t, unnamed.Validate())

	wrongShape := FSAIGuidance()
	wrongShape.Checks[0].MinC = f(1)
	assert.Error(t, wrongShape.Validate())

	looser := FSAIGuidance()
	looser.Checks[0].MaxC = f(6)
	assert.Equal(t, []string{"fridge: maximum 6.0°C is above the FSAI 5.0°C"}, looser.LooserThanFSAI())

	n := Guidance{Name: " Plan ", Checks: []GuidanceCheck{{Check: CheckFridge, Thresholds: Thresholds{MaxC: f(4.04)}}}}.Normalize()
	assert.Equal(t, "Plan", n.Name)
	assert.Equal(t, 4.0, *n.Checks[0].MaxC)
}

func TestRulesNeedsAction(t *testing.T) {
	r := DefaultRules()
	assert.True(t, r.NeedsAction(OutcomeFail))
	assert.True(t, r.NeedsAction(OutcomeBorderline))
	assert.False(t, r.NeedsAction(OutcomePass))
	assert.False(t, r.NeedsAction(OutcomeRecorded))

	r.BorderlineNeedsAction = false
	assert.False(t, r.NeedsAction(OutcomeBorderline))
	assert.True(t, r.NeedsAction(OutcomeFail), "a fail always needs a corrective action")
}

func testLists() Lists {
	return Lists{
		Units: []Unit{
			{Name: "Walk-in", Kind: RecordFridge},
			{Name: "Chest freezer", Kind: RecordFreezer},
			{Name: "Drinks fridge", Kind: RecordFridge, NonFood: true},
		},
		FoodCategories: []string{"poultry", "fish"},
		CleaningTasks:  []CleaningTask{{Task: "Deep clean fryer", Area: "Fryer station"}},
	}
}

func TestListsValidate(t *testing.T) {
	require.NoError(t, testLists().Validate())
	require.NoError(t, DefaultLists().Validate())
	require.NoError(t, emptyLists().Validate())

	bad := map[string]Lists{
		"duplicate unit":      {Units: []Unit{{Name: "Walk-in", Kind: RecordFridge}, {Name: "walk-in", Kind: RecordFreezer}}},
		"unit kind":           {Units: []Unit{{Name: "Oven", Kind: RecordCook}}},
		"empty category":      {FoodCategories: []string{""}},
		"duplicate task":      {CleaningTasks: []CleaningTask{{Task: "Mop"}, {Task: "MOP"}}},
		"empty task":          {CleaningTasks: []CleaningTask{{Area: "Floor"}}},
		"blank after trim":    Lists{FoodCategories: []string{"  "}}.Normalize(),
		"checks out of order": {Units: []Unit{{Name: "A", Kind: RecordFridge, Checks: []CheckTime{{"Late", "14:00"}, {"Early", "08:00"}}}}},
		"check bad time":      {Units: []Unit{{Name: "A", Kind: RecordFridge, Checks: []CheckTime{{"Open", "8am"}}}}},
		"check no label":      {Units: []Unit{{Name: "A", Kind: RecordFridge, Checks: []CheckTime{{"", "08:00"}}}}},
		"check label twice":   {Units: []Unit{{Name: "A", Kind: RecordFridge, Checks: []CheckTime{{"Open", "08:00"}, {"open", "09:00"}}}}},
		"preset bad type":     {CorrectiveActions: []ActionPreset{{Type: "nope", Text: "x"}}},
		"preset twice":        {CorrectiveActions: []ActionPreset{{RecordCook, "Discarded"}, {RecordCook, "discarded"}}},
	}
	for name, l := range bad {
		t.Run(name, func(t *testing.T) { assert.Error(t, l.Validate()) })
	}
}

func TestListsApply(t *testing.T) {
	l := testLists()
	strict := DefaultRules()
	loose := Rules{ListedNamesOnly: false}

	r, err := l.Apply(RecordFridge, Reading{Unit: "walk-IN"}, strict)
	require.NoError(t, err)
	assert.Equal(t, "Walk-in", r.Unit, "the listed spelling is used")

	_, err = l.Apply(RecordFridge, Reading{Unit: "Walkin"}, strict)
	assert.ErrorContains(t, err, `"Walkin" is not on the fridge list. Use one of: Walk-in, Drinks fridge`)

	_, err = l.Apply(RecordFridge, Reading{Unit: "Chest freezer"}, strict)
	assert.Error(t, err, "a freezer is not a fridge")

	r, err = l.Apply(RecordFridge, Reading{Unit: "Walkin"}, loose)
	require.NoError(t, err)
	assert.Equal(t, "Walkin", r.Unit, "unlisted names are allowed when the rule is off")

	r, err = emptyLists().Apply(RecordFridge, Reading{Unit: "Anything"}, strict)
	require.NoError(t, err)
	assert.Equal(t, "Anything", r.Unit, "an empty list accepts any name")

	r, err = l.Apply(RecordCook, Reading{Food: "Wings", FoodCategory: "POULTRY"}, strict)
	require.NoError(t, err)
	assert.Equal(t, "poultry", r.FoodCategory)
	_, err = l.Apply(RecordCook, Reading{Food: "Wings", FoodCategory: "chicken"}, strict)
	assert.Error(t, err)

	r, err = l.Apply(RecordCleaning, Reading{Task: "deep clean fryer"}, strict)
	require.NoError(t, err)
	assert.Equal(t, "Deep clean fryer", r.Task)
	assert.Equal(t, "Fryer station", r.Area, "the area comes from the list")

	sc, err := l.ApplyScope(CheckFreezer, Scope{Kind: ScopeUnit, Ref: "chest FREEZER"}, strict)
	require.NoError(t, err)
	assert.Equal(t, "Chest freezer", sc.Ref)
	_, err = l.ApplyScope(CheckFridge, Scope{Kind: ScopeCategory, Ref: "pork"}, strict)
	assert.Error(t, err)
	sc, err = l.ApplyScope(CheckCook, Scope{Kind: ScopeDish, Ref: "Anything"}, strict)
	require.NoError(t, err)
	assert.Equal(t, "Anything", sc.Ref, "dishes are not listed yet")
}

func TestDefaultListsMatchPrototype(t *testing.T) {
	l := DefaultLists()
	require.Len(t, l.Units, 3)
	assert.Equal(t, "Walk-in fridge", l.Units[0].Name)
	assert.Equal(t, RecordFreezer, l.Units[2].Kind)
	assert.Len(t, l.CleaningTasks, 6)
	assert.Equal(t, DefaultChecks(), l.Units[0].ChecksOrDefault())
	var freezer []string
	for _, a := range l.CorrectiveActions {
		if a.Type == RecordFreezer {
			freezer = append(freezer, a.Text)
		}
	}
	assert.Contains(t, freezer, "Called fridge engineer", "freezers share the fridge presets")
}
