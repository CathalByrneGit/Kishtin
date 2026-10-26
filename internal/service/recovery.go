package service

import (
	"context"
	"strings"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// Recovery is a new password for a manager who cannot log in. It is only
// taken from the Kishtin computer itself (kishtind reset-password), by
// someone who can use the computer account that holds the data folder:
// they could copy or replace every record anyway.
type Recovery struct {
	// Name is the manager. Empty means the only active manager.
	Name     string `json:"name,omitempty"`
	Password string `json:"password"`
}

// RecoveryResult says whose password was set, and whether they were made
// a manager to do it.
type RecoveryResult struct {
	User auth.User `json:"user"`
	// Created is true when there was no manager left and a new one was
	// made with this name.
	Created bool `json:"created"`
	// Promoted is true when there was no manager left and an existing staff
	// member was made one.
	Promoted bool `json:"promoted"`
}

// RecoveryDevice names where a recovery was done, in the activity log.
const RecoveryDevice = "The Kishtin computer"

// RecoverManager sets a new password for a manager. When no active manager
// is left (the last one left or was deactivated), it makes the named staff
// member a manager, or creates a new manager with that name. It also clears
// any lockout from wrong tries. Every recovery is in the activity log.
func (s *Service) RecoverManager(ctx context.Context, in Recovery) (RecoveryResult, error) {
	if err := auth.ValidatePassword(in.Password); err != nil {
		return RecoveryResult{}, newErr(KindInvalid, "%s", err)
	}
	name := strings.TrimSpace(in.Name)
	var res RecoveryResult
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		users, err := tx.Users().ListUsers(ctx)
		if err != nil {
			return err
		}
		var managers []auth.User
		var match *auth.User
		for i, u := range users {
			if u.Active && u.Role == auth.RoleManager {
				managers = append(managers, u)
			}
			if name != "" && strings.EqualFold(u.Name, name) {
				match = &users[i]
			}
		}
		now := s.clk.Now()
		switch {
		case name == "" && len(managers) == 1:
			res.User = managers[0]
		case name == "" && len(managers) == 0:
			if len(users) == 0 {
				return newErr(KindConflict, "Kishtin is not set up yet: open it in a browser on this computer to set it up")
			}
			return newErr(KindInvalid, "no manager is active: give a name with --name to make that person a manager, or a new name to add a manager")
		case name == "":
			return newErr(KindInvalid, "say whose password to reset with --name: the managers are %s", names(managers))
		case match != nil && match.Active && match.Role == auth.RoleManager:
			res.User = *match
		case len(managers) > 0:
			if match != nil {
				return newErr(KindInvalid, "%s is not an active manager: the managers are %s", match.Name, names(managers))
			}
			return newErr(KindInvalid, "no manager is called %q: the managers are %s", name, names(managers))
		case match != nil:
			match.Role, match.Active = auth.RoleManager, true
			res.User, res.Promoted = *match, true
		default:
			u, err := s.buildUser(NewUser{Name: name, Role: auth.RoleManager, Password: in.Password})
			if err != nil {
				return err
			}
			if err := tx.Users().CreateUser(ctx, u); err != nil {
				return mapStoreErr(err, "a user with that name")
			}
			res.User, res.Created = u, true
		}
		if !res.Created {
			if err := setSecrets(&res.User, "", in.Password); err != nil {
				return err
			}
			res.User.UpdatedAt = now
			if err := tx.Users().UpdateUser(ctx, res.User); err != nil {
				return mapStoreErr(err, "user")
			}
		}
		p := &auth.Principal{User: res.User, DeviceName: RecoveryDevice}
		return s.note(ctx, tx, p, ActivityPasswordRecovered, res.User.ID, map[string]any{"created": res.Created, "promoted": res.Promoted})
	})
	if err != nil {
		return RecoveryResult{}, err
	}
	s.lockout.Succeed(res.User.ID)
	s.publish(events.KindStaff, res.User.ID)
	return res, nil
}

func names(users []auth.User) string {
	out := make([]string, len(users))
	for i, u := range users {
		out[i] = u.Name
	}
	return strings.Join(out, ", ")
}
