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
}

// Decision explains whether an action is allowed.
type Decision struct {
	Allowed bool
	// NeedPassword is set when the role is right but the login was a PIN.
	NeedPassword bool
	Reason       string
}

// Can decides whether p may take action a.
func Can(p Principal, a Action) Decision {
	r, ok := policy[a]
	if !ok {
		return Decision{Reason: "unknown action"}
	}
	if !p.User.Active {
		return Decision{Reason: "this user is deactivated"}
	}
	if !slices.Contains(r.roles, p.User.Role) {
		return Decision{Reason: "your role cannot do this"}
	}
	if r.password && p.Method == MethodPIN {
		return Decision{NeedPassword: true, Reason: "log in with your manager password to do this"}
	}
	return Decision{Allowed: true}
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
