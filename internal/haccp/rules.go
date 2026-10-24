package haccp

import (
	"errors"
	"fmt"
	"strings"
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
}

// BuiltinListsID is the version ID of the built-in (empty) lists.
const BuiltinListsID = "builtin-lists-v1"

// DefaultLists returns empty lists: any name is accepted until a manager
// adds entries.
func DefaultLists() Lists {
	return Lists{Units: []Unit{}, FoodCategories: []string{}, CleaningTasks: []CleaningTask{}}
}

// Normalize trims every name and makes nil lists empty.
func (l Lists) Normalize() Lists {
	out := DefaultLists()
	for _, u := range l.Units {
		out.Units = append(out.Units, Unit{Name: strings.TrimSpace(u.Name), Kind: RecordType(strings.TrimSpace(string(u.Kind))), NonFood: u.NonFood})
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
		units = append(units, u.Name)
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
