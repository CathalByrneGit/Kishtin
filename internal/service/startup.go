package service

import (
	"context"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/autostart"
)

// Startup turns Kishtin starting by itself with the computer it runs on on
// and off. kishtind provides it.
type Startup interface {
	Status(ctx context.Context) (autostart.Status, error)
	// Enable never starts a second Kishtin: this one is already running.
	Enable(ctx context.Context) (autostart.Status, error)
	Disable(ctx context.Context) (autostart.Status, error)
}

// StartupStatus says whether Kishtin starts by itself with this computer.
func (s *Service) StartupStatus(ctx context.Context, p *auth.Principal) (autostart.Status, error) {
	if err := s.allow(p, auth.ActManageSettings); err != nil {
		return autostart.Status{}, err
	}
	if s.startup == nil {
		return autostart.Status{Note: "This server cannot change how it starts. Run kishtind startup on, on the Kishtin computer."}, nil
	}
	return s.startup.Status(ctx)
}

// SetStartup turns Kishtin starting by itself with this computer on or
// off. It changes the computer, not the database, and is noted in the
// activity log.
func (s *Service) SetStartup(ctx context.Context, p *auth.Principal, on bool) (autostart.Status, error) {
	if err := s.allow(p, auth.ActManageSettings); err != nil {
		return autostart.Status{}, err
	}
	if s.startup == nil {
		return autostart.Status{}, newErr(KindConflict, "this server cannot change how it starts: run kishtind startup on, on the Kishtin computer")
	}
	var st autostart.Status
	var err error
	if on {
		st, err = s.startup.Enable(ctx)
	} else {
		st, err = s.startup.Disable(ctx)
	}
	if err != nil {
		return autostart.Status{}, newErr(KindInvalid, "%s", err)
	}
	if err := s.note(ctx, s.st, p, ActivityStartupChanged, "", map[string]any{"on": on, "method": st.Method}); err != nil {
		return autostart.Status{}, err
	}
	return st, nil
}
