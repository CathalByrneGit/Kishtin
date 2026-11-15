package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

// TestServe starts the real server on a new data folder, as `kishtind`
// does, and stops it.
func TestServe(t *testing.T) {
	dir := t.TempDir()
	addr := freePort(t)
	t.Setenv("KISHTIN_DATA_DIR", dir)
	t.Setenv("KISHTIN_CONFIG", "")
	t.Setenv("KISHTIN_LISTEN", addr)
	t.Setenv("KISHTIN_TABLETS", "false")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, "") }()

	get := func(path string) (int, []byte) {
		t.Helper()
		var lastErr error
		for range 100 {
			res, err := http.Get("http://" + addr + path)
			if err != nil {
				lastErr = err
				select {
				case err := <-done:
					t.Fatalf("the server stopped: %v", err)
				case <-time.After(50 * time.Millisecond):
				}
				continue
			}
			defer res.Body.Close()
			b, _ := io.ReadAll(res.Body)
			return res.StatusCode, b
		}
		t.Fatalf("the server did not answer: %v", lastErr)
		return 0, nil
	}

	code, body := get("/healthz")
	assert.Equal(t, http.StatusOK, code)
	var health struct{ Status, Version string }
	require.NoError(t, json.Unmarshal(body, &health))
	assert.Equal(t, "ok", health.Status)
	assert.Equal(t, version, health.Version)

	// A new install needs setting up, and the API needs a key.
	code, body = get("/api/setup")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), `"needed":true`)
	code, _ = get("/api/haccp/today")
	assert.Equal(t, http.StatusUnauthorized, code)
	code, _ = get("/api/openapi.json")
	assert.Equal(t, http.StatusUnauthorized, code)

	// The data folder is held, so a second server is refused.
	err := serve(context.Background(), "")
	var busy busyError
	require.ErrorAs(t, err, &busy)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err, "stops cleanly")
	case <-time.After(10 * time.Second):
		t.Fatal("the server did not stop")
	}
	_, err = os.Stat(filepath.Join(dir, "kishtin.db"))
	require.NoError(t, err)

	// With the server stopped, migrate runs and finds nothing to do.
	out, _, err := run(t, "", "migrate")
	require.NoError(t, err)
	assert.Contains(t, out, "Database is up to date")
}
