package analytics

import (
	"math"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
)

// How cook times are learned. Each cook's actual cook time is the time from
// started to ready, less the dish's rest and plate time and any modifier
// (such as "well done"). The learned time is a smoothed average of those:
// each new cook moves it a fifth of the way towards itself, so recent cooks
// count most and one odd cook moves it little.
const (
	// LearnWeight is how far each new cook moves the learned time.
	LearnWeight = 0.2
	// LearnMinCooks is how many cooks a dish needs before its learned time
	// can be used.
	LearnMinCooks = 10
	// LearnWindow is how far back cooks are taken from.
	LearnWindow = 90 * 24 * time.Hour
	// A cook that took under a quarter, or over four times, the time set
	// when it was ordered is left out: most likely someone tapped ready
	// late or early.
	learnLow, learnHigh = 0.25, 4.0
)

// LearnedCook is one dish's learned cook time.
type LearnedCook struct {
	DishID string `json:"dish_id"`
	// CookSeconds is the learned cook time, before modifiers.
	CookSeconds int `json:"cook_seconds"`
	// Cooks is how many cooks it was learned from.
	Cooks int `json:"cooks"`
	// Skipped is how many cooks were left out as far from the time set.
	Skipped int `json:"skipped"`
	// Ready is true once there are enough cooks to use it.
	Ready bool `json:"ready"`
	// LastAt is when the latest cook it learned from was ready.
	LastAt time.Time `json:"last_at"`
}

// LearnCookTimes learns each dish's cook time from items that were cooked:
// started and ready, not voided. since gives, per dish, the time before
// which cooks are ignored (after a manager resets it). dishes are the
// dishes as they are now, for their modifiers; an item whose modifier the
// dish no longer has is left out.
func LearnCookTimes(items []orders.Item, dishes map[string]menu.Dish, since map[string]time.Time) map[string]LearnedCook {
	type sample struct {
		at   time.Time
		cook float64
	}
	byDish := map[string][]sample{}
	skipped := map[string]int{}
	for _, it := range items {
		d, ok := dishes[it.DishID]
		if !ok || it.State == orders.StateVoided || it.StartedAt == nil || it.ReadyAt == nil || !it.ReadyAt.After(*it.StartedAt) {
			continue
		}
		if s, ok := since[it.DishID]; ok && it.ReadyAt.Before(s) {
			continue
		}
		add, ok := modifierSeconds(d, it.Modifiers)
		if !ok {
			continue
		}
		set := it.Timing.Total()
		took := it.ReadyAt.Sub(*it.StartedAt)
		if set <= 0 || float64(took) < learnLow*float64(set) || float64(took) > learnHigh*float64(set) {
			skipped[it.DishID]++
			continue
		}
		cook := (took - it.Timing.Rest - it.Timing.Plate).Seconds() - float64(add)
		cook = math.Max(0, math.Min(cook, menu.MaxPhaseSecs))
		byDish[it.DishID] = append(byDish[it.DishID], sample{*it.ReadyAt, cook})
	}
	out := map[string]LearnedCook{}
	for id, ss := range byDish {
		slices.SortStableFunc(ss, func(a, b sample) int { return a.at.Compare(b.at) })
		avg := ss[0].cook
		for _, s := range ss[1:] {
			avg += LearnWeight * (s.cook - avg)
		}
		out[id] = LearnedCook{
			DishID: id, CookSeconds: int(math.Round(avg)), Cooks: len(ss), Skipped: skipped[id],
			Ready: len(ss) >= LearnMinCooks, LastAt: ss[len(ss)-1].at,
		}
	}
	for id, n := range skipped {
		if _, ok := out[id]; !ok {
			out[id] = LearnedCook{DishID: id, Skipped: n}
		}
	}
	return out
}

// modifierSeconds adds up the dish's modifiers named, and reports false if
// the dish no longer has one of them.
func modifierSeconds(d menu.Dish, names []string) (int, bool) {
	add := 0
	for _, n := range names {
		i := slices.IndexFunc(d.Modifiers, func(m menu.Modifier) bool { return strings.EqualFold(m.Name, strings.TrimSpace(n)) })
		if i < 0 {
			return 0, false
		}
		add += d.Modifiers[i].AddSeconds
	}
	return add, true
}
