package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndVerifySecret(t *testing.T) {
	h, err := HashSecret("4821")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(h, "$argon2id$"))
	assert.True(t, VerifySecret(h, "4821"))
	assert.False(t, VerifySecret(h, "4822"))
	assert.False(t, VerifySecret("garbage", "4821"))
	assert.False(t, VerifySecret("", ""))

	h2, err := HashSecret("4821")
	require.NoError(t, err)
	assert.NotEqual(t, h, h2, "each hash has its own salt")
}

func TestValidatePIN(t *testing.T) {
	for _, ok := range []string{"1234", "12345678"} {
		assert.NoError(t, ValidatePIN(ok), ok)
	}
	for _, bad := range []string{"", "123", "123456789", "12a4", "12 4"} {
		assert.Error(t, ValidatePIN(bad), bad)
	}
}

func TestValidatePassword(t *testing.T) {
	assert.Error(t, ValidatePassword("short"))
	assert.NoError(t, ValidatePassword("long enough pw"))
}

func TestTokens(t *testing.T) {
	tok, hash, err := NewToken(KeyPrefix)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(tok, KeyPrefix))
	assert.Equal(t, HashToken(tok), hash)
	tok2, _, err := NewToken(KeyPrefix)
	require.NoError(t, err)
	assert.NotEqual(t, tok, tok2)
}

func TestCan(t *testing.T) {
	cook := User{Role: RoleLineCook, Active: true}
	mgr := User{Role: RoleManager, Active: true}
	tests := []struct {
		name   string
		p      Principal
		a      Action
		allow  bool
		needPW bool
	}{
		{"cook logs haccp", Principal{User: cook, Method: MethodPIN}, ActLogHACCP, true, false},
		{"cook cannot export", Principal{User: cook, Method: MethodPIN}, ActExportHACCP, false, false},
		{"manager PIN cannot change limits", Principal{User: mgr, Method: MethodPIN}, ActManageLimits, false, true},
		{"manager password changes limits", Principal{User: mgr, Method: MethodPassword}, ActManageLimits, true, false},
		{"manager key changes limits", Principal{User: mgr, Method: MethodKey}, ActManageLimits, true, false},
		{"deactivated manager", Principal{User: User{Role: RoleManager}, Method: MethodPassword}, ActLogHACCP, false, false},
		{"unknown action", Principal{User: mgr, Method: MethodPassword}, Action("nope"), false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Can(tc.p, tc.a)
			assert.Equal(t, tc.allow, d.Allowed)
			assert.Equal(t, tc.needPW, d.NeedPassword)
			if !d.Allowed {
				assert.NotEmpty(t, d.Reason)
			}
		})
	}
}

func TestLockout(t *testing.T) {
	l := NewLockout()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	for range 4 {
		l.Fail("u", now)
	}
	locked, _ := l.Locked("u", now)
	assert.False(t, locked)
	l.Fail("u", now)
	locked, until := l.Locked("u", now)
	assert.True(t, locked)
	assert.Equal(t, now.Add(5*time.Minute), until)
	locked, _ = l.Locked("u", until)
	assert.False(t, locked, "unlocked once the time is up")

	l.Fail("v", now)
	l.Succeed("v")
	for range 4 {
		l.Fail("v", now)
	}
	locked, _ = l.Locked("v", now)
	assert.False(t, locked, "a good login resets the count")
}

func TestPermissionsCan(t *testing.T) {
	chef := User{ID: "u-chef", Role: RoleHeadChef, Active: true}
	cook := User{ID: "u-cook", Role: RoleLineCook, Active: true}
	mgr := User{ID: "u-mgr", Role: RoleManager, Active: true}
	pm := Permissions{Grants: []Grant{
		{Action: ActSignOff, Roles: []Role{RoleHeadChef}, PIN: true},
		{Action: ActInspect, Users: []string{"u-cook"}},
		{Action: ActLogHACCP, Roles: []Role{RoleHeadChef}, PIN: true},
		{Action: ActManageUsers, Roles: []Role{RoleLineCook}, PIN: true},
	}}
	tests := []struct {
		name     string
		u        User
		m        Method
		a        Action
		allowed  bool
		password bool
	}{
		{"granted role, PIN enough", chef, MethodPIN, ActSignOff, true, false},
		{"role not granted", cook, MethodPIN, ActSignOff, false, false},
		{"manager always may", mgr, MethodPIN, ActSignOff, true, false},
		{"named user needs password", cook, MethodPIN, ActInspect, false, true},
		{"named user with password", cook, MethodPassword, ActInspect, true, false},
		{"grant replaces default roles", cook, MethodPIN, ActLogHACCP, false, false},
		{"fixed action ignores grant", cook, MethodPIN, ActManageUsers, false, false},
		{"fixed action, manager PIN", mgr, MethodPIN, ActManageUsers, false, true},
		{"no grant keeps default", cook, MethodPIN, ActWorkStation, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := pm.Can(Principal{User: tc.u, Method: tc.m}, tc.a)
			assert.Equal(t, tc.allowed, d.Allowed)
			assert.Equal(t, tc.password, d.NeedPassword)
		})
	}
	assert.Error(t, pm.Validate(), "a grant for a fixed action is refused")
	assert.NoError(t, Permissions{Grants: pm.Grants[:3]}.Validate())
	for _, a := range Actions() {
		_, ok := policy[a.Action]
		assert.True(t, ok, a.Action)
	}
	assert.Len(t, Actions(), len(policy), "every action is on the permissions screen")
}
