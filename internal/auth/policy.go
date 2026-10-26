package auth

import (
	"slices"
	"sync"
	"time"
)

// Action is something a principal can be allowed to do.
type Action string

const (
	ActLogHACCP     Action = "haccp.log"
	ActViewHACCP    Action = "haccp.view"
	ActExportHACCP  Action = "haccp.export"
	ActManageLimits Action = "haccp.limits"
	ActManageUsers  Action = "users.manage"
	ActManageKeys   Action = "keys.manage"
	ActBackup       Action = "backup"
	// ActManageSettings changes system settings such as the backup schedule.
	ActManageSettings Action = "settings.manage"
	// ActSignOff signs off the HACCP records for a period: the manager or
	// supervisor check of a paper record book.
	ActSignOff Action = "haccp.sign_off"
	// ActInspect logs hygiene inspections and staff training: the manager's
	// or supervisor's records.
	ActInspect Action = "haccp.inspect"
	// ActFitness logs and sees fitness to work assessments, which are
	// health information.
	ActFitness Action = "haccp.fitness"
	// ActBackdate logs a record for a check done earlier, giving the time
	// it was done.
	ActBackdate Action = "haccp.backdate"
	// ActLogSignedOff adds or corrects a record in a period a manager has
	// already signed off.
	ActLogSignedOff Action = "haccp.signed_off"
	// ActManagePermissions changes who may do what. Only managers with a
	// password can, so nobody can give themselves more access.
	ActManagePermissions Action = "permissions.manage"

	// ActViewKitchen sees the menu, orders, the pass and station queues.
	ActViewKitchen Action = "kitchen.view"
	// ActManageMenu changes stations, equipment and dishes.
	ActManageMenu Action = "menu.manage"
	// ActSetAvailability marks a dish sold out (86) or back on.
	ActSetAvailability Action = "menu.availability"
	// ActTakeOrders enters orders and adds items to them.
	ActTakeOrders Action = "orders.take"
	// ActRunPass fires and serves courses, voids items, changes target
	// times and closes orders: the expo's actions.
	ActRunPass Action = "orders.pass"
	// ActWorkStation marks items started, rested, plated and ready.
	ActWorkStation Action = "orders.station"
)

type rule struct {
	roles []Role
	// password means the principal must have logged in with a full
	// password (or a user API key), not a PIN.
	password bool
}

var everyone = []Role{RoleHeadChef, RoleLineCook, RoleFrontOfHouse, RoleManager}

var policy = map[Action]rule{
	ActLogHACCP:       {roles: everyone},
	ActViewHACCP:      {roles: everyone},
	ActExportHACCP:    {roles: []Role{RoleHeadChef, RoleManager}},
	ActManageLimits:   {roles: []Role{RoleManager}, password: true},
	ActManageUsers:    {roles: []Role{RoleManager}, password: true},
	ActManageKeys:     {roles: []Role{RoleManager}, password: true},
	ActBackup:         {roles: []Role{RoleManager}, password: true},
	ActManageSettings: {roles: []Role{RoleManager}, password: true},
	ActSignOff:        {roles: []Role{RoleManager}, password: true},
	ActInspect:        {roles: []Role{RoleHeadChef, RoleManager}},
	ActFitness:        {roles: []Role{RoleManager}, password: true},
	ActBackdate:       {roles: everyone},
	ActLogSignedOff:   {roles: []Role{RoleManager}, password: true},

	ActManagePermissions: {roles: []Role{RoleManager}, password: true},

	ActViewKitchen:     {roles: everyone},
	ActManageMenu:      {roles: []Role{RoleManager}, password: true},
	ActSetAvailability: {roles: []Role{RoleHeadChef, RoleLineCook, RoleManager}},
	ActTakeOrders:      {roles: []Role{RoleFrontOfHouse, RoleHeadChef, RoleManager}},
	ActRunPass:         {roles: []Role{RoleHeadChef, RoleManager}},
	ActWorkStation:     {roles: []Role{RoleLineCook, RoleHeadChef, RoleManager}},
}

// Principal is who is making a request.
type Principal struct {
	User   User
	Method Method
	// Device and DeviceName are the API key the request came through: a
	// kitchen device, or a user key for the CLI and scripts.
	Device, DeviceName string
}

// Decision explains whether an action is allowed.
type Decision struct {
	Allowed bool
	// NeedPassword is set when the role is right but the login was a PIN.
	NeedPassword bool
	Reason       string
}

// Can decides whether p may take action a under Kishtin's default
// permissions.
func Can(p Principal, a Action) Decision { return Permissions{}.Can(p, a) }

// Can decides whether p may take action a under these permissions: the
// restaurant's grant for a if it has one, otherwise Kishtin's default.
// Managers may always take every action.
func (pm Permissions) Can(p Principal, a Action) Decision {
	r, ok := policy[a]
	if !ok {
		return Decision{Reason: "unknown action"}
	}
	if !p.User.Active {
		return Decision{Reason: "this user is deactivated"}
	}
	roles, users, password := r.roles, []string(nil), r.password
	if g, ok := pm.grant(a); ok {
		roles, users, password = g.Roles, g.Users, !g.PIN
	}
	if p.User.Role != RoleManager && !slices.Contains(roles, p.User.Role) && !slices.Contains(users, p.User.ID) {
		return Decision{Reason: "you are not allowed to do this: a manager can change this in Setup › Staff and access › Permissions"}
	}
	if password && p.Method == MethodPIN {
		return Decision{NeedPassword: true, Reason: "log in with your password to do this"}
	}
	return Decision{Allowed: true}
}

func (pm Permissions) grant(a Action) (Grant, bool) {
	if Fixed(a) {
		return Grant{}, false
	}
	for _, g := range pm.Grants {
		if g.Action == a {
			return g, true
		}
	}
	return Grant{}, false
}

// Lockout limits failed logins per user: after MaxFails failures in a row
// the user is locked for LockFor.
type Lockout struct {
	MaxFails int
	LockFor  time.Duration

	mu    sync.Mutex
	state map[string]lockState
}

type lockState struct {
	fails int
	until time.Time
}

// NewLockout returns a lockout with kitchen-friendly defaults: 5 tries, then
// 5 minutes.
func NewLockout() *Lockout {
	return &Lockout{MaxFails: 5, LockFor: 5 * time.Minute, state: map[string]lockState{}}
}

// Locked reports whether a user is locked at now, and until when.
func (l *Lockout) Locked(userID string, now time.Time) (bool, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.state[userID]
	return now.Before(s.until), s.until
}

// Fail records a failed login.
func (l *Lockout) Fail(userID string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.state[userID]
	s.fails++
	if s.fails >= l.MaxFails {
		s.fails, s.until = 0, now.Add(l.LockFor)
	}
	l.state[userID] = s
}

// Succeed clears failures after a good login.
func (l *Lockout) Succeed(userID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.state, userID)
}
