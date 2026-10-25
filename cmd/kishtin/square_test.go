package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/integrations/square"
	"github.com/cathalbyrnegit/kishtin/internal/service"
)

// fakeSquare answers like Square's catalog and order search, with
// synthetic data.
func fakeSquare(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/catalog/list":
			_, _ = w.Write([]byte(`{"objects":[
				{"type":"ITEM","id":"item-burger","item_data":{"name":"Synthetic burger","variations":[{"type":"ITEM_VARIATION","id":"var-burger","item_variation_data":{"name":"Regular"}}]}},
				{"type":"ITEM","id":"item-cola","item_data":{"name":"Synthetic cola","variations":[{"type":"ITEM_VARIATION","id":"var-cola","item_variation_data":{"name":"Regular"}}]}}]}`))
		case "/v2/locations":
			if r.Header.Get("Authorization") != "Bearer synthetic-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"errors":[{"code":"UNAUTHORIZED","detail":"This request could not be authorized."}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"locations":[{"id":"loc-synthetic","name":"The Synthetic Kitchen","status":"ACTIVE"}]}`))
		case "/v2/orders/search":
			at := time.Now().UTC().Add(time.Minute).Format(time.RFC3339)
			_, _ = w.Write([]byte(`{"orders":[{"id":"sq-1","version":1,"state":"OPEN","ticket_name":"Table 9","updated_at":"` + at + `",
				"line_items":[{"uid":"l1","catalog_object_id":"var-burger","name":"Synthetic burger","quantity":"2"},
				{"uid":"l2","catalog_object_id":"var-cola","name":"Synthetic cola","quantity":"1"}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSquareCLI(t *testing.T) {
	sq := fakeSquare(t)
	client, err := square.New(sq.URL, "synthetic-token", time.Second)
	require.NoError(t, err)
	url, key := serverWith(t, func(o *service.Options) { o.POS = client })
	cli := func(args ...string) string {
		t.Helper()
		out, _, err := run(t, url, key, "", args...)
		require.NoError(t, err, "%v", args)
		return out
	}
	cli("menu", "station", "add", "--name", "Grill")
	cli("menu", "dish", "add", "--name", "Synthetic burger", "--station", "Grill", "--cook", "8m")

	out := cli("square", "status")
	assert.Contains(t, out, "Square: connected, import is off")
	out = cli("square", "catalog")
	assert.Contains(t, out, "not mapped (suggest: Synthetic burger)")
	cli("square", "map", "var-burger", "--dish", "synthetic burger")
	cli("square", "map", "var-cola", "--ignore")
	out = cli("square", "catalog")
	assert.Contains(t, out, "→ Synthetic burger")
	assert.Contains(t, out, "not for the kitchen")

	_, _, err = run(t, url, key, "", "square", "sync")
	assert.ErrorContains(t, err, "import from Square is off")

	f := filepath.Join(t.TempDir(), "square.json")
	require.NoError(t, os.WriteFile(f, []byte(`{"enabled":true,"location_id":"loc-synthetic","poll_seconds":5}`), 0o600))
	_, preview, err := run(t, url, key, "", "settings", "set", "square", "-f", f, "--reason", "Go live", "--yes")
	require.NoError(t, err)
	assert.Contains(t, preview, "Nothing about the kitchen, staff or HACCP records is sent to Square", "what is sent is shown before saving")

	out = cli("square", "sync")
	assert.Contains(t, out, "1 changed orders: 1 entered")
	out = cli("order", "show", "9")
	assert.Contains(t, out, "Table 9")
	assert.Contains(t, out, "Synthetic burger")
	out = cli("square", "sync")
	assert.Contains(t, out, "1 already up to date")
	out = cli("square", "status")
	assert.Contains(t, out, "importing orders for location loc-synthetic every 5s")
	assert.Contains(t, out, "Table 9")
}

func TestConnectorsCLI(t *testing.T) {
	sq := fakeSquare(t)
	creds := &integrations.CredentialStore{Dir: filepath.Join(t.TempDir(), "connectors")}
	url, key := serverWith(t, func(o *service.Options) {
		o.Credentials = creds
		o.NewSquare = func(_, token string) (integrations.POSAdapter, error) { return square.New(sq.URL, token, time.Second) }
	})
	out, _, err := run(t, url, key, "", "connectors")
	require.NoError(t, err)
	assert.Contains(t, out, "Square: not connected")
	assert.Contains(t, out, "Sends: The access token")

	_, _, err = run(t, url, key, "wrong-token", "connectors", "connect", "square", "--environment", "sandbox", "--token-file", "-")
	assert.ErrorContains(t, err, "Square did not accept the connection")

	out, _, err = run(t, url, key, "synthetic-token\n", "connectors", "connect", "square", "--environment", "sandbox", "--token-file", "-")
	require.NoError(t, err)
	assert.Contains(t, out, "Connected to Square (sandbox).")
	assert.Contains(t, out, "loc-synthetic")
	cr, ok, err := creds.Load("square")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "synthetic-token", cr.Token)

	out, _, err = run(t, url, key, "", "connectors")
	require.NoError(t, err)
	assert.Contains(t, out, "Square: connected (saved, sandbox)")
	out, _, err = run(t, url, key, "", "square", "locations")
	require.NoError(t, err)
	assert.Contains(t, out, "The Synthetic Kitchen")

	out, _, err = run(t, url, key, "", "connectors", "disconnect", "square")
	require.NoError(t, err)
	assert.Contains(t, out, "Disconnected from Square")
	_, ok, err = creds.Load("square")
	require.NoError(t, err)
	assert.False(t, ok)
}
