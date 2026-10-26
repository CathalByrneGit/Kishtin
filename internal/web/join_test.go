package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func joinServer(t *testing.T, tablets *Tablets) *httptest.Server {
	t.Helper()
	h, err := New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	require.NoError(t, err)
	if tablets != nil {
		h.SetTablets(tablets)
	}
	s := httptest.NewServer(h.ForNetwork())
	t.Cleanup(s.Close)
	return s
}

func get(t *testing.T, srv *httptest.Server, method, path string, remote bool) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, nil)
	require.NoError(t, err)
	if remote {
		req.Header.Set("X-Forwarded-For", "192.168.1.40")
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func body(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return string(b)
}

var testTablets = &Tablets{CA: []byte("synthetic DER"), MobileConfig: []byte("<plist/>"), Fingerprint: "AB:CD:EF", HTTPSPort: "8443"}

func TestOtherDevicesOnlyGetTheJoinPages(t *testing.T) {
	srv := joinServer(t, testTablets)

	// This computer gets Kishtin as before.
	home := get(t, srv, http.MethodGet, "/", false)
	assert.Equal(t, http.StatusOK, home.StatusCode)
	assert.Contains(t, body(t, home), "Kishtin is running")

	// Another device is sent to HTTPS on the same address.
	res := get(t, srv, http.MethodGet, "/pass?x=1", true)
	assert.Equal(t, http.StatusFound, res.StatusCode)
	host := strings.TrimPrefix(srv.URL, "http://")
	host = host[:strings.LastIndex(host, ":")]
	assert.Equal(t, "https://"+host+":8443/pass?x=1", res.Header.Get("Location"))
	api := get(t, srv, http.MethodPost, "/api/auth/login", true)
	assert.Equal(t, http.StatusForbidden, api.StatusCode, "no API over plain HTTP from the network")

	page := get(t, srv, http.MethodGet, "/join?code=ABCD-EFGH-JKMN", true)
	require.Equal(t, http.StatusOK, page.StatusCode)
	html := body(t, page)
	assert.Contains(t, html, `value="ABCD-EFGH-JKMN"`)
	assert.Contains(t, html, "AB:CD:EF", "shows the fingerprint")
	assert.Contains(t, html, `":8443"`)
	assert.Contains(t, page.Header.Get("Content-Security-Policy"), "script-src 'nonce-")

	crt := get(t, srv, http.MethodGet, "/join/kishtin-ca.crt", true)
	assert.Equal(t, "application/x-x509-ca-cert", crt.Header.Get("Content-Type"))
	assert.Equal(t, "synthetic DER", body(t, crt))
	mc := get(t, srv, http.MethodGet, "/join/kishtin.mobileconfig", true)
	assert.Equal(t, "application/x-apple-aspen-config", mc.Header.Get("Content-Type"))

	ping := get(t, srv, http.MethodGet, "/join/ping", true)
	assert.Equal(t, http.StatusNoContent, ping.StatusCode)
	assert.Equal(t, "*", ping.Header.Get("Access-Control-Allow-Origin"), "the plain join page may ask")
}

func TestJoinPageEscapesTheCode(t *testing.T) {
	srv := joinServer(t, testTablets)
	html := body(t, get(t, srv, http.MethodGet, `/join?code="><script>alert(1)</script>`, true))
	assert.NotContains(t, html, "<script>alert(1)")
}

func TestTabletsOff(t *testing.T) {
	srv := joinServer(t, nil)
	assert.Equal(t, http.StatusForbidden, get(t, srv, http.MethodGet, "/pass", true).StatusCode)
	assert.Contains(t, body(t, get(t, srv, http.MethodGet, "/join", true)), "Tablets are turned off")
	assert.Equal(t, http.StatusNotFound, get(t, srv, http.MethodGet, "/join/kishtin-ca.crt", true).StatusCode)
}
