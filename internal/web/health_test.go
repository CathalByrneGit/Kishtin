package web

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealth(t *testing.T) {
	e := newEnv(t)
	r := e.do(t, http.MethodGet, "/healthz", "", "", nil)
	assert.Equal(t, http.StatusOK, r.status, "needs no key")
	assert.JSONEq(t, `{"status":"ok","version":"dev"}`, string(r.body))
}
