package web

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite"
)

func TestPairATabletOverTheAPI(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "kishtin.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	fs, err := files.New(filepath.Join(dir, "files"))
	require.NoError(t, err)
	svc := service.New(service.Options{
		Store: st, Clock: clock.NewFixed(time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)), Files: fs, Events: events.NewHub(),
		Tablets: &service.TabletNetwork{Addresses: func() []net.IP { return []net.IP{net.ParseIP("192.168.1.20").To4()} }, JoinPort: "8080", HTTPSPort: "8443", Fingerprint: "AB:CD"},
	})
	require.NoError(t, svc.Init(ctx))
	setup, err := svc.Setup(ctx, service.NewUser{Name: "Máire Manager", Password: "synthetic-password"})
	require.NoError(t, err)
	h, err := New(svc, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	r := call(t, srv, http.MethodPost, "/api/devices/join-codes", "", false, map[string]string{"name": "Pass tablet"})
	assert.Equal(t, http.StatusUnauthorized, r.status, "making a code needs a key")
	r = call(t, srv, http.MethodPost, "/api/devices/join-codes", setup.Secret, false, map[string]string{"name": "Pass tablet"})
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	jc := decode[service.JoinCode](t, r)
	require.Len(t, jc.Links, 1)
	assert.Equal(t, "AB:CD", jc.Fingerprint)

	// The tablet has no key: the code is enough, once.
	r = call(t, srv, http.MethodPost, "/api/devices/pair", "", true, map[string]string{"code": jc.Code})
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	p := decode[service.Paired](t, r)
	assert.Equal(t, "Pass tablet", p.Name)
	staff := call(t, srv, http.MethodGet, "/api/auth/staff", p.Secret, true, nil)
	assert.Equal(t, http.StatusOK, staff.status, "the new device key works")
	r = call(t, srv, http.MethodPost, "/api/devices/pair", "", true, map[string]string{"code": jc.Code})
	assert.Equal(t, http.StatusNotFound, r.status)
	assert.Contains(t, string(r.body), "ask a manager for a new QR code")
}
