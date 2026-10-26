package analytics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
)

var burger = menu.Dish{ID: "burger", Name: "Burger", CookSeconds: 480, RestSeconds: 120, PlateSeconds: 60,
	Modifiers: []menu.Modifier{{Name: "Well done", AddSeconds: 120}}}

// cooked is an item that took took from started to ready, ready at at.
func cooked(dish menu.Dish, at time.Time, took time.Duration, mods ...string) orders.Item {
	timing, err := dish.TimingFor(mods)
	if err != nil {
		panic(err)
	}
	start := at.Add(-took)
	return orders.Item{DishID: dish.ID, DishName: dish.Name, Modifiers: mods, State: orders.StateServed, Timing: timing, StartedAt: &start, ReadyAt: &at}
}

func TestLearnCookTimes(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	dishes := map[string]menu.Dish{"burger": burger}
	var items []orders.Item
	// Ten cooks taking 13 minutes from start to ready: 3 minutes of rest
	// and plate, so 10 minutes of cooking against 8 set.
	for i := range 10 {
		items = append(items, cooked(burger, t0.Add(time.Duration(i)*time.Hour), 13*time.Minute))
	}
	got := LearnCookTimes(items, dishes, nil)["burger"]
	assert.Equal(t, LearnedCook{DishID: "burger", CookSeconds: 600, Cooks: 10, Ready: true, LastAt: t0.Add(9 * time.Hour)}, got)

	t.Run("not ready under ten cooks", func(t *testing.T) {
		got := LearnCookTimes(items[:9], dishes, nil)["burger"]
		assert.False(t, got.Ready)
		assert.Equal(t, 9, got.Cooks)
	})

	t.Run("modifiers come off", func(t *testing.T) {
		// Well done adds 2 minutes; a well-done burger taking 15 minutes
		// is still 10 minutes of plain cooking.
		more := append(append([]orders.Item{}, items...), cooked(burger, t0.Add(10*time.Hour), 15*time.Minute, "Well done"))
		assert.Equal(t, 600, LearnCookTimes(more, dishes, nil)["burger"].CookSeconds)
	})

	t.Run("recent cooks count most", func(t *testing.T) {
		more := append([]orders.Item{}, items...)
		for i := range 10 {
			more = append(more, cooked(burger, t0.Add(time.Duration(20+i)*time.Hour), 11*time.Minute))
		}
		got := LearnCookTimes(more, dishes, nil)["burger"]
		// Ten cooks at 8 minutes after ten at 10: most of the way to 8.
		assert.InDelta(t, 480, got.CookSeconds, 15)
		assert.Equal(t, 20, got.Cooks)
	})

	t.Run("far from the set time is left out", func(t *testing.T) {
		more := append(append([]orders.Item{}, items...),
			cooked(burger, t0.Add(11*time.Hour), 2*time.Minute),  // under a quarter of 11 minutes
			cooked(burger, t0.Add(12*time.Hour), 50*time.Minute)) // over four times
		got := LearnCookTimes(more, dishes, nil)["burger"]
		assert.Equal(t, 600, got.CookSeconds)
		assert.Equal(t, 10, got.Cooks)
		assert.Equal(t, 2, got.Skipped)
	})

	t.Run("since a reset", func(t *testing.T) {
		got := LearnCookTimes(items, dishes, map[string]time.Time{"burger": t0.Add(5 * time.Hour)})["burger"]
		assert.Equal(t, 5, got.Cooks, "cooks before the reset are ignored")
	})

	t.Run("not cooked", func(t *testing.T) {
		voided := cooked(burger, t0, 13*time.Minute)
		voided.State = orders.StateVoided
		notStarted := cooked(burger, t0, 13*time.Minute)
		notStarted.StartedAt = nil
		unknownMod := cooked(menu.Dish{ID: "burger", CookSeconds: 480, Modifiers: []menu.Modifier{{Name: "Rare", AddSeconds: -60}}}, t0, 8*time.Minute, "Rare")
		gone := cooked(menu.Dish{ID: "soup", PlateSeconds: 60}, t0, time.Minute)
		got := LearnCookTimes([]orders.Item{voided, notStarted, unknownMod, gone}, dishes, nil)
		assert.Empty(t, got)
	})

	t.Run("never below zero", func(t *testing.T) {
		salad := menu.Dish{ID: "salad", PlateSeconds: 180}
		got := LearnCookTimes([]orders.Item{cooked(salad, t0, 2*time.Minute)}, map[string]menu.Dish{"salad": salad}, nil)
		require.Contains(t, got, "salad")
		assert.Equal(t, 0, got["salad"].CookSeconds)
	})
}
