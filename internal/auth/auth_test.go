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
		{"cook logs haccp", Principal{cook, MethodPIN}, ActLogHACCP, true, false},
		{"cook cannot export", Principal{cook, MethodPIN}, ActExportHACCP, false, false},
		{"manager PIN cannot change limits", Principal{mgr, MethodPIN}, ActManageLimits, false, true},
		{"manager password changes limits", Principal{mgr, MethodPassword}, ActManageLimits, true, false},
		{"manager key changes limits", Principal{mgr, MethodKey}, ActManageLimits, true, false},
		{"deactivated manager", Principal{User{Role: RoleManager}, MethodPassword}, ActLogHACCP, false, false},
		{"unknown action", Principal{mgr, MethodPassword}, Action("nope"), false, false},
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
