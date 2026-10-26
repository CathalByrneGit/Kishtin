package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/scheduler"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// SnapshotPlan is what a plan snapshot keeps: each station's
// recommendations and each fired course's status, with their reasons.
// Items in progress and per-item timings are left out; they follow from
// the order events.
type SnapshotPlan struct {
	Stations []SnapshotStation        `json:"stations"`
	Courses  []scheduler.CourseStatus `json:"courses"`
}

// SnapshotStation is one station's recommendations in a snapshot.
type SnapshotStation struct {
	StationID string             `json:"station_id"`
	Name      string             `json:"name"`
	Actions   []scheduler.Action `json:"actions"`
}

// snapshotOf trims a plan to what a snapshot keeps.
func snapshotOf(p scheduler.Plan) SnapshotPlan {
	out := SnapshotPlan{Stations: []SnapshotStation{}, Courses: p.Courses}
	if out.Courses == nil {
		out.Courses = []scheduler.CourseStatus{}
	}
	for _, st := range p.Stations {
		if len(st.Actions) > 0 {
			out.Stations = append(out.Stations, SnapshotStation{StationID: st.StationID, Name: st.Name, Actions: st.Actions})
		}
	}
	return out
}

// planHash identifies what a plan recommends, without its wording, so a
// plan whose reasons only count the minutes is not kept again.
func planHash(sp SnapshotPlan, strategy, version string) string {
	type act struct {
		Station, Dish string
		Mods          []string
		Qty           int
		Items         []string
		Start         time.Time
		Now, Late     bool
	}
	type course struct {
		Order         string
		Course        menu.Course
		Target, Ready time.Time
		Status, Cause string
	}
	var key struct {
		Strategy, Version string
		Actions           []act
		Courses           []course
	}
	key.Strategy, key.Version = strategy, version
	for _, st := range sp.Stations {
		for _, a := range st.Actions {
			key.Actions = append(key.Actions, act{a.StationID, a.DishID, a.Modifiers, a.Quantity, a.ItemIDs, a.StartAt, a.Now, a.Late})
		}
	}
	for _, c := range sp.Courses {
		key.Courses = append(key.Courses, course{c.OrderID, c.Course, c.Target, c.PlannedReady, c.Status, c.Cause})
	}
	b, _ := json.Marshal(key)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// RecordPlan plans the kitchen and keeps the plan if it differs from the
// last one kept. trigger says what prompted it: "timer", or the kind of
// change. It reports whether a snapshot was kept.
func (s *Service) RecordPlan(ctx context.Context, trigger string) (bool, error) {
	view, err := s.kitchenNow(ctx, s.st)
	if err != nil {
		return false, err
	}
	sp := snapshotOf(view.Plan)
	hash := planHash(sp, view.Plan.Strategy, view.Plan.SettingsVersion)
	last := s.lastPlan.Load()
	if last == nil {
		prev, err := s.st.Activity().LastPlanSnapshot(ctx)
		switch {
		case err == nil:
			last = &prev.Hash
		case !isNotFound(err):
			return false, err
		}
	}
	if last != nil && *last == hash {
		return false, nil
	}
	body, err := json.Marshal(sp)
	if err != nil {
		return false, fmt.Errorf("encode plan snapshot: %w", err)
	}
	snap := store.PlanSnapshot{
		ID: newID(), At: view.At, Trigger: trigger, Strategy: view.Plan.Strategy,
		SettingsVersion: view.Plan.SettingsVersion, Hash: hash, Body: body,
	}
	if err := s.st.Activity().AppendPlanSnapshot(ctx, snap); err != nil {
		return false, fmt.Errorf("keep plan snapshot: %w", err)
	}
	s.lastPlan.Store(&hash)
	return true, nil
}

// planTriggers are the changes that can change the plan.
var planTriggers = map[string]bool{events.KindOrder: true, events.KindMenu: true, events.KindSettings: true}

// RunPlanRecorder keeps a snapshot of the scheduler's plan whenever it
// changes: after every order, menu or settings change, and on the
// scheduler's re-plan timer, so courses going late are caught too. It
// also deletes voice recordings past their keep date. It runs until ctx
// ends.
func (s *Service) RunPlanRecorder(ctx context.Context, log *slog.Logger) {
	var ch <-chan events.Event
	if s.events != nil {
		sub, stop := s.events.Subscribe()
		defer stop()
		ch = sub
	}
	every := func() time.Duration {
		set, err := s.Scheduler().current(ctx, s.st)
		if err != nil || set.Value.ReplanSeconds <= 0 {
			return 15 * time.Second
		}
		return time.Duration(set.Value.ReplanSeconds) * time.Second
	}
	tick := time.NewTimer(every())
	defer tick.Stop()
	purge := time.NewTicker(time.Hour)
	defer purge.Stop()
	record := func(trigger string) {
		if _, err := s.RecordPlan(ctx, trigger); err != nil && ctx.Err() == nil {
			log.Warn("keep plan snapshot", "err", err)
		}
	}
	record("start")
	if _, err := s.PurgeVoiceAudio(ctx); err != nil {
		log.Warn("delete old voice recordings", "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				ch = nil
				continue
			}
			if planTriggers[ev.Kind] {
				record(ev.Kind)
			}
		case <-tick.C:
			record("timer")
			tick.Reset(every())
		case <-purge.C:
			if _, err := s.PurgeVoiceAudio(ctx); err != nil {
				log.Warn("delete old voice recordings", "err", err)
			}
		}
	}
}

// PlanSnapshotView is a kept plan, decoded.
type PlanSnapshotView struct {
	ID              string       `json:"id"`
	At              time.Time    `json:"at"`
	Trigger         string       `json:"trigger"`
	Strategy        string       `json:"strategy"`
	SettingsVersion string       `json:"settings_version"`
	Plan            SnapshotPlan `json:"plan"`
}

// MaxSnapshotLimit caps one page of plan snapshots.
const MaxSnapshotLimit = 2000

// PlanSnapshots lists the plans kept between from and to, oldest first.
func (s *Service) PlanSnapshots(ctx context.Context, p *auth.Principal, from, to time.Time, limit int) ([]PlanSnapshotView, error) {
	if err := s.allow(p, auth.ActRunPass); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > MaxSnapshotLimit {
		limit = MaxSnapshotLimit
	}
	snaps, err := s.st.Activity().ListPlanSnapshots(ctx, from, to, limit)
	if err != nil {
		return nil, err
	}
	out := make([]PlanSnapshotView, 0, len(snaps))
	for _, sn := range snaps {
		v := PlanSnapshotView{ID: sn.ID, At: sn.At, Trigger: sn.Trigger, Strategy: sn.Strategy, SettingsVersion: sn.SettingsVersion}
		if err := json.Unmarshal(sn.Body, &v.Plan); err != nil {
			return nil, fmt.Errorf("decode plan snapshot %s: %w", sn.ID, err)
		}
		out = append(out, v)
	}
	return out, nil
}
