package web

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/service"
)

func TestSquareOverHTTP(t *testing.T) {
	e := newEnv(t)
	key := e.mgrKey

	r := e.do(t, http.MethodGet, "/api/square", key, "", nil)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	st := decode[service.SquareStatus](t, r)
	assert.False(t, st.Connected)
	assert.False(t, st.Settings.Value.Enabled)

	r = e.do(t, http.MethodGet, "/api/settings/square", key, "", nil)
	require.Equal(t, http.StatusOK, r.status, string(r.body))

	r = e.do(t, http.MethodPost, "/api/settings/square/preview", key, "", map[string]any{
		"value": map[string]any{"enabled": true, "location_id": "loc-synthetic", "poll_seconds": 5}, "reason": "Go live",
	})
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	assert.Contains(t, string(r.body), "No Square access token is set on the server")

	r = e.do(t, http.MethodGet, "/api/square/catalog", key, "", nil)
	assert.Equal(t, http.StatusConflict, r.status)
	r = e.do(t, http.MethodPost, "/api/square/sync", key, "", nil)
	assert.Equal(t, http.StatusConflict, r.status)
	assert.Contains(t, string(r.body), "Order import from Square is off")
	r = e.do(t, http.MethodPut, "/api/square/mappings", key, "", map[string]any{"mappings": []map[string]any{
		{"external_id": "var-x", "kind": "item", "name": "X"},
	}})
	assert.Equal(t, http.StatusUnprocessableEntity, r.status)
	assert.Contains(t, string(r.body), "choose the dish")
}
