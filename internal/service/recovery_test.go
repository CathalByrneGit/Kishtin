package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
)

func TestRecoverTheOnlyManager(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, err := f.svc.Authenticate(ctx, f.device, "")
	require.NoError(t, err)
	for range 6 {
		_, _ = f.svc.LoginPassword(ctx, c, f.manager.User.ID, "forgotten-password")
	}
	_, err = f.svc.LoginPassword(ctx, c, f.manager.User.ID, "synthetic-password")
	require.Equal(t, KindLocked, KindOf(err), "locked out by wrong tries")

	_, err = f.svc.RecoverManager(ctx, Recovery{Password: "short"})
	assert.Equal(t, KindInvalid, KindOf(err), "the new password follows the usual rules")

	res, err := f.svc.RecoverManager(ctx, Recovery{Password: "a-new-synthetic-password"})
	require.NoError(t, err)
	assert.Equal(t, "Máire Manager", res.User.Name, "with one manager, no name is needed")
	assert.False(t, res.Created || res.Promoted)

	_, err = f.svc.LoginPassword(ctx, c, f.manager.User.ID, "a-new-synthetic-password")
	require.NoError(t, err, "the lockout is cleared and the new password works")
	_, err = f.svc.LoginPIN(ctx, c, f.manager.User.ID, "9999")
	require.NoError(t, err, "the PIN is unchanged")

	acts, err := f.svc.Activity(ctx, f.manager, ActivityQuery{Kinds: []string{ActivityPasswordRecovered}})
	require.NoError(t, err)
	require.Len(t, acts, 1)
	assert.Equal(t, "Máire Manager", acts[0].UserName)
	assert.Equal(t, RecoveryDevice, acts[0].DeviceName)
}

func TestRecoverWithSeveralManagers(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.svc.CreateUser(ctx, f.manager, NewUser{Name: "Aoife Chef", Role: auth.RoleManager, Password: "synthetic-password"})
	require.NoError(t, err)

	_, err = f.svc.RecoverManager(ctx, Recovery{Password: "a-new-synthetic-password"})
	require.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "Aoife Chef, Máire Manager", "lists the managers to choose from")

	for _, name := range []string{"Liam Cook", "Nobody Here"} {
		_, err = f.svc.RecoverManager(ctx, Recovery{Name: name, Password: "a-new-synthetic-password"})
		assert.Equal(t, KindInvalid, KindOf(err), "%s cannot be made a manager while managers remain", name)
	}
	res, err := f.svc.RecoverManager(ctx, Recovery{Name: "aoife chef", Password: "a-new-synthetic-password"})
	require.NoError(t, err)
	assert.Equal(t, "Aoife Chef", res.User.Name, "names match in any case")
}

func TestRecoverWhenNoManagerIsLeft(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name              string
		created, promoted bool
	}{
		{"Liam Cook", false, true},
		{"Síle New", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			// The last manager has left and been deactivated.
			left := f.manager.User
			left.Active = false
			require.NoError(t, f.svc.st.Users().UpdateUser(ctx, left))

			_, err := f.svc.RecoverManager(ctx, Recovery{Password: "a-new-synthetic-password"})
			assert.Equal(t, KindInvalid, KindOf(err), "a name is needed when no manager is left")

			res, err := f.svc.RecoverManager(ctx, Recovery{Name: tc.name, Password: "a-new-synthetic-password"})
			require.NoError(t, err)
			assert.Equal(t, tc.created, res.Created)
			assert.Equal(t, tc.promoted, res.Promoted)
			assert.Equal(t, auth.RoleManager, res.User.Role)
			assert.True(t, res.User.Active)
		})
	}
}
