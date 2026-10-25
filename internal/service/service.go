// Package service holds Kishtin's actions: logging HACCP records, managing
// limits, staff, keys and backups. The HTTP API and the CLI both call these
// functions, so people and agents work under the same rules.
package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/cathalbyrnegit/kishtin/internal/ai"
	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/speech"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
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
	// speech is the local speech sidecar client; nil turns voice audio off.
	speech *speech.Client
	voice  voiceState
	// wakeWords start a hands-free voice command.
	wakeWords []string
	// pos is the Square adapter; nil when Square is not connected. posMu
	// guards it, because a manager can connect Square while the server
	// runs.
	posMu sync.RWMutex
	pos   integrations.POSAdapter
	// posSource is where the Square token came from: "environment" or
	// "kishtin.toml" (fixed while the server runs), or "saved" (connected
	// on the screens).
	posSource string
	creds     *integrations.CredentialStore
	newSquare func(environment, token string) (integrations.POSAdapter, error)
	ai        *ai.Client
	fetch     func(ctx context.Context, url string) ([]byte, error)
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
	// Speech is the local speech-to-text and text-to-speech client. Nil
	// leaves typed voice commands working and spoken ones off.
	Speech *speech.Client
	// WakeWords start a hands-free voice command. Empty uses "kishtin".
	WakeWords []string
	// POS reads orders from Square, when the token is set in the
	// environment or kishtin.toml; POSSource says which. It cannot be
	// changed on the screens.
	POS       integrations.POSAdapter
	POSSource string
	// Credentials keeps tokens saved on the screens, outside the database.
	Credentials *integrations.CredentialStore
	// NewSquare makes a Square client for a saved token.
	NewSquare func(environment, token string) (integrations.POSAdapter, error)
	// AI is an optional local model that reads copies of record books and
	// photos of paper pages. Nil leaves those suggestions off.
	AI *ai.Client
	// Fetch downloads a public URL, for checking record book editions.
	// Nil uses a plain HTTP client.
	Fetch func(ctx context.Context, url string) ([]byte, error)
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
	wake := o.WakeWords
	if len(wake) == 0 {
		wake = voice.DefaultWakeWords
	}
	return &Service{wakeWords: wake, st: o.Store, clk: clk, files: o.Files, lockout: auth.NewLockout(), backups: o.Backups, systemDefaults: sys, events: o.Events, speech: o.Speech, pos: o.POS,
		posSource: o.POSSource, creds: o.Credentials, newSquare: o.NewSquare, ai: o.AI, fetch: o.Fetch}
}

// Events returns the hub that change notices are published to.
func (s *Service) Events() *events.Hub { return s.events }

func (s *Service) publish(kind, id string) { s.events.Publish(events.Event{Kind: kind, ID: id}) }

// Init stores the built-in FSAI default limit versions and the built-in
// guidance, rules and lists if they are missing.
func (s *Service) Init(ctx context.Context) error {
	if err := s.loadSavedSquare(); err != nil {
		return err
	}
	return s.st.WithTx(ctx, func(tx store.Store) error {
		for _, seed := range []func(context.Context, store.Store) error{s.Guidance().seed, s.Rules().seed, s.Lists().seed, s.Scheduler().seed, s.Square().seed, s.RecordBookSettings().seed} {
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
