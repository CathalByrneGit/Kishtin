package auth

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Permissions are the restaurant's changes to who may do what: a grant per
// action, replacing Kishtin's default for that action. Actions without a
// grant keep the default.
type Permissions struct {
	Grants []Grant `json:"grants"`
}

// Grant says who may take one action.
type Grant struct {
	Action Action `json:"action"`
	// Roles may take the action. Managers always may.
	Roles []Role `json:"roles"`
	// Users are named staff who may take the action whatever their role,
	// by user ID.
	Users []string `json:"users,omitempty"`
	// PIN means a PIN login is enough. Otherwise the action needs a
	// password login (or a user API key).
	PIN bool `json:"pin,omitempty"`
}

// ActionInfo describes an action for the permissions screen.
type ActionInfo struct {
	Action Action `json:"action"`
	Group  string `json:"group"`
	Label  string `json:"label"`
	// Roles and PIN are Kishtin's default.
	Roles []Role `json:"roles"`
	PIN   bool   `json:"pin"`
	// Fixed actions are always managers with a password, and cannot be
	// given to anyone else.
	Fixed bool `json:"fixed,omitempty"`
	// Why says why a fixed action is fixed.
	Why string `json:"why,omitempty"`
}

// fixed actions cannot be delegated: changing staff, keys and permissions
// would let someone give themselves any access, and fitness to work records
// are health information.
var fixed = map[Action]string{
	ActManageUsers:       "Adding staff and setting roles could give anyone any access.",
	ActManageKeys:        "An API key acts as a member of staff.",
	ActManagePermissions: "Changing permissions could give anyone any access.",
	ActFitness:           "Fitness to work records are health information.",
}

// Fixed reports whether an action is always managers with a password.
func Fixed(a Action) bool { _, ok := fixed[a]; return ok }

var actionInfo = []struct {
	a            Action
	group, label string
}{
	{ActViewHACCP, "HACCP", "See HACCP records and Today"},
	{ActLogHACCP, "HACCP", "Log checks"},
	{ActBackdate, "HACCP", "Log a check done earlier, or catch up missed checks"},
	{ActLogSignedOff, "HACCP", "Add or correct records in a period already signed off"},
	{ActInspect, "HACCP", "Log hygiene inspections and training, import paper pages"},
	{ActSignOff, "HACCP", "Sign off records"},
	{ActExportHACCP, "HACCP", "Exports and record books"},
	{ActFitness, "HACCP", "Fitness to work"},
	{ActViewKitchen, "Kitchen", "See the pass, stations and orders"},
	{ActTakeOrders, "Kitchen", "Take orders"},
	{ActRunPass, "Kitchen", "Run the pass: fire, serve and void"},
	{ActWorkStation, "Kitchen", "Work a station: start and finish items"},
	{ActSetAvailability, "Kitchen", "Mark dishes sold out or back on"},
	{ActManageMenu, "Setup", "Change the menu, stations and equipment"},
	{ActManageLimits, "Setup", "Change limits, guidance, food-safety rules, lists and record books"},
	{ActManageSettings, "Setup", "Change system settings, the scheduler and Square"},
	{ActBackup, "Setup", "Back up now and see backups"},
	{ActManageUsers, "Setup", "Add and change staff"},
	{ActManageKeys, "Setup", "Create and revoke API keys"},
	{ActManagePermissions, "Setup", "Change permissions"},
}

// Actions lists every action, in screen order, with Kishtin's defaults.
func Actions() []ActionInfo {
	out := make([]ActionInfo, 0, len(actionInfo))
	for _, ai := range actionInfo {
		r := policy[ai.a]
		why, f := fixed[ai.a]
		out = append(out, ActionInfo{Action: ai.a, Group: ai.group, Label: ai.label, Roles: slices.Clone(r.roles), PIN: !r.password, Fixed: f, Why: why})
	}
	return out
}

// Label is an action's name on the screens.
func Label(a Action) string {
	for _, ai := range actionInfo {
		if ai.a == a {
			return ai.label
		}
	}
	return string(a)
}

// Normalize sorts grants into screen order, drops managers from roles
// (they always may) and removes duplicates.
func (pm Permissions) Normalize() Permissions {
	order := map[Action]int{}
	for i, ai := range actionInfo {
		order[ai.a] = i
	}
	out := Permissions{Grants: []Grant{}}
	for _, g := range pm.Grants {
		roles := []Role{}
		for _, r := range Roles {
			if r != RoleManager && slices.Contains(g.Roles, r) {
				roles = append(roles, r)
			}
		}
		users := []string{}
		for _, u := range g.Users {
			if u = strings.TrimSpace(u); u != "" && !slices.Contains(users, u) {
				users = append(users, u)
			}
		}
		slices.Sort(users)
		out.Grants = append(out.Grants, Grant{Action: g.Action, Roles: roles, Users: users, PIN: g.PIN})
	}
	slices.SortStableFunc(out.Grants, func(a, b Grant) int { return order[a.Action] - order[b.Action] })
	return out
}

// Validate checks the grants: known actions, at most one grant each, none
// for a fixed action, known roles.
func (pm Permissions) Validate() error {
	var errs []error
	seen := map[Action]bool{}
	for _, g := range pm.Grants {
		if _, ok := policy[g.Action]; !ok {
			errs = append(errs, fmt.Errorf("unknown action %q", g.Action))
			continue
		}
		if why, ok := fixed[g.Action]; ok {
			errs = append(errs, fmt.Errorf("%q is always managers with a password: %s", Label(g.Action), why))
		}
		if seen[g.Action] {
			errs = append(errs, fmt.Errorf("%q is set twice", Label(g.Action)))
		}
		seen[g.Action] = true
		for _, r := range g.Roles {
			if !r.Valid() {
				errs = append(errs, fmt.Errorf("%q: unknown role %q", Label(g.Action), r))
			}
		}
	}
	return errors.Join(errs...)
}

// Same reports whether a grant gives exactly Kishtin's default, so it can
// be left out.
func (g Grant) Same() bool {
	r := policy[g.Action]
	def := []Role{}
	for _, role := range Roles {
		if role != RoleManager && slices.Contains(r.roles, role) {
			def = append(def, role)
		}
	}
	return len(g.Users) == 0 && g.PIN == !r.password && slices.Equal(g.Roles, def)
}
