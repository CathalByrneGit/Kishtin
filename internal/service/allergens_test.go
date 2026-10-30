package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
)

func TestAllergens(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()

	chart, err := k.svc.Allergens(ctx, k.foh)
	require.NoError(t, err, "front of house can see the chart")
	assert.Len(t, chart.Allergens, 14)
	assert.Equal(t, 2, chart.Unchecked, "dishes start unchecked")
	assert.Equal(t, menu.CourseStarter, chart.Dishes[0].Course, "in course order")

	in := DishInput{Name: k.burger.Name, Course: k.burger.Course, StationID: k.burger.StationID, EquipmentID: k.burger.EquipmentID,
		CookSeconds: k.burger.CookSeconds, RestSeconds: k.burger.RestSeconds, PlateSeconds: k.burger.PlateSeconds,
		HoldSeconds: k.burger.HoldSeconds, Batchable: k.burger.Batchable, Modifiers: k.burger.Modifiers,
		Allergens: &[]string{"milk", "GLUTEN"}, AllergensChecked: new(true)}
	d, err := k.svc.UpdateDish(ctx, k.manager, k.burger.ID, in)
	require.NoError(t, err)
	assert.Equal(t, []string{"gluten", "milk"}, d.Allergens)

	in.Allergens = nil
	in.CookSeconds = 500
	d, err = k.svc.UpdateDish(ctx, k.manager, k.burger.ID, in)
	require.NoError(t, err)
	assert.Equal(t, []string{"gluten", "milk"}, d.Allergens, "left out, allergens stay as they are")

	in.AllergensNone = true
	in.Allergens = &[]string{"milk"}
	_, err = k.svc.UpdateDish(ctx, k.manager, k.burger.ID, in)
	assert.Equal(t, KindInvalid, KindOf(err), "none and some at once")
	in.Allergens = nil
	d, err = k.svc.UpdateDish(ctx, k.manager, k.burger.ID, in)
	require.NoError(t, err)
	assert.Equal(t, []string{}, d.Allergens)

	in.AllergensNone, in.Allergens = false, &[]string{"garlic"}
	_, err = k.svc.UpdateDish(ctx, k.manager, k.burger.ID, in)
	assert.ErrorContains(t, err, "not one of the 14 allergens")

	acts, err := k.svc.Activity(ctx, k.manager, ActivityQuery{Kinds: []string{ActivityAllergensChanged}})
	require.NoError(t, err)
	require.Len(t, acts, 2, "a change that leaves allergens alone is not logged")
	assert.Equal(t, "none", acts[0].Detail["before"])
	assert.Equal(t, "Cereals containing gluten, Milk", acts[0].Detail["after"])
	assert.Equal(t, "true", acts[0].Detail["checked"])
	assert.Equal(t, "none", acts[1].Detail["after"])

	chart, err = k.svc.Allergens(ctx, k.foh)
	require.NoError(t, err)
	assert.Equal(t, 1, chart.Unchecked)
}
