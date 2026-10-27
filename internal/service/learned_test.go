package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/analytics"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/scheduler"
)

// cookBurger orders one burger, fires it, cooks it for took from start to
// ready, and serves it.
func (k *kitchen) cookBurger(t *testing.T, took time.Duration) orders.Item {
	t.Helper()
	ctx := context.Background()
	o, err := k.svc.CreateOrder(ctx, k.chef, NewOrder{Kind: orders.KindTakeaway, Items: []NewItem{{DishID: k.burger.ID}}, Fire: []menu.Course{menu.CourseMain}})
	require.NoError(t, err)
	id := o.Items[0].ID
	_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: []string{id}, To: orders.StateCooking})
	require.NoError(t, err)
	// Most of the time cooking, then 2 minutes resting and 1 plating.
	for _, step := range []struct {
		wait time.Duration
		to   orders.State
	}{{took - 3*time.Minute, orders.StateResting}, {2 * time.Minute, orders.StatePlating}, {time.Minute, orders.StateReady}} {
		k.clk.Advance(step.wait)
		_, err = k.svc.MoveItems(ctx, k.cook, ItemMove{ItemIDs: []string{id}, To: step.to})
		require.NoError(t, err)
	}
	k.clk.Advance(30 * time.Minute)
	return o.Items[0]
}

func (k *kitchen) learnedBurger(t *testing.T) LearnedDish {
	t.Helper()
	k.clk.Advance(learnedFor) // past the cache
	all, err := k.svc.Learned(context.Background(), k.cook)
	require.NoError(t, err)
	for _, d := range all.Dishes {
		if d.DishID == k.burger.ID {
			return d
		}
	}
	t.Fatal("no burger")
	return LearnedDish{}
}

func TestLearnedCookTimes(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()

	// The burger is set at 8 minutes cooking, 2 resting, 1 plating, but
	// takes 13 minutes from start to ready: 10 minutes of cooking.
	for range analytics.LearnMinCooks {
		k.cookBurger(t, 13*time.Minute)
	}
	b := k.learnedBurger(t)
	assert.Equal(t, 480, b.SetSeconds)
	assert.Equal(t, 600, b.LearnedSeconds)
	assert.Equal(t, analytics.LearnMinCooks, b.Cooks)
	assert.True(t, b.Ready)
	assert.False(t, b.InUse, "off until a manager turns it on")

	it := k.cookBurger(t, 13*time.Minute)
	assert.Equal(t, 8*time.Minute, it.Timing.Cook, "orders use the set time while it is off")
	assert.False(t, it.CookLearned)

	// Turning it on is reviewed like any scheduler change.
	cur, err := k.svc.Scheduler().Current(ctx, k.manager)
	require.NoError(t, err)
	next := cur.Value
	next.LearnedTimes = true
	pre, err := k.svc.Scheduler().Preview(ctx, k.manager, SettingsChange[scheduler.Settings]{Value: next, Reason: "Use what the grill really takes"})
	require.NoError(t, err)
	assert.Contains(t, pre.Notes, "Synthetic burger: cook time 8:00 set, 10:00 learned from 11 cooks.")
	assert.Contains(t, pre.Notes, "1 other dishes keep their set times until they have 10 cooks.")
	_, err = k.svc.Scheduler().Save(ctx, k.manager, SettingsChange[scheduler.Settings]{Value: next, Reason: "Use what the grill really takes"})
	require.NoError(t, err)

	k.clk.Advance(learnedFor)
	o, err := k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: "4", Items: []NewItem{
		{DishID: k.burger.ID, Modifiers: []string{"Well done"}}, {DishID: k.salad.ID},
	}})
	require.NoError(t, err)
	burger, salad := o.Items[0], o.Items[1]
	assert.Equal(t, 12*time.Minute, burger.Timing.Cook, "learned 10 minutes plus 2 for well done")
	assert.True(t, burger.CookLearned)
	assert.Equal(t, time.Duration(0), salad.Timing.Cook, "a dish without enough cooks keeps its set time")
	assert.False(t, salad.CookLearned)
	assert.True(t, k.learnedBurger(t).InUse)

	// A manager resets it after changing the recipe; a cook cannot.
	_, err = k.svc.ResetLearned(ctx, k.cook, k.burger.ID)
	assert.Equal(t, KindForbidden, KindOf(err))
	b, err = k.svc.ResetLearned(ctx, k.manager, k.burger.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, b.Cooks)
	assert.False(t, b.InUse)
	require.NotNil(t, b.ResetAt)
	o, err = k.svc.CreateOrder(ctx, k.foh, NewOrder{Kind: orders.KindTable, Table: "5", Items: []NewItem{{DishID: k.burger.ID}}})
	require.NoError(t, err)
	assert.Equal(t, 8*time.Minute, o.Items[0].Timing.Cook, "back to the set time until it has learned again")

	acts, err := k.svc.Activity(ctx, k.manager, ActivityQuery{Kinds: []string{ActivityLearnedReset}})
	require.NoError(t, err)
	require.Len(t, acts, 1)
	assert.Equal(t, "Synthetic burger", acts[0].Detail["dish"])

	_, err = k.svc.ResetLearned(ctx, k.manager, "no-such-dish")
	assert.Equal(t, KindNotFound, KindOf(err))
}
