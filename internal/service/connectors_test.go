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
	k.svc.newSquare = func(env, token string) (integrations.POSAdapter, error) {
		gotEnv, gotToken = env, token
		return till, nil
	}

	list, err := k.svc.Connectors(ctx, k.cook)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "square", list[0].ID)
	assert.Equal(t, "Square", list[0].Name)
	assert.False(t, list[0].Connected)
	assert.NotEmpty(t, list[0].Sends, "says what leaves the building")

	in := ConnectInput{Environment: "sandbox", Token: " synthetic-token "}
	_, err = k.svc.ConnectSquare(ctx, k.chef, in)
	assert.Equal(t, KindForbidden, KindOf(err), "a manager with the password connects")

	// A token Square refuses is not saved.
	till.err = errors.New("Square refused the access token")
	_, err = k.svc.ConnectSquare(ctx, k.manager, in)
	assert.ErrorContains(t, err, "Square did not accept the connection: Square refused the access token. Nothing was saved.")
	_, saved, err := creds.Load("square")
	require.NoError(t, err)
	assert.False(t, saved)

	till.err = nil
	res, err := k.svc.ConnectSquare(ctx, k.manager, in)
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

	locs, err := k.svc.SquareLocations(ctx, k.manager)
	require.NoError(t, err)
	assert.Len(t, locs, 1)

	// A saved connection comes back after a restart.
	k.svc.pos, k.svc.posSource = nil, ""
	require.NoError(t, k.svc.loadSavedSquare())
	assert.NotNil(t, k.svc.square())

	// Disconnect refuses while import is on.
	on := integrations.POSSettings{Enabled: true, LocationID: "loc-synthetic", PollSeconds: 5}
	_, err = k.svc.Square().Save(ctx, k.manager, SettingsChange[integrations.POSSettings]{Value: on, Reason: "Go live"})
	require.NoError(t, err)
	err = k.svc.DisconnectSquare(ctx, k.manager)
	assert.ErrorContains(t, err, "Turn it off first")
	on.Enabled = false
	_, err = k.svc.Square().Save(ctx, k.manager, SettingsChange[integrations.POSSettings]{Value: on, Reason: "Pause"})
	require.NoError(t, err)
	require.NoError(t, k.svc.DisconnectSquare(ctx, k.manager))
	assert.Nil(t, k.svc.square())
	_, saved, err = creds.Load("square")
	require.NoError(t, err)
	assert.False(t, saved)
}

func TestSquareTokenFromEnvironmentIsFixed(t *testing.T) {
	k := setupKitchen(t)
	ctx := context.Background()
	k.svc.pos, k.svc.posSource = &fakeTill{}, SourceEnvironment
	k.svc.creds = &integrations.CredentialStore{Dir: t.TempDir()}
	k.svc.newSquare = func(string, string) (integrations.POSAdapter, error) { return &fakeTill{}, nil }

	_, err := k.svc.ConnectSquare(ctx, k.manager, ConnectInput{Environment: "production", Token: "x"})
	assert.ErrorContains(t, err, "set in the server's environment (KISHTIN_SQUARE_TOKEN)")
	assert.ErrorContains(t, k.svc.DisconnectSquare(ctx, k.manager), "Change it there")
	list, err := k.svc.Connectors(ctx, k.cook)
	require.NoError(t, err)
	assert.True(t, list[0].Connected)
	assert.Equal(t, SourceEnvironment, list[0].Source)
}
