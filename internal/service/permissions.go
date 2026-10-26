package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// builtinPermissionsID is the version ID of Kishtin's default permissions.
const builtinPermissionsID = "builtin-permissions-v1"

// Permissions is who may do what, where it differs from Kishtin's defaults.
// A manager with a password changes it in Setup › Permissions.
func (s *Service) Permissions() *Group[auth.Permissions] {
	return &Group[auth.Permissions]{s: s, spec: groupSpec[auth.Permissions]{
		kind: settings.KindPermissions, builtinID: builtinPermissionsID, seeded: true,
		builtin: func(*Service) auth.Permissions { return auth.Permissions{Grants: []auth.Grant{}} },
		normalize: func(pm auth.Permissions) auth.Permissions {
			pm = pm.Normalize()
			pm.Grants = slices.DeleteFunc(pm.Grants, auth.Grant.Same)
			return pm
		},
		validate:   auth.Permissions.Validate,
		lessStrict: func(_, _ auth.Permissions) []string { return nil },
		notes:      permissionNotes,
		changes:    permissionChanges,
		edit:       auth.ActManagePermissions,
		saved:      func(ctx context.Context, s *Service) error { return s.loadPermissions(ctx) },
	}}
}

// permissions returns the permissions in force.
func (s *Service) permissions() auth.Permissions {
	if p := s.perms.Load(); p != nil {
		return *p
	}
	return auth.Permissions{}
}

// loadPermissions reads the permissions in force into the cache.
func (s *Service) loadPermissions(ctx context.Context) error {
	cur, err := s.Permissions().current(ctx, s.st)
	if err != nil {
		return fmt.Errorf("load permissions: %w", err)
	}
	s.perms.Store(&cur.Value)
	return nil
}

// Allowed is whether a principal may take an action: "yes", "password"
// (after logging in with a password) or "no".
type Allowed string

const (
	AllowedYes      Allowed = "yes"
	AllowedPassword Allowed = "password"
	AllowedNo       Allowed = "no"
)

// Can lists what a principal may do, for the screens to show only what
// works. The server checks every action again.
func (s *Service) Can(p *auth.Principal) map[auth.Action]Allowed {
	out := map[auth.Action]Allowed{}
	pm := s.permissions()
	for _, ai := range auth.Actions() {
		out[ai.Action] = AllowedNo
		if p == nil {
			continue
		}
		d := pm.Can(*p, ai.Action)
		switch {
		case d.Allowed:
			out[ai.Action] = AllowedYes
		case d.NeedPassword:
			out[ai.Action] = AllowedPassword
		}
	}
	return out
}

// PermissionActions lists every action with Kishtin's defaults, for the
// permissions screen.
func (s *Service) PermissionActions(p *auth.Principal) ([]auth.ActionInfo, error) {
	if err := s.allow(p, auth.ActViewHACCP); err != nil {
		return nil, err
	}
	return auth.Actions(), nil
}

var roleWords = map[auth.Role]string{
	auth.RoleHeadChef: "head chefs", auth.RoleLineCook: "line cooks", auth.RoleFrontOfHouse: "front of house", auth.RoleManager: "managers",
}

// who describes the people a grant allows, with named staff.
func who(roles []auth.Role, users []string, names map[string]string) string {
	parts := []string{"managers"}
	for _, r := range roles {
		if r != auth.RoleManager {
			parts = append(parts, roleWords[r])
		}
	}
	for _, u := range users {
		parts = append(parts, names[u])
	}
	return strings.Join(parts, ", ")
}

func grantFor(pm auth.Permissions, a auth.ActionInfo) auth.Grant {
	for _, g := range pm.Grants {
		if g.Action == a.Action {
			return g
		}
	}
	return auth.Grant{Action: a.Action, Roles: a.Roles, PIN: a.PIN}
}

func login(pin bool) string {
	if pin {
		return "PIN is enough"
	}
	return "needs a password"
}

// permissionChanges describes a change action by action. Named staff are
// counted here and named in the notes.
func permissionChanges(cur, next auth.Permissions) []string {
	var out []string
	for _, a := range auth.Actions() {
		if a.Fixed {
			continue
		}
		c, n := grantFor(cur, a), grantFor(next, a)
		if slices.Equal(c.Roles, n.Roles) && slices.Equal(c.Users, n.Users) && c.PIN == n.PIN {
			continue
		}
		line := fmt.Sprintf("%s: %s, %s", a.Label, who(n.Roles, nil, nil), login(n.PIN))
		if len(n.Users) > 0 {
			line += fmt.Sprintf(", and %d named %s", len(n.Users), plural(len(n.Users), "person", "people"))
		}
		out = append(out, line)
	}
	return out
}

// permissionNotes names who each changed action would allow, and refuses
// staff who do not exist.
func permissionNotes(ctx context.Context, _ *Service, st store.Store, cur, next auth.Permissions) ([]string, error) {
	users, err := st.Users().ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	names, noPassword := map[string]string{}, map[string]bool{}
	for _, u := range users {
		names[u.ID] = u.Name
		if !u.Active {
			names[u.ID] += " (deactivated)"
		}
		noPassword[u.ID] = u.PasswordHash == ""
	}
	out := []string{}
	for _, a := range auth.Actions() {
		if a.Fixed {
			continue
		}
		c, n := grantFor(cur, a), grantFor(next, a)
		if slices.Equal(c.Roles, n.Roles) && slices.Equal(c.Users, n.Users) && c.PIN == n.PIN {
			continue
		}
		for _, u := range n.Users {
			if _, ok := names[u]; !ok {
				return nil, newErr(KindInvalid, "%s: no member of staff has ID %s", a.Label, u)
			}
		}
		line := fmt.Sprintf("%s: %s (%s).", a.Label, who(n.Roles, n.Users, names), login(n.PIN))
		out = append(out, line)
		if !n.PIN {
			for _, u := range n.Users {
				if noPassword[u] {
					out = append(out, fmt.Sprintf("%s has no password, so cannot %s until a manager sets one in Setup › Staff, or PIN is made enough.", names[u], strings.ToLower(a.Label[:1])+a.Label[1:]))
				}
			}
		}
	}
	return out, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
