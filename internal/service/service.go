// Package service holds Kishtin's actions: logging HACCP records, managing
// limits, staff, keys and backups. The HTTP API and the CLI both call these
// functions, so people and agents work under the same rules.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// Kind classifies a service error, so callers can map it to an HTTP status
// or an exit code.
type Kind int

const (
	KindInvalid Kind = iota + 1
	KindUnauthenticated
	KindForbidden
	KindNotFound
	KindConflict
	KindLocked
)

// Error is a service error with a message written for staff.
type Error struct {
	Kind Kind
	Msg  string
	// NeedPassword is set on a forbidden error when a manager password
	// login would allow the action.
	NeedPassword bool
}

func (e *Error) Error() string { return e.Msg }

func newErr(k Kind, format string, args ...any) error {
	return &Error{Kind: k, Msg: fmt.Sprintf(format, args...)}
}

// KindOf returns the kind of a service error, or 0 for other errors.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return 0
}

// Service runs Kishtin's actions.
type Service struct {
	st      store.Store
	clk     clock.Clock
	files   *files.Store
	lockout *auth.Lockout
	backups BackupConfig
	// systemDefaults are the system settings from kishtin.toml, used until
	// a change is saved.
	systemDefaults settings.System
	events         *events.Hub
}

// Options configures a Service.
type Options struct {
	Store   store.Store
	Clock   clock.Clock
	Files   *files.Store
	Backups BackupConfig
	// System is the starting system settings, from kishtin.toml.
	System settings.System
	// Events receives a notice of every change, for live screens.
	Events *events.Hub
}

// DefaultSystem returns the system settings used when none are configured.
func DefaultSystem() settings.System {
	return settings.System{BackupIntervalMinutes: 6 * 60, BackupKeep: 28, MaxUploadMB: 15, PreviewRecords: 50}
}

// New returns a service. Call Init before use.
func New(o Options) *Service {
	clk := o.Clock
	if clk == nil {
		clk = clock.Real{}
	}
	sys := o.System
	if sys == (settings.System{}) {
		sys = DefaultSystem()
	}
	return &Service{st: o.Store, clk: clk, files: o.Files, lockout: auth.NewLockout(), backups: o.Backups, systemDefaults: sys, events: o.Events}
}

// Events returns the hub that change notices are published to.
func (s *Service) Events() *events.Hub { return s.events }

func (s *Service) publish(kind, id string) { s.events.Publish(events.Event{Kind: kind, ID: id}) }

// Init stores the built-in FSAI default limit versions and the built-in
// guidance, rules and lists if they are missing.
func (s *Service) Init(ctx context.Context) error {
	return s.st.WithTx(ctx, func(tx store.Store) error {
		for _, seed := range []func(context.Context, store.Store) error{s.Guidance().seed, s.Rules().seed, s.Lists().seed} {
			if err := seed(ctx, tx); err != nil {
				return err
			}
		}
		for _, lv := range haccp.FSAIDefaults() {
			_, err := tx.HACCP().GetLimitVersion(ctx, lv.ID)
			if err == nil {
				continue
			}
			if !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("check default limit %s: %w", lv.ID, err)
			}
			if err := tx.HACCP().AppendLimitVersion(ctx, lv); err != nil {
				return fmt.Errorf("store default limit %s: %w", lv.ID, err)
			}
		}
		return nil
	})
}

// allow checks that p may take action a.
func allow(p *auth.Principal, a auth.Action) error {
	if p == nil {
		return newErr(KindUnauthenticated, "log in first")
	}
	d := auth.Can(*p, a)
	if !d.Allowed {
		return &Error{Kind: KindForbidden, Msg: d.Reason, NeedPassword: d.NeedPassword}
	}
	return nil
}

func newID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// NewV7 only fails if the system random source fails.
		return uuid.NewString()
	}
	return id.String()
}

func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }

func mapStoreErr(err error, what string) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return newErr(KindNotFound, "%s not found", what)
	case errors.Is(err, store.ErrConflict):
		return newErr(KindConflict, "%s already exists", what)
	}
	return err
}
