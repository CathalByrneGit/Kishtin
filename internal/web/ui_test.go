package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeUI(t *testing.T) {
	ui := fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html><title>Kishtin</title>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
	}
	h, err := New(nil, slog.New(slog.DiscardHandler), ui)
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	defer srv.Close()

	get := func(p string) (*http.Response, string) {
		res, err := http.Get(srv.URL + p)
		require.NoError(t, err)
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}

	res, body := get("/")
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, "<title>Kishtin</title>")
	assert.Equal(t, "no-store", res.Header.Get("Cache-Control"))
	assert.Contains(t, res.Header.Get("Content-Security-Policy"), "default-src 'self'")

	res, body = get("/assets/app-1.js")
	assert.Equal(t, "console.log(1)", body)
	assert.Contains(t, res.Header.Get("Cache-Control"), "immutable")

	_, body = get("/records")
	assert.Contains(t, body, "<title>Kishtin</title>", "app paths get index.html")

	res, _ = get("/api/nope")
	assert.Equal(t, http.StatusNotFound, res.StatusCode, "unknown API paths are not the app")
}
