package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
)

func TestConnectSquare(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	creds := &integrations.CredentialStore{Dir: filepath.Join(t.TempDir(), "connectors")}
	till := &fakeTill{}
	var gotEnv, gotToken string
	k.svc.creds = creds
	setNewTill(k.svc, TillSquare, func(env, token string) (integrations.POSAdapter, error) {
		gotEnv, gotToken = env, token
		return till, nil
	})

	list, err := k.svc.Connectors(ctx, k.cook)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "square", list[0].ID)
	assert.Equal(t, "sumup", list[1].ID)
	assert.Equal(t, "Square", list[0].Name)
	assert.False(t, list[0].Connected)
	assert.NotEmpty(t, list[0].Sends, "says what leaves the building")

	in := ConnectInput{Environment: "sandbox", Token: " synthetic-token "}
	_, err = k.svc.ConnectTill(ctx, k.chef, TillSquare, in)
	assert.Equal(t, KindForbidden, KindOf(err), "a manager with the password connects")

	// A token Square refuses is not saved.
	till.err = errors.New("Square refused the access token")
	_, err = k.svc.ConnectTill(ctx, k.manager, TillSquare, in)
	assert.ErrorContains(t, err, "Square did not accept the connection: Square refused the access token. Nothing was saved.")
	_, saved, err := creds.Load("square")
	require.NoError(t, err)
	assert.False(t, saved)

	till.err = nil
	res, err := k.svc.ConnectTill(ctx, k.manager, TillSquare, in)
	require.NoError(t, err)
	assert.Equal(t, "sandbox", gotEnv)
	assert.Equal(t, "synthetic-token", gotToken)
	assert.Equal(t, []integrations.Location{{ID: "loc-synthetic", Name: "The Synthetic Kitchen", Active: true}}, res.Locations)
	assert.True(t, res.Connector.Connected)
	assert.Equal(t, SourceSaved, res.Connector.Source)
	assert.Equal(t, "sandbox", res.Connector.Environment)
	cr, saved, err := creds.Load("square")
	require.NoError(t, err)
	assert.True(t, saved)
	assert.Equal(t, "synthetic-token", cr.Token)

	locs, err := k.svc.TillLocations(ctx, k.manager, TillSquare)
	require.NoError(t, err)
	assert.Len(t, locs, 1)

	// A saved connection comes back after a restart.
	k.svc.setAdapter(TillSquare, nil, "")
	require.NoError(t, k.svc.loadSavedTills())
	assert.NotNil(t, k.svc.adapter(TillSquare))

	// Disconnect refuses while import is on.
	on := integrations.POSSettings{Enabled: true, LocationID: "loc-synthetic", PollSeconds: 5}
	_, err = k.svc.Square().Save(ctx, k.manager, SettingsChange[integrations.POSSettings]{Value: on, Reason: "Go live"})
	require.NoError(t, err)
	err = k.svc.DisconnectTill(ctx, k.manager, TillSquare)
	assert.ErrorContains(t, err, "Turn it off first")
	on.Enabled = false
	_, err = k.svc.Square().Save(ctx, k.manager, SettingsChange[integrations.POSSettings]{Value: on, Reason: "Pause"})
	require.NoError(t, err)
	require.NoError(t, k.svc.DisconnectTill(ctx, k.manager, TillSquare))
	assert.Nil(t, k.svc.adapter(TillSquare))
	_, saved, err = creds.Load("square")
	require.NoError(t, err)
	assert.False(t, saved)
}

func TestSquareTokenFromEnvironmentIsFixed(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	k.svc.setAdapter(TillSquare, &fakeTill{}, SourceEnvironment)
	k.svc.creds = &integrations.CredentialStore{Dir: t.TempDir()}
	setNewTill(k.svc, TillSquare, func(string, string) (integrations.POSAdapter, error) { return &fakeTill{}, nil })

	_, err := k.svc.ConnectTill(ctx, k.manager, TillSquare, ConnectInput{Environment: "production", Token: "x"})
	assert.ErrorContains(t, err, "set in the server's environment (KISHTIN_SQUARE_TOKEN)")
	assert.ErrorContains(t, k.svc.DisconnectTill(ctx, k.manager, TillSquare), "Change it there")
	list, err := k.svc.Connectors(ctx, k.cook)
	require.NoError(t, err)
	assert.True(t, list[0].Connected)
	assert.Equal(t, SourceEnvironment, list[0].Source)
}

// setNewTill sets how a till makes an adapter for a saved token.
func setNewTill(s *Service, id string, f func(environment, token string) (integrations.POSAdapter, error)) {
	s.posMu.Lock()
	defer s.posMu.Unlock()
	c := s.pos[id]
	c.New = f
	s.pos[id] = c
}
