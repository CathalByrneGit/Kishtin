package web

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite"
)

type env struct {
	srv       *httptest.Server
	clk       *clock.Fixed
	mgrKey    string
	deviceKey string
	cookID    string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "kishtin.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	fs, err := files.New(filepath.Join(dir, "files"), 1<<20)
	require.NoError(t, err)
	clk := clock.NewFixed(time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC))
	svc := service.New(service.Options{Store: st, Clock: clk, Files: fs, Backups: service.BackupConfig{
		Dir:  filepath.Join(dir, "backups"),
		Open: func(ctx context.Context, p string) (store.Store, error) { return sqlite.OpenSnapshot(ctx, p) },
	}})
	require.NoError(t, svc.Init(ctx))
	setup, err := svc.Setup(ctx, service.NewUser{Name: "Máire Manager", Password: "synthetic-password"})
	require.NoError(t, err)
	mgr := &auth.Principal{User: setup.User, Method: auth.MethodKey}
	cook, err := svc.CreateUser(ctx, mgr, service.NewUser{Name: "Liam Cook", Role: auth.RoleLineCook, PIN: "1234"})
	require.NoError(t, err)
	dev, err := svc.CreateKey(ctx, mgr, auth.KeyDevice, "Pass tablet", "")
	require.NoError(t, err)

	h, err := New(svc, slog.New(slog.NewTextHandler(io.Discard, nil)), 1<<20)
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{srv: srv, clk: clk, mgrKey: setup.Secret, deviceKey: dev.Secret, cookID: cook.ID}
}

type resp struct {
	status int
	header http.Header
	body   []byte
}

func (e *env) do(t *testing.T, method, path, key, session string, body any) resp {
	t.Helper()
	var r io.Reader
	if b, ok := body.([]byte); ok {
		r = bytes.NewReader(b)
	} else if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, r)
	require.NoError(t, err)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if session != "" {
		req.Header.Set(SessionHeader, session)
	}
	if _, raw := body.([]byte); raw {
		req.Header.Set("Content-Type", "application/octet-stream")
	} else if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return resp{res.StatusCode, res.Header, b}
}

func decode[T any](t *testing.T, r resp) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(r.body, &v), string(r.body))
	return v
}

func TestEveryAPIRouteNeedsAKey(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/api/auth/staff", "/api/haccp/records", "/api/haccp/verify", "/api/openapi.json", "/api/users"} {
		r := e.do(t, http.MethodGet, p, "", "", nil)
		assert.Equal(t, http.StatusUnauthorized, r.status, p)
		r = e.do(t, http.MethodGet, p, "kishtin_key_not-real", "", nil)
		assert.Equal(t, http.StatusUnauthorized, r.status, p)
	}
	home := e.do(t, http.MethodGet, "/", "", "", nil)
	assert.Equal(t, http.StatusOK, home.status)
	assert.Contains(t, string(home.body), "Kishtin is running")
}

func TestDeviceLoginAndLogRecord(t *testing.T) {
	e := newEnv(t)
	staff := decode[struct {
		Staff []service.StaffEntry `json:"staff"`
	}](t, e.do(t, http.MethodGet, "/api/auth/staff", e.deviceKey, "", nil))
	require.Len(t, staff.Staff, 2)

	r := e.do(t, http.MethodPost, "/api/haccp/records", e.deviceKey, "", map[string]any{
		"type": "fridge", "reading": map[string]any{"unit": "Walk-in", "temp_c": 3.4},
	})
	assert.Equal(t, http.StatusUnauthorized, r.status, "a device key alone cannot log records")

	r = e.do(t, http.MethodPost, "/api/auth/login", e.deviceKey, "", map[string]any{"user_id": e.cookID, "pin": "0000"})
	assert.Equal(t, http.StatusUnauthorized, r.status)
	r = e.do(t, http.MethodPost, "/api/auth/login", e.deviceKey, "", map[string]any{"user_id": e.cookID, "pin": "1234"})
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	login := decode[service.LoginResult](t, r)
	assert.True(t, strings.HasPrefix(login.Token, auth.SessionPrefix))

	me := decode[struct {
		User *auth.User `json:"user"`
	}](t, e.do(t, http.MethodGet, "/api/auth/me", e.deviceKey, login.Token, nil))
	require.NotNil(t, me.User)
	assert.Equal(t, "Liam Cook", me.User.Name)

	r = e.do(t, http.MethodPost, "/api/haccp/check", e.deviceKey, login.Token, map[string]any{
		"type": "fridge", "reading": map[string]any{"unit": "Walk-in", "temp_c": 6.2},
	})
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	chk := decode[service.Check](t, r)
	assert.Equal(t, haccp.OutcomeFail, chk.Result.Outcome)

	r = e.do(t, http.MethodPost, "/api/haccp/records", e.deviceKey, login.Token, map[string]any{
		"type": "fridge", "reading": map[string]any{"unit": "Walk-in", "temp_c": 6.2},
	})
	assert.Equal(t, http.StatusUnprocessableEntity, r.status)
	assert.Contains(t, string(r.body), "Add a corrective action")

	r = e.do(t, http.MethodPost, "/api/haccp/records", e.deviceKey, login.Token, map[string]any{
		"type": "fridge", "reading": map[string]any{"unit": "Walk-in", "temp_c": 3.4},
	})
	require.Equal(t, http.StatusCreated, r.status, string(r.body))
	rec := decode[haccp.Record](t, r)
	assert.Equal(t, haccp.OutcomePass, rec.Outcome)

	got := decode[haccp.Record](t, e.do(t, http.MethodGet, "/api/haccp/records/"+rec.ID, e.deviceKey, login.Token, nil))
	assert.Equal(t, rec.Hash, got.Hash)

	r = e.do(t, http.MethodGet, "/api/haccp/export?from=2026-05-01&to=2026-05-01", e.deviceKey, login.Token, nil)
	assert.Equal(t, http.StatusForbidden, r.status, "line cooks cannot export")

	r = e.do(t, http.MethodPost, "/api/auth/logout", e.deviceKey, login.Token, nil)
	assert.Equal(t, http.StatusNoContent, r.status)
	r = e.do(t, http.MethodGet, "/api/auth/me", e.deviceKey, login.Token, nil)
	assert.Equal(t, http.StatusUnauthorized, r.status, "the session has ended")
}

func TestManagerFlows(t *testing.T) {
	e := newEnv(t)
	key := e.mgrKey
	for _, temp := range []float64{3.4, 4.6} {
		r := e.do(t, http.MethodPost, "/api/haccp/records", key, "", map[string]any{
			"type": "fridge", "reading": map[string]any{"unit": "Walk-in", "temp_c": temp},
		})
		require.Equal(t, http.StatusCreated, r.status, string(r.body))
	}

	list := decode[struct {
		Records []haccp.Record `json:"records"`
	}](t, e.do(t, http.MethodGet, "/api/haccp/records?type=fridge&newest=true&limit=1", key, "", nil))
	require.Len(t, list.Records, 1)
	assert.Equal(t, int64(2), list.Records[0].Seq)

	r := e.do(t, http.MethodGet, "/api/haccp/records?type=nope", key, "", nil)
	assert.Equal(t, http.StatusUnprocessableEntity, r.status)
	r = e.do(t, http.MethodGet, "/api/haccp/records?from=yesterday", key, "", nil)
	assert.Equal(t, http.StatusUnprocessableEntity, r.status)

	ver := decode[service.VerifyReport](t, e.do(t, http.MethodGet, "/api/haccp/verify", key, "", nil))
	assert.True(t, ver.OK)
	assert.Equal(t, 2, ver.Chain.Checked)

	r = e.do(t, http.MethodGet, "/api/haccp/export?from=2026-05-01&to=2026-05-01", key, "", nil)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	assert.Equal(t, "text/csv; charset=utf-8", r.header.Get("Content-Type"))
	assert.Equal(t, "2", r.header.Get("X-Kishtin-Records"))
	assert.Equal(t, ver.Chain.HeadHash, r.header.Get("X-Kishtin-Chain-Head"))
	rows, err := csv.NewReader(bytes.NewReader(r.body)).ReadAll()
	require.NoError(t, err)
	assert.Len(t, rows, 3)

	limit := map[string]any{
		"check": "fridge", "scope": map[string]any{"kind": "unit", "ref": "Walk-in"},
		"max_c": 4.0, "reason": "Fish stored in the walk-in",
	}
	pv := decode[service.LimitPreview](t, e.do(t, http.MethodPost, "/api/haccp/limits/preview", key, "", limit))
	assert.Equal(t, 1, pv.Changed)
	r = e.do(t, http.MethodPost, "/api/haccp/limits", key, "", limit)
	require.Equal(t, http.StatusCreated, r.status, string(r.body))

	r = e.do(t, http.MethodGet, "/api/haccp/limits/export", key, "", nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Contains(t, string(r.body), "Fish stored in the walk-in")

	r = e.do(t, http.MethodPost, "/api/backups", key, "", nil)
	require.Equal(t, http.StatusCreated, r.status, string(r.body))
	b := decode[service.Backup](t, r)
	assert.True(t, b.Verified)

	r = e.do(t, http.MethodPost, "/api/keys", key, "", map[string]any{"kind": "device", "name": "Grill tablet"})
	require.Equal(t, http.StatusCreated, r.status, string(r.body))
	nk := decode[service.NewKeyResult](t, r)
	r = e.do(t, http.MethodDelete, "/api/keys/"+nk.Key.ID, key, "", nil)
	assert.Equal(t, http.StatusNoContent, r.status)
	r = e.do(t, http.MethodGet, "/api/auth/staff", nk.Secret, "", nil)
	assert.Equal(t, http.StatusUnauthorized, r.status, "a revoked key stops working")

	spec := e.do(t, http.MethodGet, "/api/openapi.json", key, "", nil)
	require.Equal(t, http.StatusOK, spec.status)
	assert.Contains(t, string(spec.body), `"log-record"`)
}

func TestManagerPINCannotChangeLimits(t *testing.T) {
	e := newEnv(t)
	ctxUsers := decode[struct {
		Users []auth.User `json:"users"`
	}](t, e.do(t, http.MethodGet, "/api/users", e.mgrKey, "", nil))
	var mgrID string
	for _, u := range ctxUsers.Users {
		if u.Role == auth.RoleManager {
			mgrID = u.ID
		}
	}
	pin := "4321"
	r := e.do(t, http.MethodPatch, "/api/users/"+mgrID, e.mgrKey, "", map[string]any{"pin": pin})
	require.Equal(t, http.StatusOK, r.status, string(r.body))

	login := decode[service.LoginResult](t, e.do(t, http.MethodPost, "/api/auth/login", e.deviceKey, "", map[string]any{"user_id": mgrID, "pin": pin}))
	limit := map[string]any{"check": "fridge", "scope": map[string]any{"kind": "default"}, "max_c": 4.0, "reason": "Tighter"}
	r = e.do(t, http.MethodPost, "/api/haccp/limits", e.deviceKey, login.Token, limit)
	assert.Equal(t, http.StatusForbidden, r.status)
	assert.Contains(t, string(r.body), "manager password")

	pw := decode[service.LoginResult](t, e.do(t, http.MethodPost, "/api/auth/login", e.deviceKey, "", map[string]any{"user_id": mgrID, "password": "synthetic-password"}))
	r = e.do(t, http.MethodPost, "/api/haccp/limits", e.deviceKey, pw.Token, limit)
	assert.Equal(t, http.StatusCreated, r.status, string(r.body))

	e.clk.Advance(auth.PasswordSessionTTL)
	r = e.do(t, http.MethodPost, "/api/haccp/limits", e.deviceKey, pw.Token, limit)
	assert.Equal(t, http.StatusUnauthorized, r.status, "password sessions are short")
}

func TestFileUploadAndDownload(t *testing.T) {
	e := newEnv(t)
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0}
	r := e.do(t, http.MethodPost, "/api/files?name=docket.png", e.mgrKey, "", png)
	require.Equal(t, http.StatusCreated, r.status, string(r.body))
	m := decode[store.FileMeta](t, r)
	assert.Equal(t, "image/png", m.MIME)

	r = e.do(t, http.MethodPost, "/api/files", e.mgrKey, "", []byte("not an image"))
	assert.Equal(t, http.StatusUnprocessableEntity, r.status)

	r = e.do(t, http.MethodGet, "/api/files/"+m.SHA256, e.mgrKey, "", nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, png, r.body)
	assert.Equal(t, "image/png", r.header.Get("Content-Type"))

	r = e.do(t, http.MethodGet, "/api/files/"+strings.Repeat("0", 64), e.mgrKey, "", nil)
	assert.Equal(t, http.StatusNotFound, r.status)
}
