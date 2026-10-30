package web

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectionNeedsOnlyADeviceKey(t *testing.T) {
	e := newEnv(t)
	r := e.do(t, http.MethodGet, "/api/collection", "", "", nil)
	assert.Equal(t, http.StatusUnauthorized, r.status, "a key is still needed")

	r = e.do(t, http.MethodGet, "/api/collection", e.deviceKey, "", nil)
	require.Equal(t, http.StatusOK, r.status, "no staff login: the screen faces customers")
	assert.JSONEq(t, `{"restaurant":"","preparing":[],"ready":[]}`, string(r.body))

	r = e.do(t, http.MethodGet, "/api/kitchen", e.deviceKey, "", nil)
	assert.NotEqual(t, http.StatusOK, r.status, "the rest of the kitchen still needs a login")
}
