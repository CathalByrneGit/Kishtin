package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// Where a connector's token came from.
const (
	SourceEnvironment = "environment"
	SourceConfig      = "kishtin.toml"
	SourceSaved       = "saved"
)

// Tills Kishtin imports orders from, by connector ID.
const (
	TillSquare = "square"
	TillSumUp  = "sumup"
)

// till is a point-of-sale system Kishtin imports orders from.
type till struct {
	id, name string
	kind     settings.Kind
	builtin  string
	// envVar and tokenHelp say where its token comes from, for messages.
	envVar, tokenHelp string
	// sends and returns say what each poll sends the till and gets back,
	// for the preview a manager reads before turning import on.
	sends   func(set integrations.POSSettings) string
	returns string
	// startsAt says which orders are imported once it is turned on.
	startsAt string
}

// user is who imported orders are entered by, in order histories.
func (t till) user() auth.User {
	return auth.User{ID: "pos:" + t.id, Name: t.name, Role: auth.RoleFrontOfHouse, Active: true}
}

var tills = []till{
	{
		id: TillSquare, name: "Square", kind: settings.KindSquare, builtin: "builtin-square-v1",
		envVar: "KISHTIN_SQUARE_TOKEN", tokenHelp: "paste the access token from the Square Developer Dashboard",
		sends: func(set integrations.POSSettings) string {
			return fmt.Sprintf("Every %d seconds Kishtin will send Square the access token, the location ID (%s) and the time of the last order change it has seen, to ask for new and changed orders.", set.PollSeconds, set.LocationID)
		},
		returns:  "Square sends back those orders: tickets, items, modifiers, notes and pickup names. Nothing about the kitchen, staff or HACCP records is sent to Square.",
		startsAt: "Orders changed on the till from the moment this is saved are imported. Earlier orders are not.",
	},
	{
		id: TillSumUp, name: "SumUp", kind: settings.KindSumUp, builtin: "builtin-sumup-v1",
		envVar: "KISHTIN_SUMUP_TOKEN", tokenHelp: "paste an API key from SumUp's dashboard: Settings, For Developers, Toolkit, API Keys",
		sends: func(set integrations.POSSettings) string {
			return fmt.Sprintf("Every %d seconds Kishtin will send SumUp the API key, the merchant code (%s) and the time of the last change it has seen, to ask for new, refunded and cancelled sales; then it asks for each of those sales by its ID.", set.PollSeconds, set.LocationID)
		},
		returns:  "SumUp sends back each sale: its code, status, time and items (names and quantities). Its answer also has payment details, such as the card type and last four digits; Kishtin reads only the items and keeps none of the rest. Nothing about the kitchen, staff or HACCP records is sent to SumUp.",
		startsAt: "Sales paid or changed from the moment this is saved are imported. Earlier sales are not. A sale reaches Kishtin once it is paid, so this suits counter service and takeaways, where people pay first.",
	},
}

func tillByID(id string) (till, error) {
	i := slices.IndexFunc(tills, func(t till) bool { return t.id == id })
	if i < 0 {
		return till{}, newErr(KindNotFound, "no till %q: Kishtin imports from square and sumup", id)
	}
	return tills[i], nil
}

// TillConnection is a till Kishtin is connected to when it starts: an
// adapter made from a token in the environment or kishtin.toml, and how to
// make one from a token saved on the screens.
type TillConnection struct {
	Adapter integrations.POSAdapter
	// Source is where Adapter's token came from: environment or
	// kishtin.toml. A token set there cannot be changed on the screens.
	Source string
	// New makes an adapter for a token saved on the screens.
	New func(environment, token string) (integrations.POSAdapter, error)
}

// adapter returns a till's adapter, or nil when it is not connected.
func (s *Service) adapter(id string) integrations.POSAdapter {
	s.posMu.RLock()
	defer s.posMu.RUnlock()
	return s.pos[id].Adapter
}

func (s *Service) source(id string) string {
	s.posMu.RLock()
	defer s.posMu.RUnlock()
	return s.pos[id].Source
}

func (s *Service) setAdapter(id string, a integrations.POSAdapter, source string) {
	s.posMu.Lock()
	defer s.posMu.Unlock()
	c := s.pos[id]
	c.Adapter, c.Source = a, source
	s.pos[id] = c
}

// loadSavedTills connects each till with a token saved on the screens,
// when none is set in the environment or kishtin.toml.
func (s *Service) loadSavedTills() error {
	for _, t := range tills {
		s.posMu.RLock()
		c := s.pos[t.id]
		s.posMu.RUnlock()
		if c.Adapter != nil || s.creds == nil || c.New == nil {
			continue
		}
		cr, ok, err := s.creds.Load(t.id)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		a, err := c.New(cr.Environment, cr.Token)
		if err != nil {
			return fmt.Errorf("saved %s connection: %w", t.name, err)
		}
		s.setAdapter(t.id, a, SourceSaved)
	}
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
	if err := s.allow(p, auth.ActViewKitchen); err != nil {
		return nil, err
	}
	out := make([]ConnectorStatus, 0, len(integrations.Connectors))
	for _, c := range integrations.Connectors {
		cs, err := s.connectorStatus(ctx, c)
		if err != nil {
			return nil, err
		}
		out = append(out, cs)
	}
	return out, nil
}

func (s *Service) connectorStatus(ctx context.Context, c integrations.Connector) (ConnectorStatus, error) {
	cs := ConnectorStatus{Connector: c}
	t, err := tillByID(c.ID)
	if err != nil {
		return cs, nil
	}
	cs.Connected, cs.Source = s.adapter(t.id) != nil, s.source(t.id)
	if cs.Source == SourceSaved && s.creds != nil {
		if cr, ok, err := s.creds.Load(t.id); err == nil && ok {
			cs.Environment = cr.Environment
		}
	}
	set, err := s.tillGroup(t).current(ctx, s.st)
	if err != nil {
		return cs, err
	}
	cs.Enabled = set.Value.Enabled
	if st, err := s.st.POS().GetPOSState(ctx, t.id); err == nil {
		cs.LastOKAt, cs.LastError = st.LastOKAt, st.LastError
	} else if !errors.Is(err, store.ErrNotFound) {
		return cs, err
	}
	return cs, nil
}

// ConnectInput is a token to connect with.
type ConnectInput struct {
	// Environment is production or sandbox (Square only).
	Environment string `json:"environment,omitempty" enum:"production,sandbox"`
	Token       string `json:"token" minLength:"1" maxLength:"1000"`
}

// ConnectResult is a connection that worked, with the account's locations
// to choose from.
type ConnectResult struct {
	Connector ConnectorStatus         `json:"connector"`
	Locations []integrations.Location `json:"locations"`
}

// ConnectTill checks a till's token by asking the till for the account's
// locations, and saves it if the till accepts it. The token is saved in a
// file only the server can read, outside the database, so it is never in a
// backup or export. A token set in the environment or kishtin.toml cannot
// be changed here.
func (s *Service) ConnectTill(ctx context.Context, p *auth.Principal, id string, in ConnectInput) (ConnectResult, error) {
	if err := s.allow(p, auth.ActManageSettings); err != nil {
		return ConnectResult{}, err
	}
	t, err := tillByID(id)
	if err != nil {
		return ConnectResult{}, err
	}
	if err := s.lockedTill(t); err != nil {
		return ConnectResult{}, err
	}
	s.posMu.RLock()
	newAdapter := s.pos[t.id].New
	s.posMu.RUnlock()
	if s.creds == nil || newAdapter == nil {
		return ConnectResult{}, newErr(KindConflict, "This server cannot save connections. Set %s instead.", t.envVar)
	}
	token := strings.TrimSpace(in.Token)
	if token == "" {
		return ConnectResult{}, newErr(KindInvalid, "%s", t.tokenHelp)
	}
	c, _ := integrations.ConnectorByID(t.id)
	if in.Environment == "" {
		in.Environment = "production"
	}
	if !slices.Contains(c.Environments, in.Environment) {
		return ConnectResult{}, newErr(KindInvalid, "environment must be %s", strings.Join(c.Environments, " or "))
	}
	a, err := newAdapter(in.Environment, token)
	if err != nil {
		return ConnectResult{}, newErr(KindInvalid, "%s", err)
	}
	locs, err := a.Locations(ctx)
	if err != nil {
		return ConnectResult{}, newErr(KindInvalid, "%s did not accept the connection: %s. Nothing was saved.", t.name, err)
	}
	if err := s.creds.Save(t.id, integrations.Credentials{Environment: in.Environment, Token: token}); err != nil {
		return ConnectResult{}, err
	}
	s.setAdapter(t.id, a, SourceSaved)
	s.publish(events.KindSettings, "connectors")
	cs, err := s.connectorStatus(ctx, c)
	if err != nil {
		return ConnectResult{}, err
	}
	return ConnectResult{Connector: cs, Locations: locs}, nil
}

// DisconnectTill removes a token saved on the screens. Order import must be
// turned off first, so it is never left on with nothing to import from.
func (s *Service) DisconnectTill(ctx context.Context, p *auth.Principal, id string) error {
	if err := s.allow(p, auth.ActManageSettings); err != nil {
		return err
	}
	t, err := tillByID(id)
	if err != nil {
		return err
	}
	if err := s.lockedTill(t); err != nil {
		return err
	}
	set, err := s.tillGroup(t).current(ctx, s.st)
	if err != nil {
		return err
	}
	if set.Value.Enabled {
		return newErr(KindConflict, "Order import from %s is on. Turn it off first, then disconnect.", t.name)
	}
	if s.creds != nil {
		if err := s.creds.Delete(t.id); err != nil {
			return err
		}
	}
	s.setAdapter(t.id, nil, "")
	s.publish(events.KindSettings, "connectors")
	return nil
}

// lockedTill refuses changes to a token set outside Kishtin.
func (s *Service) lockedTill(t till) error {
	switch s.source(t.id) {
	case SourceEnvironment:
		return newErr(KindConflict, "The %s token is set in the server's environment (%s). Change it there and restart kishtind.", t.name, t.envVar)
	case SourceConfig:
		return newErr(KindConflict, "The %s token is set in kishtin.toml. Change it there and restart kishtind.", t.name)
	}
	return nil
}

// notConnected is the error for a till with no token.
func notConnected(t till) error {
	return newErr(KindConflict, "%s is not connected. A manager connects it on Setup, Connections, %s.", t.name, t.name)
}

// TillLocations lists the till account's locations (for SumUp, its
// merchant accounts). It sends the till its token only.
func (s *Service) TillLocations(ctx context.Context, p *auth.Principal, id string) ([]integrations.Location, error) {
	if err := s.allow(p, auth.ActManageSettings); err != nil {
		return nil, err
	}
	t, err := tillByID(id)
	if err != nil {
		return nil, err
	}
	a := s.adapter(t.id)
	if a == nil {
		return nil, notConnected(t)
	}
	locs, err := a.Locations(ctx)
	if err != nil {
		return nil, newErr(KindConflict, "Could not ask %s for its locations: %s", t.name, err)
	}
	return locs, nil
}
