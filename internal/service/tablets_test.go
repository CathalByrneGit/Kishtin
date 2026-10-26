package service

import (
	"context"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
)

func withTablets(f *fixture, fingerprint string) {
	f.svc.tablets = &TabletNetwork{
		Addresses:   func() []net.IP { return []net.IP{net.ParseIP("192.168.1.20").To4(), net.ParseIP("fd00::20")} },
		JoinPort:    "8080",
		HTTPSPort:   "8443",
		Fingerprint: fingerprint,
	}
}

func TestPairATabletByJoinCode(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	withTablets(f, "AB:CD")
	mgr := &auth.Principal{User: f.manager.User, Method: auth.MethodPassword}

	jc, err := f.svc.CreateJoinCode(ctx, mgr, "  Pass tablet 2 ")
	require.NoError(t, err)
	assert.Equal(t, "Pass tablet 2", jc.Name)
	assert.Equal(t, start.Add(JoinCodeLifetime), jc.ExpiresAt)
	assert.Regexp(t, `^[A-Z2-9]{4}-[A-Z2-9]{4}-[A-Z2-9]{4}$`, jc.Code)
	require.Len(t, jc.Links, 2)
	u, err := url.Parse(jc.Links[0].URL)
	require.NoError(t, err)
	assert.Equal(t, "http://192.168.1.20:8080/join", u.Scheme+"://"+u.Host+u.Path, "starts on the join page, before the tablet trusts Kishtin")
	assert.Equal(t, jc.Code, u.Query().Get("code"))
	assert.Equal(t, "[fd00::20]:8080", strings.SplitN(strings.TrimPrefix(jc.Links[1].URL, "http://"), "/", 2)[0])
	require.NotEmpty(t, jc.Links[0].QR)
	assert.Len(t, jc.Links[0].QR[0], len(jc.Links[0].QR), "square")

	// Typed in lower case without dashes still works, once.
	p, err := f.svc.Pair(ctx, strings.ToLower(strings.ReplaceAll(jc.Code, "-", "")))
	require.NoError(t, err)
	assert.Equal(t, "Pass tablet 2", p.Name)
	assert.Equal(t, auth.KeyDevice, p.Key.Kind)
	c, err := f.svc.Authenticate(ctx, p.Secret, "")
	require.NoError(t, err, "the new device key works")
	assert.Equal(t, p.Key.ID, c.Key.ID)
	_, err = f.svc.Pair(ctx, jc.Code)
	assert.Equal(t, KindNotFound, KindOf(err), "a code works once")

	acts, err := f.svc.Activity(ctx, mgr, ActivityQuery{Kinds: []string{ActivityDevicePaired}})
	require.NoError(t, err)
	require.Len(t, acts, 1)
	assert.Equal(t, "Máire Manager", acts[0].UserName, "names the manager who made the code")
	assert.Equal(t, "Pass tablet 2", acts[0].DeviceName)
}

func TestJoinCodesEndAndNeedAManager(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	mgr := &auth.Principal{User: f.manager.User, Method: auth.MethodPassword}

	_, err := f.svc.CreateJoinCode(ctx, mgr, "Pass tablet")
	assert.Equal(t, KindInvalid, KindOf(err), "tablets turned off")

	withTablets(f, "AB:CD")
	_, err = f.svc.CreateJoinCode(ctx, f.cook, "Pass tablet")
	assert.Equal(t, KindForbidden, KindOf(err))
	_, err = f.svc.CreateJoinCode(ctx, &auth.Principal{User: f.manager.User, Method: auth.MethodPIN}, "Pass tablet")
	assert.Equal(t, KindForbidden, KindOf(err), "a manager's PIN is not enough to add a device")
	_, err = f.svc.CreateJoinCode(ctx, mgr, " ")
	assert.Equal(t, KindInvalid, KindOf(err))

	jc, err := f.svc.CreateJoinCode(ctx, mgr, "Pass tablet")
	require.NoError(t, err)
	f.clk.Advance(JoinCodeLifetime + time.Second)
	_, err = f.svc.Pair(ctx, jc.Code)
	assert.Equal(t, KindNotFound, KindOf(err), "ended")
	_, err = f.svc.Pair(ctx, "AAAA-BBBB-CCCC")
	assert.Equal(t, KindNotFound, KindOf(err))

	f.svc.tablets.Addresses = func() []net.IP { return nil }
	_, err = f.svc.CreateJoinCode(ctx, mgr, "Pass tablet")
	assert.Equal(t, KindInvalid, KindOf(err), "not on a network")
}

func TestJoinLinkWithItsOwnCertificate(t *testing.T) {
	f := setup(t)
	withTablets(f, "")
	jc, err := f.svc.CreateJoinCode(context.Background(), &auth.Principal{User: f.manager.User, Method: auth.MethodPassword}, "Pass tablet")
	require.NoError(t, err)
	assert.Equal(t, "https://192.168.1.20:8443/#join="+jc.Code, jc.Links[0].URL, "goes straight to the secure address")
}
