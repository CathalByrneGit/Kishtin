package web

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventsStream(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.srv.URL+"/api/events", nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	res.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode, "the stream needs a key")

	req.Header.Set("Authorization", "Bearer "+e.deviceKey)
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))

	lines := bufio.NewScanner(res.Body)
	next := func() string {
		for lines.Scan() {
			if l := lines.Text(); strings.HasPrefix(l, "data:") || strings.HasPrefix(l, "event:") {
				return l
			}
		}
		return ""
	}
	assert.Equal(t, "event: ready", next())
	next() // ready data

	r := e.do(t, http.MethodPost, "/api/haccp/records", e.mgrKey, "", map[string]any{
		"type": "fridge", "reading": map[string]any{"unit": "Walk-in fridge", "temp_c": 3.4},
	})
	require.Equal(t, http.StatusCreated, r.status, string(r.body))
	assert.Equal(t, "event: change", next())
	assert.Contains(t, next(), `"kind":"record"`)
}
