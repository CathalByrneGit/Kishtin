package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// Where a connector's token came from.
const (
	SourceEnvironment = "environment"
	SourceConfig      = "kishtin.toml"
	SourceSaved       = "saved"
)

// square returns the Square adapter, or nil when Square is not connected.
func (s *Service) square() integrations.POSAdapter {
	s.posMu.RLock()
	defer s.posMu.RUnlock()
	return s.pos
}

// loadSavedSquare connects Square with a token saved on the screens, when
// none is set in the environment or kishtin.toml.
func (s *Service) loadSavedSquare() error {
	if s.square() != nil || s.creds == nil || s.newSquare == nil {
		return nil
	}
	cr, ok, err := s.creds.Load(posSquare)
	if err != nil || !ok {
		return err
	}
	pos, err := s.newSquare(cr.Environment, cr.Token)
	if err != nil {
		return fmt.Errorf("saved Square connection: %w", err)
	}
	s.posMu.Lock()
	s.pos, s.posSource = pos, SourceSaved
	s.posMu.Unlock()
	return nil
}

// ConnectorStatus is a connector and how it stands.
type ConnectorStatus struct {
	integrations.Connector
	// Connected is set when Kishtin has a working token for it.
	Connected bool `json:"connected"`
	// Source is where the token came from: environment, kishtin.toml or
	// saved (on the screens). Only a saved token can be changed there.
	Source      string `json:"source,omitempty"`
	Environment string `json:"environment,omitempty"`
	// Enabled is set when it is in use, such as order import turned on.
	Enabled   bool       `json:"enabled"`
	LastOKAt  *time.Time `json:"last_ok_at,omitempty"`
	LastError string     `json:"last_error,omitempty"`
}

// Connectors lists what Kishtin can connect to, and how each stands.
func (s *Service) Connectors(ctx context.Context, p *auth.Principal) ([]ConnectorStatus, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return nil, err
	}
	out := make([]ConnectorStatus, 0, len(integrations.Connectors))
	for _, c := range integrations.Connectors {
		cs := ConnectorStatus{Connector: c}
		if c.ID == posSquare {
			s.posMu.RLock()
			cs.Connected, cs.Source = s.pos != nil, s.posSource
			s.posMu.RUnlock()
			if cs.Source == SourceSaved && s.creds != nil {
				if cr, ok, err := s.creds.Load(posSquare); err == nil && ok {
					cs.Environment = cr.Environment
				}
			}
			set, err := s.Square().current(ctx, s.st)
			if err != nil {
				return nil, err
			}
			cs.Enabled = set.Value.Enabled
			if st, err := s.st.POS().GetPOSState(ctx, posSquare); err == nil {
				cs.LastOKAt, cs.LastError = st.LastOKAt, st.LastError
			} else if !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
		}
		out = append(out, cs)
	}
	return out, nil
}

// ConnectInput is a token to connect with.
type ConnectInput struct {
	// Environment is production or sandbox.
	Environment string `json:"environment" enum:"production,sandbox"`
	Token       string `json:"token" minLength:"1" maxLength:"1000"`
}

// ConnectResult is a connection that worked, with the account's locations
// to choose from.
type ConnectResult struct {
	Connector ConnectorStatus         `json:"connector"`
	Locations []integrations.Location `json:"locations"`
}

// ConnectSquare checks a Square access token by asking Square for the
// account's locations, and saves it if Square accepts it. The token is
// saved in a file only the server can read, outside the database, so it is
// never in a backup or export. A token set in the environment or
// kishtin.toml cannot be changed here.
func (s *Service) ConnectSquare(ctx context.Context, p *auth.Principal, in ConnectInput) (ConnectResult, error) {
	if err := allow(p, auth.ActManageSettings); err != nil {
		return ConnectResult{}, err
	}
	if err := s.lockedSquare(); err != nil {
		return ConnectResult{}, err
	}
	if s.creds == nil || s.newSquare == nil {
		return ConnectResult{}, newErr(KindConflict, "This server cannot save connections. Set KISHTIN_SQUARE_TOKEN instead.")
	}
	token := strings.TrimSpace(in.Token)
	if token == "" {
		return ConnectResult{}, newErr(KindInvalid, "paste the access token from the Square Developer Dashboard")
	}
	if in.Environment != "production" && in.Environment != "sandbox" {
		return ConnectResult{}, newErr(KindInvalid, "environment must be production or sandbox")
	}
	pos, err := s.newSquare(in.Environment, token)
	if err != nil {
		return ConnectResult{}, newErr(KindInvalid, "%s", err)
	}
	locs, err := pos.Locations(ctx)
	if err != nil {
		return ConnectResult{}, newErr(KindInvalid, "Square did not accept the connection: %s. Nothing was saved.", err)
	}
	if err := s.creds.Save(posSquare, integrations.Credentials{Environment: in.Environment, Token: token}); err != nil {
		return ConnectResult{}, err
	}
	s.posMu.Lock()
	s.pos, s.posSource = pos, SourceSaved
	s.posMu.Unlock()
	s.publish(events.KindSettings, "connectors")
	list, err := s.Connectors(ctx, p)
	if err != nil {
		return ConnectResult{}, err
	}
	return ConnectResult{Connector: list[0], Locations: locs}, nil
}

// DisconnectSquare removes a token saved on the screens. Order import must
// be turned off first, so it is never left on with nothing to import from.
func (s *Service) DisconnectSquare(ctx context.Context, p *auth.Principal) error {
	if err := allow(p, auth.ActManageSettings); err != nil {
		return err
	}
	if err := s.lockedSquare(); err != nil {
		return err
	}
	set, err := s.Square().current(ctx, s.st)
	if err != nil {
		return err
	}
	if set.Value.Enabled {
		return newErr(KindConflict, "Order import from Square is on. Turn it off first, then disconnect.")
	}
	if s.creds != nil {
		if err := s.creds.Delete(posSquare); err != nil {
			return err
		}
	}
	s.posMu.Lock()
	s.pos, s.posSource = nil, ""
	s.posMu.Unlock()
	s.publish(events.KindSettings, "connectors")
	return nil
}

// lockedSquare refuses changes to a token set outside Kishtin.
func (s *Service) lockedSquare() error {
	s.posMu.RLock()
	src := s.posSource
	s.posMu.RUnlock()
	switch src {
	case SourceEnvironment:
		return newErr(KindConflict, "The Square token is set in the server's environment (KISHTIN_SQUARE_TOKEN). Change it there and restart kishtind.")
	case SourceConfig:
		return newErr(KindConflict, "The Square token is set in kishtin.toml. Change it there and restart kishtind.")
	}
	return nil
}

// SquareLocations lists the Square account's locations. It sends Square
// the access token only.
func (s *Service) SquareLocations(ctx context.Context, p *auth.Principal) ([]integrations.Location, error) {
	if err := allow(p, auth.ActManageSettings); err != nil {
		return nil, err
	}
	pos := s.square()
	if pos == nil {
		return nil, newErr(KindConflict, "Square is not connected. A manager connects it on Setup, Connections, Square.")
	}
	locs, err := pos.Locations(ctx)
	if err != nil {
		return nil, newErr(KindConflict, "Could not ask Square for its locations: %s", err)
	}
	return locs, nil
}
