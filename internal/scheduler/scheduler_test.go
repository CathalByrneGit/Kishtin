package scheduler

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
)

var now = time.Date(2026, 5, 1, 18, 0, 0, 0, time.UTC)

// randomKitchen builds a busy kitchen from a seed.
func randomKitchen(seed uint64) KitchenState {
	r := rand.New(rand.NewPCG(seed, 7))
	ks := KitchenState{
		Stations:  []Station{{ID: "grill", Name: "Grill"}, {ID: "fryer", Name: "Fryer"}, {ID: "cold", Name: "Cold"}},
		Equipment: []Equipment{{ID: "flat", StationID: "grill", Name: "Flat-top"}, {ID: "fry", StationID: "fryer", Name: "Fryer"}},
	}
	dishes := []Item{
		{DishID: "burger", DishName: "Burger", StationID: "grill", EquipmentID: "flat", Capacity: 3, Cook: 8 * time.Minute, Rest: 2 * time.Minute, Plate: time.Minute, HoldMax: 3 * time.Minute, Batchable: true},
		{DishID: "steak", DishName: "Steak", StationID: "grill", EquipmentID: "flat", Capacity: 2, Cook: 12 * time.Minute, Rest: 5 * time.Minute, Plate: time.Minute},
		{DishID: "fries", DishName: "Fries", StationID: "fryer", EquipmentID: "fry", Capacity: 2, Cook: 4 * time.Minute, Plate: 30 * time.Second, HoldMax: 2 * time.Minute, Batchable: true},
		{DishID: "salad", DishName: "Salad", StationID: "cold", Plate: 3 * time.Minute, HoldMax: 5 * time.Minute},
	}
	cooking := map[string]float64{}
	for c := range 3 + r.IntN(6) {
		fired := now.Add(-time.Duration(r.IntN(10)) * time.Minute)
		course := Course{OrderID: fmt.Sprintf("o%d", c), Label: fmt.Sprintf("Table %d", c), Course: menu.CourseMain, FiredAt: fired, Urgent: r.IntN(5) == 0}
		var longest time.Duration
		for i := range 1 + r.IntN(4) {
			it := dishes[r.IntN(len(dishes))]
			it.ID = fmt.Sprintf("o%d-%d", c, i)
			it.State = orders.StateFired
			it.StateAt = fired
			// Some items already cooking, never more than the equipment holds.
			if r.IntN(4) == 0 && it.Cook > 0 && cooking[it.EquipmentID]+1/float64(it.Capacity) <= 1+epsilon {
				cooking[it.EquipmentID] += 1 / float64(it.Capacity)
				it.State = orders.StateCooking
				it.StateAt = fired.Add(time.Minute)
			}
			longest = max(longest, it.total())
			course.Items = append(course.Items, it)
		}
		course.Target = fired.Add(longest)
		ks.Courses = append(ks.Courses, course)
	}
	return ks
}

func TestPlanNeverOverloadsEquipment(t *testing.T) {
	cfg := DefaultSettings().Config("v", time.UTC)
	for seed := range uint64(300) {
		ks := randomKitchen(seed)
		for _, strategy := range Strategies {
			cfg.Strategy = strategy
			p := PlanKitchen(ks, cfg, now)

			// Rebuild equipment use from items already cooking and the plan.
			byID := map[string]Item{}
			use := map[string][]interval{}
			for _, c := range ks.Courses {
				for _, it := range c.Items {
					byID[it.ID] = it
					if it.State == orders.StateCooking {
						end := it.StateAt.Add(it.Cook)
						if end.Before(now) {
							end = now
						}
						use[it.EquipmentID] = append(use[it.EquipmentID], interval{start: it.StateAt, end: end, use: 1 / float64(it.Capacity)})
					}
				}
			}
			planned := map[string]bool{}
			for _, ip := range p.Items {
				it := byID[ip.ItemID]
				planned[it.ID] = true
				assert.False(t, ip.StartAt.Before(now), "seed %d: nothing starts in the past", seed)
				if it.EquipmentID != "" {
					use[it.EquipmentID] = append(use[it.EquipmentID], interval{start: ip.StartAt, end: ip.StartAt.Add(it.Cook), use: 1 / float64(it.Capacity)})
				}
			}
			for eq, list := range use {
				for _, iv := range list {
					assert.LessOrEqual(t, useAt(list, iv.start), 1+epsilon, "seed %d %s: %s over capacity at %s", seed, strategy, eq, iv.start)
				}
			}
			// Every fired item is planned, and every action's items are in it.
			for _, c := range ks.Courses {
				for _, it := range c.Items {
					if it.State == orders.StateFired {
						assert.True(t, planned[it.ID], "seed %d: %s planned", seed, it.ID)
					}
				}
			}
			n := 0
			for _, sp := range p.Stations {
				for _, a := range sp.Actions {
					n += len(a.ItemIDs)
					assert.Equal(t, a.Quantity, len(a.ItemIDs))
					assert.NotEmpty(t, a.Reason, "every recommendation has a reason")
				}
			}
			assert.Equal(t, len(p.Items), n)
			assert.Len(t, p.Courses, len(ks.Courses))
		}
	}
}

func TestBackwardFinishesCoursesTogether(t *testing.T) {
	cfg := DefaultSettings().Config("v", time.UTC)
	cfg.Batching = false
	for seed := range uint64(300) {
		ks := randomKitchen(seed)
		p := PlanKitchen(ks, cfg, now)
		plans := map[string]ItemPlan{}
		for _, ip := range p.Items {
			plans[ip.ItemID] = ip
		}
		status := map[string]CourseStatus{}
		for _, cs := range p.Courses {
			status[cs.OrderID] = cs
		}
		for _, c := range ks.Courses {
			// The course is ready when its last item is: that is its planned
			// ready time.
			last := time.Time{}
			own := map[string]float64{}
			for _, it := range c.Items {
				if ip, ok := plans[it.ID]; ok {
					last = maxTime(last, ip.ReadyAt)
					own[it.EquipmentID] += 1 / float64(max(it.Capacity, 1))
				}
			}
			if last.IsZero() {
				continue
			}
			assert.False(t, last.After(status[c.OrderID].PlannedReady), "seed %d: %s", seed, c.Label)
			// An item ready before the rest waits at the pass no longer than
			// the dish can wait, unless the course alone needs more of the
			// equipment than it holds.
			for _, it := range c.Items {
				ip, ok := plans[it.ID]
				if !ok || own[it.EquipmentID] > 1+epsilon || it.EquipmentID == "" {
					continue
				}
				wait := last.Sub(ip.ReadyAt)
				assert.LessOrEqual(t, wait, it.HoldMax+2*time.Second, "seed %d: %s %s waits at the pass", seed, c.Label, it.ID)
			}
		}
	}
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func TestSettingsValidate(t *testing.T) {
	require.NoError(t, DefaultSettings().Validate())
	cases := map[string]func(*Settings){
		"unknown strategy": func(s *Settings) { s.Strategy = "chaos" },
		"long window":      func(s *Settings) { s.BatchWindowSeconds = 901 },
		"fast replan":      func(s *Settings) { s.ReplanSeconds = 1 },
		"buffer no id":     func(s *Settings) { s.StationBuffers = []StationBuffer{{Seconds: 30}} },
		"two buffers":      func(s *Settings) { s.StationBuffers = []StationBuffer{{"g", 30}, {"g", 60}} },
		"negative buffer":  func(s *Settings) { s.StationBuffers = []StationBuffer{{"g", -1}} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := DefaultSettings()
			mutate(&s)
			assert.Error(t, s.Validate())
		})
	}
}

func TestEmptyKitchen(t *testing.T) {
	p := PlanKitchen(KitchenState{Stations: []Station{{ID: "grill", Name: "Grill"}}}, DefaultSettings().Config("v", nil), now)
	require.Len(t, p.Stations, 1)
	assert.Empty(t, p.Stations[0].Actions)
	assert.Empty(t, p.Courses)
	assert.Equal(t, StrategyBackward, p.Strategy)
	assert.Equal(t, "v", p.SettingsVersion)
}
