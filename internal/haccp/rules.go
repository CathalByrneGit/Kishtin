package haccp

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Rules are the restaurant's food-safety rules around the limits.
//
// One rule is not customisable: a failed reading always needs a corrective
// action.
type Rules struct {
	// BorderlineNeedsAction makes a borderline reading need a corrective
	// action, like a fail.
	BorderlineNeedsAction bool `json:"borderline_needs_action"`
	// ListedNamesOnly makes fridges, freezers, food categories and cleaning
	// tasks come from the managed lists, when a list has entries. This stops
	// "Walkin" and "Walk-in" becoming two fridges.
	ListedNamesOnly bool `json:"listed_names_only"`
}

// BuiltinRulesID is the version ID of the built-in rules.
const BuiltinRulesID = "builtin-rules-v1"

// DefaultRules returns the built-in rules.
func DefaultRules() Rules {
	return Rules{BorderlineNeedsAction: true, ListedNamesOnly: true}
}

// NeedsAction reports whether a record with outcome o can only be saved
// with a corrective action.
func (r Rules) NeedsAction(o Outcome) bool {
	switch o {
	case OutcomeFail:
		return true
	case OutcomeBorderline:
		return r.BorderlineNeedsAction
	}
	return false
}

// Unit is a fridge or freezer.
type Unit struct {
	Name string `json:"name"`
	// Kind is fridge or freezer.
	Kind RecordType `json:"kind"`
	// NonFood marks a unit that holds no food, such as a drinks fridge.
	NonFood bool `json:"non_food,omitempty"`
	// Checks are the temperature checks due each day, in time order. Empty
	// means the default: AM (before 12:00) and PM.
	Checks []CheckTime `json:"checks,omitempty"`
}

// CheckTime is one daily temperature check on a unit. A check covers the
// time from its From time (local, HH:MM) until the next check's From time;
// the first check also covers the start of the day. A check not logged by
// the time the next one starts is missed.
type CheckTime struct {
	Label string `json:"label"`
	From  string `json:"from"`
}

// DefaultChecks are the AM and PM checks.
func DefaultChecks() []CheckTime {
	return []CheckTime{{Label: "AM", From: "00:00"}, {Label: "PM", From: "12:00"}}
}

// ChecksOrDefault returns the unit's checks, or the default AM and PM.
func (u Unit) ChecksOrDefault() []CheckTime {
	if len(u.Checks) == 0 {
		return DefaultChecks()
	}
	return u.Checks
}

// ParseClock reads a local HH:MM time as minutes after midnight.
func ParseClock(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a time like 08:30", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// ActionPreset is a quick-pick corrective action offered for a record type.
type ActionPreset struct {
	Type RecordType `json:"type"`
	Text string     `json:"text"`
}

// CleaningTask is a cleaning task staff tick off.
type CleaningTask struct {
	Task string `json:"task"`
	Area string `json:"area,omitempty"`
}

// Lists are the names staff choose from when logging records.
type Lists struct {
	Units          []Unit         `json:"units"`
	FoodCategories []string       `json:"food_categories"`
	CleaningTasks  []CleaningTask `json:"cleaning_tasks"`
	// CorrectiveActions are the quick-pick actions offered when a reading
	// is not a pass. Staff can always write their own.
	CorrectiveActions []ActionPreset `json:"corrective_actions" required:"false"`
}

// BuiltinListsID is the version ID of the built-in lists.
const BuiltinListsID = "builtin-lists-v1"

// DefaultLists returns the starting lists, taken from the browser
// prototype. A manager changes them in settings.
func DefaultLists() Lists {
	tasks := []string{
		"Food-contact surfaces sanitised",
		"Chopping boards washed & sanitised",
		"Fridge handles & seals wiped",
		"Floors swept & mopped",
		"Bins emptied & cleaned",
		"Hand-wash sinks stocked (soap, towels)",
	}
	l := Lists{
		Units: []Unit{
			{Name: "Walk-in fridge", Kind: RecordFridge},
			{Name: "Prep fridge", Kind: RecordFridge},
			{Name: "Chest freezer", Kind: RecordFreezer},
		},
		FoodCategories: []string{},
	}
	for _, t := range tasks {
		l.CleaningTasks = append(l.CleaningTasks, CleaningTask{Task: t})
	}
	presets := map[RecordType][]string{
		RecordFridge:   {"Moved food to a working unit", "Discarded high-risk food", "Adjusted thermostat, recheck in 30 min", "Called fridge engineer"},
		RecordDelivery: {"Rejected delivery", "Informed supplier", "Accepted, used immediately"},
		RecordCook:     {"Cooked longer and re-probed", "Discarded"},
		RecordCooling:  {"Under 10°C at 2.5 h: refrigerated", "Over 10°C at 2.5 h: discarded", "Over 10°C at 2.5 h: served immediately"},
		RecordHotHold:  {"Reheated to 70°C+", "Discarded (held under 63°C over 2 h)"},
	}
	presets[RecordFreezer] = presets[RecordFridge]
	presets[RecordReheat] = presets[RecordCook]
	for _, t := range RecordTypes {
		for _, text := range presets[t] {
			l.CorrectiveActions = append(l.CorrectiveActions, ActionPreset{Type: t, Text: text})
		}
	}
	return l
}

// emptyLists has every list present and empty.
func emptyLists() Lists {
	return Lists{Units: []Unit{}, FoodCategories: []string{}, CleaningTasks: []CleaningTask{}, CorrectiveActions: []ActionPreset{}}
}

// Normalize trims every name and makes nil lists empty.
func (l Lists) Normalize() Lists {
	out := emptyLists()
	for _, u := range l.Units {
		nu := Unit{Name: strings.TrimSpace(u.Name), Kind: RecordType(strings.TrimSpace(string(u.Kind))), NonFood: u.NonFood}
		for _, c := range u.Checks {
			nu.Checks = append(nu.Checks, CheckTime{Label: strings.TrimSpace(c.Label), From: strings.TrimSpace(c.From)})
		}
		out.Units = append(out.Units, nu)
	}
	for _, a := range l.CorrectiveActions {
		out.CorrectiveActions = append(out.CorrectiveActions, ActionPreset{Type: a.Type, Text: strings.TrimSpace(a.Text)})
	}
	for _, c := range l.FoodCategories {
		out.FoodCategories = append(out.FoodCategories, strings.TrimSpace(c))
	}
	for _, t := range l.CleaningTasks {
		out.CleaningTasks = append(out.CleaningTasks, CleaningTask{Task: strings.TrimSpace(t.Task), Area: strings.TrimSpace(t.Area)})
	}
	return out
}

// Validate checks that names are set and not repeated (ignoring case).
func (l Lists) Validate() error {
	unique := func(list string, names []string) error {
		seen := map[string]bool{}
		for _, n := range names {
			if n == "" {
				return fmt.Errorf("an entry in %s has no name", list)
			}
			k := strings.ToLower(n)
			if seen[k] {
				return fmt.Errorf("%q is in %s twice", n, list)
			}
			seen[k] = true
		}
		return nil
	}
	var units, tasks []string
	for _, u := range l.Units {
		if u.Kind != RecordFridge && u.Kind != RecordFreezer {
			return fmt.Errorf("unit %q must be a fridge or a freezer", u.Name)
		}
		if err := validateChecks(u); err != nil {
			return err
		}
		units = append(units, u.Name)
	}
	seenAction := map[string]bool{}
	for _, a := range l.CorrectiveActions {
		if !a.Type.Valid() {
			return fmt.Errorf("corrective action %q has an unknown record type %q", a.Text, a.Type)
		}
		if a.Text == "" {
			return errors.New("a corrective action preset has no text")
		}
		k := string(a.Type) + "\x00" + strings.ToLower(a.Text)
		if seenAction[k] {
			return fmt.Errorf("corrective action %q is listed twice for %s", a.Text, a.Type)
		}
		seenAction[k] = true
	}
	for _, t := range l.CleaningTasks {
		tasks = append(tasks, t.Task)
	}
	return errors.Join(
		unique("units", units),
		unique("food_categories", l.FoodCategories),
		unique("cleaning_tasks", tasks),
	)
}

func validateChecks(u Unit) error {
	prev := -1
	labels := map[string]bool{}
	for _, c := range u.Checks {
		if c.Label == "" {
			return fmt.Errorf("a check on %q has no label", u.Name)
		}
		if labels[strings.ToLower(c.Label)] {
			return fmt.Errorf("%q has two checks called %q", u.Name, c.Label)
		}
		labels[strings.ToLower(c.Label)] = true
		m, err := ParseClock(c.From)
		if err != nil {
			return fmt.Errorf("check %q on %q: %w", c.Label, u.Name, err)
		}
		if m <= prev {
			return fmt.Errorf("the checks on %q must be in time order", u.Name)
		}
		prev = m
	}
	return nil
}

// Unit returns the listed unit with this name, ignoring case.
func (l Lists) Unit(name string) (Unit, bool) {
	for _, u := range l.Units {
		if strings.EqualFold(u.Name, name) {
			return u, true
		}
	}
	return Unit{}, false
}

func (l Lists) unitNames(kind RecordType) []string {
	var out []string
	for _, u := range l.Units {
		if u.Kind == kind {
			out = append(out, u.Name)
		}
	}
	return out
}

func (l Lists) taskNames() []string {
	var out []string
	for _, t := range l.CleaningTasks {
		out = append(out, t.Task)
	}
	return out
}

// match finds name in names ignoring case. With strict set, a name that is
// not listed is an error when the list has entries.
func match(name string, names []string, what string, strict bool) (string, error) {
	if name == "" || len(names) == 0 {
		return name, nil
	}
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return n, nil
		}
	}
	if !strict {
		return name, nil
	}
	return "", fmt.Errorf("%q is not on the %s list. Use one of: %s. A manager can add it to the list", name, what, strings.Join(names, ", "))
}

// Apply checks the names in a reading against the lists and replaces them
// with the listed spelling. A cleaning task's area is filled in from the
// list when the reading has none.
func (l Lists) Apply(t RecordType, r Reading, rules Rules) (Reading, error) {
	strict := rules.ListedNamesOnly
	var err error
	if t == RecordFridge || t == RecordFreezer {
		if r.Unit, err = match(r.Unit, l.unitNames(t), string(t), strict); err != nil {
			return r, err
		}
	}
	if r.FoodCategory, err = match(r.FoodCategory, l.FoodCategories, "food category", strict); err != nil {
		return r, err
	}
	if t == RecordCleaning {
		if r.Task, err = match(r.Task, l.taskNames(), "cleaning task", strict); err != nil {
			return r, err
		}
		for _, ct := range l.CleaningTasks {
			if ct.Task == r.Task && r.Area == "" {
				r.Area = ct.Area
			}
		}
	}
	return r, nil
}

// ApplyScope checks a limit scope's name against the lists and returns it
// with the listed spelling.
func (l Lists) ApplyScope(check CheckType, s Scope, rules Rules) (Scope, error) {
	var err error
	switch s.Kind {
	case ScopeUnit:
		kind := RecordFridge
		if check == CheckFreezer {
			kind = RecordFreezer
		}
		s.Ref, err = match(s.Ref, l.unitNames(kind), string(kind), rules.ListedNamesOnly)
	case ScopeCategory:
		s.Ref, err = match(s.Ref, l.FoodCategories, "food category", rules.ListedNamesOnly)
	}
	return s, err
}
