package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/integrations/sumup"
	"github.com/cathalbyrnegit/kishtin/internal/service"
)

// fakeSumUp answers like SumUp's API, with one synthetic merchant account
// and one paid sale.
func fakeSumUp(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sup_sk_synthetic" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Authentication credentials are missing or invalid."})
			return
		}
		switch r.URL.Path {
		case "/v0.1/memberships":
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "items": []any{map[string]any{
				"resource_id": "MSYNTH01", "type": "merchant", "status": "accepted", "resource": map[string]any{"id": "MSYNTH01", "name": "The Synthetic Kitchen"},
			}}})
		case "/v2.1/merchants/MSYNTH01/transactions/history":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"id": "tx-1", "transaction_code": "TSYNTH1", "status": "SUCCESSFUL", "type": "PAYMENT"}}, "links": []any{}})
		case "/v2.1/merchants/MSYNTH01/transactions":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "tx-1", "transaction_code": "TSYNTH1", "status": "SUCCESSFUL", "timestamp": time.Now().UTC().Format(time.RFC3339),
				"products": []any{map[string]any{"name": "Synthetic flat white", "quantity": 1}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSumUpCLI(t *testing.T) {
	su := fakeSumUp(t)
	creds := &integrations.CredentialStore{Dir: filepath.Join(t.TempDir(), "connectors")}
	url, key := serverWith(t, func(o *service.Options) {
		o.Credentials = creds
		o.Tills = map[string]service.TillConnection{service.TillSumUp: {New: func(_, token string) (integrations.POSAdapter, error) {
			return sumup.New(su.URL, token, time.Second)
		}}}
	})
	out, _, err := run(t, url, key, "", "connectors")
	require.NoError(t, err)
	assert.Contains(t, out, "SumUp: not connected")

	_, _, err = run(t, url, key, "sup_sk_wrong", "connectors", "connect", "sumup", "--token-file", "-")
	assert.ErrorContains(t, err, "SumUp did not accept the connection")
	_, _, err = run(t, url, key, "sup_sk_synthetic", "connectors", "connect", "sumup", "--environment", "sandbox", "--token-file", "-")
	assert.ErrorContains(t, err, "environment must be production")

	out, _, err = run(t, url, key, "sup_sk_synthetic\n", "connectors", "connect", "sumup", "--token-file", "-")
	require.NoError(t, err)
	assert.Contains(t, out, "Connected to SumUp (production).")
	assert.Contains(t, out, "MSYNTH01")

	out, _, err = run(t, url, key, "", "sumup", "status")
	require.NoError(t, err)
	assert.Contains(t, out, "SumUp: connected, import is off")
	out, _, err = run(t, url, key, "", "sumup", "catalog")
	require.NoError(t, err)
	assert.Contains(t, out, "Synthetic flat white", "items sold in recent sales")
	assert.Contains(t, out, "name:synthetic flat white")
	out, _, err = run(t, url, key, "", "sumup", "map", "name:synthetic flat white", "--ignore")
	require.NoError(t, err)
	assert.Contains(t, out, "Mapped Synthetic flat white")

	_, _, err = run(t, url, key, "", "connectors", "connect", "lightspeed", "--token-file", "-")
	assert.ErrorContains(t, err, "Kishtin connects to square and sumup")
}
