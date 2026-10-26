package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/analytics"
	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// learnedFor is how long learned cook times are kept before being worked
// out again from past cooks. They change slowly, and working them out reads
// LearnWindow of orders.
const learnedFor = 5 * time.Minute

// learnCache holds the learned cook times last worked out.
type learnCache struct {
	mu     sync.Mutex
	at     time.Time
	byDish map[string]analytics.LearnedCook
}

// learned returns each dish's learned cook time, worked out again when the
// last one is older than learnedFor.
func (s *Service) learned(ctx context.Context) (map[string]analytics.LearnedCook, error) {
	s.learnt.mu.Lock()
	defer s.learnt.mu.Unlock()
	now := s.clk.Now()
	if s.learnt.byDish != nil && now.Sub(s.learnt.at) < learnedFor && !now.Before(s.learnt.at) {
		return s.learnt.byDish, nil
	}
	dishes, err := s.st.Menu().ListDishes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list dishes: %w", err)
	}
	resets, err := s.st.Menu().LearnedResets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list learned resets: %w", err)
	}
	os, err := s.st.Orders().ListOrders(ctx, store.OrderFilter{From: now.Add(-analytics.LearnWindow), To: now.Add(time.Minute)})
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	byID := make(map[string]menu.Dish, len(dishes))
	for _, d := range dishes {
		byID[d.ID] = d
	}
	var n int
	for _, o := range os {
		n += len(o.Items)
	}
	items := make([]orders.Item, 0, n)
	for _, o := range os {
		items = append(items, o.Items...)
	}
	s.learnt.byDish = analytics.LearnCookTimes(items, byID, resets)
	s.learnt.at = now
	return s.learnt.byDish, nil
}

func (s *Service) forgetLearned() {
	s.learnt.mu.Lock()
	s.learnt.byDish = nil
	s.learnt.mu.Unlock()
}

// cookTimes returns the cook time new orders use for each dish whose
// learned time is ready, when the scheduler settings say to use learned
// times; otherwise none, and dishes use the time set on the menu.
func (s *Service) cookTimes(ctx context.Context, st store.Store) (map[string]int, error) {
	set, err := s.Scheduler().current(ctx, st)
	if err != nil {
		return nil, err
	}
	if !set.Value.LearnedTimes {
		return nil, nil
	}
	learned, err := s.learned(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for id, l := range learned {
		if l.Ready {
			out[id] = l.CookSeconds
		}
	}
	return out, nil
}

// LearnedDish is one dish's cook time as set on the menu and as learned
// from past cooks.
type LearnedDish struct {
	DishID   string `json:"dish_id"`
	DishName string `json:"dish_name"`
	// SetSeconds is the cook time set on the menu.
	SetSeconds int `json:"set_seconds"`
	// LearnedSeconds is the learned cook time; 0 with no cooks yet.
	LearnedSeconds int `json:"learned_seconds"`
	// Cooks is how many cooks it was learned from, since any reset, in the
	// last 90 days.
	Cooks int `json:"cooks"`
	// Skipped is how many cooks were left out as too far from the time set.
	Skipped int `json:"skipped"`
	// Ready is true once there are enough cooks to use it.
	Ready bool `json:"ready"`
	// InUse is true when new orders use the learned time.
	InUse bool `json:"in_use"`
	// LastCook is when the latest cook learned from was ready.
	LastCook *time.Time `json:"last_cook,omitempty"`
	// ResetAt is when a manager last reset it.
	ResetAt *time.Time `json:"reset_at,omitempty"`
}

// LearnedTimes are every dish's learned cook times, with whether new orders
// use them.
type LearnedTimes struct {
	// On is true when the scheduler settings say to use learned times.
	On bool `json:"on"`
	// MinCooks is how many cooks a dish needs before its time is used.
	MinCooks int           `json:"min_cooks"`
	Dishes   []LearnedDish `json:"dishes"`
}

// Learned lists each active dish's set and learned cook times. Any staff
// member who can see the menu can see them.
func (s *Service) Learned(ctx context.Context, p *auth.Principal) (LearnedTimes, error) {
	if err := s.allow(p, auth.ActViewKitchen); err != nil {
		return LearnedTimes{}, err
	}
	return s.learnedTimes(ctx)
}

func (s *Service) learnedTimes(ctx context.Context) (LearnedTimes, error) {
	set, err := s.Scheduler().current(ctx, s.st)
	if err != nil {
		return LearnedTimes{}, err
	}
	learned, err := s.learned(ctx)
	if err != nil {
		return LearnedTimes{}, err
	}
	dishes, err := s.st.Menu().ListDishes(ctx)
	if err != nil {
		return LearnedTimes{}, err
	}
	resets, err := s.st.Menu().LearnedResets(ctx)
	if err != nil {
		return LearnedTimes{}, err
	}
	out := LearnedTimes{On: set.Value.LearnedTimes, MinCooks: analytics.LearnMinCooks, Dishes: []LearnedDish{}}
	for _, d := range dishes {
		if !d.Active {
			continue
		}
		l := learned[d.ID]
		ld := LearnedDish{
			DishID: d.ID, DishName: d.Name, SetSeconds: d.CookSeconds, LearnedSeconds: l.CookSeconds,
			Cooks: l.Cooks, Skipped: l.Skipped, Ready: l.Ready, InUse: out.On && l.Ready,
		}
		if !l.LastAt.IsZero() {
			at := l.LastAt
			ld.LastCook = &at
		}
		if r, ok := resets[d.ID]; ok {
			ld.ResetAt = &r
		}
		out.Dishes = append(out.Dishes, ld)
	}
	sort.SliceStable(out.Dishes, func(i, j int) bool { return out.Dishes[i].DishName < out.Dishes[j].DishName })
	return out, nil
}

// ResetLearned starts a dish's learned cook time again from its next cook,
// for example after a new recipe or new equipment. It is in the activity
// log.
func (s *Service) ResetLearned(ctx context.Context, p *auth.Principal, dishID string) (LearnedDish, error) {
	if err := s.allow(p, auth.ActManageMenu); err != nil {
		return LearnedDish{}, err
	}
	var name string
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		d, err := tx.Menu().GetDish(ctx, dishID)
		if err != nil {
			return mapStoreErr(err, "dish")
		}
		name = d.Name
		if err := tx.Menu().ResetLearned(ctx, d.ID, s.clk.Now(), p.User.ID); err != nil {
			return err
		}
		return s.note(ctx, tx, p, ActivityLearnedReset, d.ID, map[string]any{"dish": d.Name})
	})
	if err != nil {
		return LearnedDish{}, err
	}
	s.forgetLearned()
	s.publish(events.KindMenu, dishID)
	all, err := s.learnedTimes(ctx)
	if err != nil {
		return LearnedDish{}, err
	}
	for _, d := range all.Dishes {
		if d.DishID == dishID {
			return d, nil
		}
	}
	return LearnedDish{DishID: dishID, DishName: name}, nil
}

// learnedNotes says, for a change to the scheduler settings, which dishes'
// learned times new orders will start or stop using.
func (s *Service) learnedNotes(ctx context.Context, cur, next bool) ([]string, error) {
	if cur == next {
		return nil, nil
	}
	if !next {
		return []string{"New orders will use the cook times set on the menu again. Orders already in the kitchen keep their times."}, nil
	}
	all, err := s.learnedTimes(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	waiting := 0
	for _, d := range all.Dishes {
		if !d.Ready {
			waiting++
			continue
		}
		out = append(out, fmt.Sprintf("%s: cook time %s set, %s learned from %d cooks.", d.DishName, minSec(d.SetSeconds), minSec(d.LearnedSeconds), d.Cooks))
	}
	if len(out) == 0 {
		out = append(out, fmt.Sprintf("No dish has %d cooks yet, so nothing changes until one does.", analytics.LearnMinCooks))
	} else {
		out = append([]string{"New orders will use these learned cook times. Orders already in the kitchen keep their times."}, out...)
	}
	if waiting > 0 {
		out = append(out, fmt.Sprintf("%d other dishes keep their set times until they have %d cooks.", waiting, analytics.LearnMinCooks))
	}
	return out, nil
}

func minSec(secs int) string { return fmt.Sprintf("%d:%02d", secs/60, secs%60) }
