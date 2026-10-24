package service

import (
	"context"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/scheduler"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// KitchenView is everything the pass and the station screens show: open
// orders, the scheduler's plan, and the settings that produced it.
type KitchenView struct {
	At       time.Time      `json:"at"`
	Orders   []orders.Order `json:"orders"`
	Plan     scheduler.Plan `json:"plan"`
	Stations []menu.Station `json:"stations"`
	// ReplanSeconds is how often screens should ask again when nothing
	// changes.
	ReplanSeconds int `json:"replan_seconds"`
}

// schedulerState turns open orders into the scheduler's input: every fired
// course not yet served.
func schedulerState(setup menu.Setup, open []orders.Order) scheduler.KitchenState {
	ks := scheduler.KitchenState{Stations: []scheduler.Station{}, Equipment: []scheduler.Equipment{}, Courses: []scheduler.Course{}}
	for _, st := range setup.Stations {
		if st.Active {
			ks.Stations = append(ks.Stations, scheduler.Station{ID: st.ID, Name: st.Name})
		}
	}
	for _, e := range setup.Equipment {
		ks.Equipment = append(ks.Equipment, scheduler.Equipment{ID: e.ID, StationID: e.StationID, Name: e.Name})
	}
	for _, o := range open {
		for _, c := range o.Courses {
			if c.State != orders.CourseFired || c.FiredAt == nil || c.TargetAt == nil {
				continue
			}
			sc := scheduler.Course{
				OrderID: o.ID, Label: o.Label(), Course: c.Course, FiredAt: *c.FiredAt, Target: *c.TargetAt,
				Urgent: c.Urgent, Allergy: o.Allergy, Takeaway: o.Kind == orders.KindTakeaway, Covers: o.Covers,
			}
			for _, it := range o.ItemsIn(c.Course) {
				sc.Items = append(sc.Items, scheduler.Item{
					ID: it.ID, DishID: it.DishID, DishName: it.DishName, Modifiers: it.Modifiers,
					StationID: it.StationID, EquipmentID: it.EquipmentID, Capacity: it.BatchCapacity,
					Cook: it.Timing.Cook, Rest: it.Timing.Rest, Plate: it.Timing.Plate,
					HoldMax: time.Duration(it.HoldSeconds) * time.Second, Batchable: it.Batchable,
					State: it.State, StateAt: it.StateAt,
				})
			}
			ks.Courses = append(ks.Courses, sc)
		}
	}
	return ks
}

// Kitchen returns open orders and a fresh plan. The plan is computed on
// every request, so it always reflects the latest change.
func (s *Service) Kitchen(ctx context.Context, p *auth.Principal) (KitchenView, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return KitchenView{}, err
	}
	var view KitchenView
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		setup, err := menuSetup(ctx, tx)
		if err != nil {
			return err
		}
		open, err := tx.Orders().ListOrders(ctx, store.OrderFilter{OpenOnly: true})
		if err != nil {
			return err
		}
		set, err := s.Scheduler().current(ctx, tx)
		if err != nil {
			return err
		}
		now := s.clk.Now()
		cfg := set.Value.Config(set.Version.ID, clock.Dublin)
		view = KitchenView{
			At: now, Orders: open, Stations: setup.Stations, ReplanSeconds: set.Value.ReplanSeconds,
			Plan: scheduler.PlanKitchen(schedulerState(setup, open), cfg, now),
		}
		return nil
	})
	return view, err
}
