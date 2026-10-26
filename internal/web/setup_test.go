package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite"
)

// newBlankServer is a server for a new install, with no staff.
func newBlankServer(t *testing.T, code string) (*httptest.Server, *bool) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "kishtin.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	fs, err := files.New(filepath.Join(dir, "files"))
	require.NoError(t, err)
	svc := service.New(service.Options{Store: st, Clock: clock.NewFixed(time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)), Files: fs, Events: events.NewHub()})
	require.NoError(t, svc.Init(ctx))
	h, err := New(svc, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	require.NoError(t, err)
	done := false
	h.SetSetupCode(code, func() { done = true })
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, &done
}

// call sends a request; remote marks it as passed on from another device.
func call(t *testing.T, srv *httptest.Server, method, path, key string, remote bool, body any) resp {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, r)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if remote {
		req.Header.Set("X-Forwarded-For", "192.168.1.40")
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return resp{res.StatusCode, res.Header, b}
}

type setupStatus struct {
	Needed     bool `json:"needed"`
	CodeNeeded bool `json:"code_needed"`
	Possible   bool `json:"possible"`
}

func firstRun(code string) map[string]any {
	return map[string]any{"restaurant": "The Synthetic Kitchen", "name": "Máire Manager", "password": "synthetic-password", "device": "Office PC", "code": code}
}

func TestSetupFromThisMachine(t *testing.T) {
	srv, done := newBlankServer(t, "ABCD-EFGH-JKLM")

	st := decode[setupStatus](t, call(t, srv, http.MethodGet, "/api/setup", "", false, nil))
	assert.Equal(t, setupStatus{Needed: true, Possible: true}, st)

	bad := firstRun("")
	bad["restaurant"] = " "
	r := call(t, srv, http.MethodPost, "/api/setup", "", false, bad)
	assert.Equal(t, http.StatusUnprocessableEntity, r.status, string(r.body))
	assert.False(t, *done)

	r = call(t, srv, http.MethodPost, "/api/setup", "", false, firstRun(""))
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	res := decode[service.FirstRunResult](t, r)
	assert.Equal(t, "Máire Manager", res.User.Name)
	assert.True(t, *done)

	// The device key pairs this screen; the manager logs in on it with
	// their password.
	staff := call(t, srv, http.MethodGet, "/api/auth/staff", res.DeviceKey, false, nil)
	require.Equal(t, http.StatusOK, staff.status)
	login := call(t, srv, http.MethodPost, "/api/auth/login", res.DeviceKey, false, map[string]string{"user_id": res.User.ID, "password": "synthetic-password"})
	require.Equal(t, http.StatusOK, login.status, string(login.body))
	sys := call(t, srv, http.MethodGet, "/api/settings/system", res.APIKey, false, nil)
	require.Equal(t, http.StatusOK, sys.status, string(sys.body))
	assert.Contains(t, string(sys.body), "The Synthetic Kitchen")

	// Once set up, setup is closed.
	st = decode[setupStatus](t, call(t, srv, http.MethodGet, "/api/setup", "", false, nil))
	assert.Equal(t, setupStatus{}, st)
	again := call(t, srv, http.MethodPost, "/api/setup", "", false, firstRun(""))
	assert.Equal(t, http.StatusConflict, again.status)
}

func TestSetupFromAnotherDeviceNeedsTheCode(t *testing.T) {
	srv, _ := newBlankServer(t, "ABCD-EFGH-JKLM")
	st := decode[setupStatus](t, call(t, srv, http.MethodGet, "/api/setup", "", true, nil))
	assert.Equal(t, setupStatus{Needed: true, CodeNeeded: true, Possible: true}, st)

	for _, code := range []string{"", "ABCD-EFGH-JKLN"} {
		r := call(t, srv, http.MethodPost, "/api/setup", "", true, firstRun(code))
		assert.Equal(t, http.StatusForbidden, r.status, code)
	}
	r := call(t, srv, http.MethodPost, "/api/setup", "", true, firstRun("abcd efgh jklm"))
	assert.Equal(t, http.StatusOK, r.status, string(r.body))
}

func TestSetupWithoutACodeOnlyOnThisMachine(t *testing.T) {
	srv, _ := newBlankServer(t, "")
	st := decode[setupStatus](t, call(t, srv, http.MethodGet, "/api/setup", "", true, nil))
	assert.Equal(t, setupStatus{Needed: true, CodeNeeded: true}, st)
	r := call(t, srv, http.MethodPost, "/api/setup", "", true, firstRun(""))
	assert.Equal(t, http.StatusForbidden, r.status)
}

func TestOnlySetupIsPublic(t *testing.T) {
	srv, _ := newBlankServer(t, "")
	h, err := New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	require.NoError(t, err)
	var public []string
	for _, item := range h.OpenAPI().Paths {
		for _, op := range []*huma.Operation{item.Get, item.Post, item.Put, item.Patch, item.Delete} {
			if op != nil && op.Metadata[publicKey] == true {
				public = append(public, op.OperationID)
				assert.Empty(t, op.Security, op.OperationID)
			}
		}
	}
	assert.ElementsMatch(t, []string{"setup-status", "setup"}, public)
	// A blank install still needs a key for everything else.
	for _, p := range []string{"/api/auth/staff", "/api/haccp/records", "/api/users"} {
		assert.Equal(t, http.StatusUnauthorized, call(t, srv, http.MethodGet, p, "", false, nil).status, p)
	}
}
