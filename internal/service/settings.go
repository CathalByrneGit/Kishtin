package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/scheduler"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// Versioned is the value of a settings group and the version it came from.
type Versioned[T any] struct {
	Version store.SettingsVersion `json:"version"`
	Value   T                     `json:"value"`
	// Builtin is set when the value is Kishtin's own default (or, for system
	// settings, comes from kishtin.toml) rather than a saved change.
	Builtin bool `json:"builtin"`
}

// SettingsChange is a proposed new value for a settings group.
type SettingsChange[T any] struct {
	Value  T      `json:"value"`
	Reason string `json:"reason"`
	// PlanRef cites the HACCP plan section or validation behind a change
	// that makes food safety less strict.
	PlanRef string `json:"plan_ref,omitempty"`
}

// SettingsPreview shows what a settings change would do, before it is
// saved.
type SettingsPreview[T any] struct {
	Current  Versioned[T] `json:"current"`
	Proposed T            `json:"proposed"`
	// Changes lists each field that would change.
	Changes []string `json:"changes"`
	// LessStrict lists how the change makes food safety less strict. Such a
	// change needs a plan reference.
	LessStrict []string `json:"less_strict,omitempty"`
	// Notes lists effects on limits and recent records.
	Notes []string `json:"notes"`
	// Problem is set when the change cannot be saved as it is.
	Problem string `json:"problem,omitempty"`
}

// groupSpec describes one settings group.
type groupSpec[T any] struct {
	kind      settings.Kind
	builtinID string
	builtin   func(s *Service) T
	// seeded groups store their built-in value as a version on first start.
	seeded    bool
	normalize func(T) T
	validate  func(T) error
	// lessStrict lists how next is less strict than the baseline.
	lessStrict func(cur, next T) []string
	notes      func(ctx context.Context, s *Service, st store.Store, cur, next T) ([]string, error)
	edit       auth.Action
}

// Group is a versioned settings group: reference guidance, food-safety
// rules, managed lists or system settings.
type Group[T any] struct {
	s    *Service
	spec groupSpec[T]
}

// Guidance is the reference guidance limits are measured against.
func (s *Service) Guidance() *Group[haccp.Guidance] {
	return &Group[haccp.Guidance]{s: s, spec: groupSpec[haccp.Guidance]{
		kind: settings.KindGuidance, builtinID: haccp.BuiltinGuidanceID, seeded: true,
		builtin:    func(*Service) haccp.Guidance { return haccp.FSAIGuidance() },
		normalize:  haccp.Guidance.Normalize,
		validate:   haccp.Guidance.Validate,
		lessStrict: func(_, next haccp.Guidance) []string { return next.LooserThanFSAI() },
		notes:      guidanceNotes,
		edit:       auth.ActManageLimits,
	}}
}

// Rules is the food-safety rules.
func (s *Service) Rules() *Group[haccp.Rules] {
	return &Group[haccp.Rules]{s: s, spec: groupSpec[haccp.Rules]{
		kind: settings.KindRules, builtinID: haccp.BuiltinRulesID, seeded: true,
		builtin:   func(*Service) haccp.Rules { return haccp.DefaultRules() },
		normalize: func(r haccp.Rules) haccp.Rules { return r },
		validate:  func(haccp.Rules) error { return nil },
		lessStrict: func(_, next haccp.Rules) []string {
			if !next.BorderlineNeedsAction {
				return []string{"borderline readings could be saved without a corrective action"}
			}
			return nil
		},
		notes: func(_ context.Context, _ *Service, _ store.Store, _, next haccp.Rules) ([]string, error) {
			out := []string{"A failed reading always needs a corrective action. That rule cannot be changed."}
			if !next.ListedNamesOnly {
				out = append(out, "Names not on the lists will be accepted as typed.")
			}
			return out, nil
		},
		edit: auth.ActManageLimits,
	}}
}

// Lists is the managed lists of fridges, freezers, food categories and
// cleaning tasks.
func (s *Service) Lists() *Group[haccp.Lists] {
	return &Group[haccp.Lists]{s: s, spec: groupSpec[haccp.Lists]{
		kind: settings.KindLists, builtinID: haccp.BuiltinListsID, seeded: true,
		builtin:    func(*Service) haccp.Lists { return haccp.DefaultLists() },
		normalize:  haccp.Lists.Normalize,
		validate:   haccp.Lists.Validate,
		lessStrict: func(_, _ haccp.Lists) []string { return nil },
		notes:      listNotes,
		edit:       auth.ActManageLimits,
	}}
}

// System is the system settings. Until a change is saved, they come from
// kishtin.toml.
func (s *Service) System() *Group[settings.System] {
	return &Group[settings.System]{s: s, spec: groupSpec[settings.System]{
		kind:       settings.KindSystem,
		builtin:    func(s *Service) settings.System { return s.systemDefaults },
		normalize:  func(v settings.System) settings.System { return v },
		validate:   settings.System.Validate,
		lessStrict: func(_, _ settings.System) []string { return nil },
		notes: func(context.Context, *Service, store.Store, settings.System, settings.System) ([]string, error) {
			return []string{"The backup folder, listen address and TLS stay in kishtin.toml."}, nil
		},
		edit: auth.ActManageSettings,
	}}
}

// Scheduler is the cook scheduler's settings: its strategy, batching,
// priorities and buffers.
func (s *Service) Scheduler() *Group[scheduler.Settings] {
	return &Group[scheduler.Settings]{s: s, spec: groupSpec[scheduler.Settings]{
		kind: settings.KindScheduler, builtinID: scheduler.BuiltinSettingsID, seeded: true,
		builtin:    func(*Service) scheduler.Settings { return scheduler.DefaultSettings() },
		normalize:  scheduler.Settings.Normalize,
		validate:   scheduler.Settings.Validate,
		lessStrict: func(_, _ scheduler.Settings) []string { return nil },
		notes:      schedulerNotes,
		edit:       auth.ActManageSettings,
	}}
}

// schedulerNotes checks station buffers name real stations, and says what
// the change does to service.
func schedulerNotes(ctx context.Context, _ *Service, st store.Store, cur, next scheduler.Settings) ([]string, error) {
	stations, err := st.Menu().ListStations(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, x := range stations {
		names[x.ID] = x.Name
	}
	var out []string
	for _, b := range next.StationBuffers {
		if names[b.StationID] == "" {
			out = append(out, fmt.Sprintf("Station %s does not exist; its buffer will have no effect.", b.StationID))
		}
	}
	if cur.Strategy != next.Strategy {
		out = append(out, "Every screen switches to the new strategy on its next plan, within a few seconds.")
	}
	out = append(out, "Plans already shown are not changed; the next plan uses the new settings.")
	return out, nil
}

var builtinTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func (g *Group[T]) builtinVersion() (store.SettingsVersion, error) {
	body, err := json.Marshal(g.spec.builtin(g.s))
	if err != nil {
		return store.SettingsVersion{}, fmt.Errorf("encode built-in %s: %w", g.spec.kind, err)
	}
	v := store.SettingsVersion{
		ID: g.spec.builtinID, Kind: g.spec.kind, Body: body,
		CreatedBy: haccp.SystemUser, CreatedAt: builtinTime, Reason: "Kishtin default",
	}
	if !g.spec.seeded {
		v.Reason = "From kishtin.toml"
	}
	return v, nil
}

// seed stores the built-in version if the group has none.
func (g *Group[T]) seed(ctx context.Context, tx store.Store) error {
	if !g.spec.seeded {
		return nil
	}
	_, err := tx.Settings().GetSettings(ctx, g.spec.builtinID)
	if err == nil {
		return nil
	}
	if !isNotFound(err) {
		return fmt.Errorf("check built-in %s: %w", g.spec.kind, err)
	}
	v, err := g.builtinVersion()
	if err != nil {
		return err
	}
	if err := tx.Settings().AppendSettings(ctx, v); err != nil {
		return fmt.Errorf("store built-in %s: %w", g.spec.kind, err)
	}
	return nil
}

// current reads the version in force through st.
func (g *Group[T]) current(ctx context.Context, st store.Store) (Versioned[T], error) {
	versions, err := st.Settings().ListSettings(ctx, g.spec.kind)
	if err != nil {
		return Versioned[T]{}, err
	}
	superseded := map[string]bool{}
	for _, v := range versions {
		if v.Supersedes != "" {
			superseded[v.Supersedes] = true
		}
	}
	var head *store.SettingsVersion
	for k := range versions {
		v := &versions[k]
		if !superseded[v.ID] && (head == nil || v.CreatedAt.After(head.CreatedAt)) {
			head = v
		}
	}
	if head == nil {
		v, err := g.builtinVersion()
		if err != nil {
			return Versioned[T]{}, err
		}
		return Versioned[T]{Version: v, Value: g.spec.builtin(g.s), Builtin: true}, nil
	}
	var val T
	if err := json.Unmarshal(head.Body, &val); err != nil {
		return Versioned[T]{}, fmt.Errorf("decode %s settings %s: %w", g.spec.kind, head.ID, err)
	}
	return Versioned[T]{Version: *head, Value: val, Builtin: head.ID == g.spec.builtinID}, nil
}

// Current returns the settings in force. Any staff member can read them.
func (g *Group[T]) Current(ctx context.Context, p *auth.Principal) (Versioned[T], error) {
	if err := allow(p, auth.ActViewHACCP); err != nil {
		return Versioned[T]{}, err
	}
	return g.current(ctx, g.s.st)
}

// History returns every saved version, oldest first.
func (g *Group[T]) History(ctx context.Context, p *auth.Principal) ([]Versioned[T], error) {
	if err := allow(p, auth.ActViewHACCP); err != nil {
		return nil, err
	}
	versions, err := g.s.st.Settings().ListSettings(ctx, g.spec.kind)
	if err != nil {
		return nil, err
	}
	out := []Versioned[T]{}
	for _, v := range versions {
		var val T
		if err := json.Unmarshal(v.Body, &val); err != nil {
			return nil, fmt.Errorf("decode %s settings %s: %w", g.spec.kind, v.ID, err)
		}
		out = append(out, Versioned[T]{Version: v, Value: val, Builtin: v.ID == g.spec.builtinID})
	}
	return out, nil
}

// check builds the preview for a change, read through st.
func (g *Group[T]) check(ctx context.Context, st store.Store, ch SettingsChange[T]) (SettingsPreview[T], error) {
	cur, err := g.current(ctx, st)
	if err != nil {
		return SettingsPreview[T]{}, err
	}
	next := g.spec.normalize(ch.Value)
	pv := SettingsPreview[T]{Current: cur, Proposed: next, Notes: []string{}}
	if pv.Changes, err = settings.Changes(cur.Value, next); err != nil {
		return pv, err
	}
	if err := g.spec.validate(next); err != nil {
		pv.Problem = err.Error()
		return pv, nil
	}
	pv.LessStrict = g.spec.lessStrict(cur.Value, next)
	if pv.Notes, err = g.spec.notes(ctx, g.s, st, cur.Value, next); err != nil {
		return pv, err
	}
	switch {
	case len(pv.Changes) == 0:
		pv.Problem = "nothing would change"
	case strings.TrimSpace(ch.Reason) == "":
		pv.Problem = "a settings change needs a reason"
	case len(pv.LessStrict) > 0 && strings.TrimSpace(ch.PlanRef) == "":
		pv.Problem = "this change makes food safety less strict (" + strings.Join(pv.LessStrict, "; ") +
			"): add the HACCP plan section or validation that supports it"
	}
	return pv, nil
}

// Preview shows what a change would do without saving it.
func (g *Group[T]) Preview(ctx context.Context, p *auth.Principal, ch SettingsChange[T]) (SettingsPreview[T], error) {
	if err := allow(p, g.spec.edit); err != nil {
		return SettingsPreview[T]{}, err
	}
	return g.check(ctx, g.s.st, ch)
}

// Save stores a change as a new version. It refuses anything Preview
// reports as a problem.
func (g *Group[T]) Save(ctx context.Context, p *auth.Principal, ch SettingsChange[T]) (Versioned[T], error) {
	if err := allow(p, g.spec.edit); err != nil {
		return Versioned[T]{}, err
	}
	var out Versioned[T]
	err := g.s.st.WithTx(ctx, func(tx store.Store) error {
		pv, err := g.check(ctx, tx, ch)
		if err != nil {
			return err
		}
		if pv.Problem != "" {
			return newErr(KindInvalid, "%s", pv.Problem)
		}
		body, err := json.Marshal(pv.Proposed)
		if err != nil {
			return fmt.Errorf("encode %s settings: %w", g.spec.kind, err)
		}
		v := store.SettingsVersion{
			ID: newID(), Kind: g.spec.kind, Body: body,
			CreatedBy: p.User.ID, CreatedAt: g.s.clk.Now(),
			Reason: strings.TrimSpace(ch.Reason), PlanRef: strings.TrimSpace(ch.PlanRef),
		}
		// A system default from kishtin.toml is not stored, so there is
		// nothing to supersede.
		if !pv.Current.Builtin || g.spec.seeded {
			v.Supersedes = pv.Current.Version.ID
		}
		if err := tx.Settings().AppendSettings(ctx, v); err != nil {
			return mapStoreErr(err, "a newer version of these settings")
		}
		out = Versioned[T]{Version: v, Value: pv.Proposed}
		return nil
	})
	if err == nil {
		g.s.publish(events.KindSettings, string(g.spec.kind))
	}
	return out, err
}

// guidanceNotes lists current limits whose standing against the guidance
// would change.
func guidanceNotes(ctx context.Context, _ *Service, st store.Store, cur, next haccp.Guidance) ([]string, error) {
	history, err := st.HACCP().ListLimitVersions(ctx)
	if err != nil {
		return nil, err
	}
	out := []string{"Saved records keep their result. Limits stay as they are; only the comparison changes."}
	for _, lv := range haccp.Current(history) {
		before, after := lv.LooserThanGuidance(cur), lv.LooserThanGuidance(next)
		label := fmt.Sprintf("%s limit (%s)", lv.Check, lv.Scope)
		switch {
		case len(before) == 0 && len(after) > 0:
			out = append(out, fmt.Sprintf("The %s would be below the new guidance: %s.", label, strings.Join(after, "; ")))
		case len(before) > 0 && len(after) == 0:
			out = append(out, fmt.Sprintf("The %s would no longer be below the guidance.", label))
		}
	}
	return out, nil
}

// listNotes lists limits and recent records that use names the new lists
// do not have.
func listNotes(ctx context.Context, s *Service, st store.Store, _, next haccp.Lists) ([]string, error) {
	out := []string{"Saved records keep the names they were logged with."}
	rules, err := s.Rules().current(ctx, st)
	if err != nil {
		return nil, err
	}
	strict := haccp.Rules{ListedNamesOnly: true}
	history, err := st.HACCP().ListLimitVersions(ctx)
	if err != nil {
		return nil, err
	}
	for _, lv := range haccp.Current(history) {
		if _, err := next.ApplyScope(lv.Check, lv.Scope, strict); err != nil {
			out = append(out, fmt.Sprintf("The %s limit for %q names something not on the new lists. It still applies to readings with that exact name.", lv.Check, lv.Scope.Ref))
		}
	}
	recs, err := st.HACCP().ListRecords(ctx, store.RecordFilter{Newest: true, Limit: 500})
	if err != nil {
		return nil, err
	}
	var unlisted []string
	for _, r := range recs {
		if _, err := next.Apply(r.Type, r.Reading, strict); err != nil {
			name := subject(r)
			if r.Reading.FoodCategory != "" {
				name += " (" + r.Reading.FoodCategory + ")"
			}
			if !slices.Contains(unlisted, name) {
				unlisted = append(unlisted, name)
			}
		}
	}
	if len(unlisted) > 0 {
		verb := "would be refused"
		if !rules.Value.ListedNamesOnly {
			verb = "would still be accepted, because listed names only is off"
		}
		out = append(out, fmt.Sprintf("Recent records use names not on the new lists, so new readings like them %s: %s.", verb, strings.Join(unlisted, ", ")))
	}
	return out, nil
}
