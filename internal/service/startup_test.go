package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/autostart"
)

type fakeStartup struct {
	on   bool
	fail bool
}

func (f *fakeStartup) Status(context.Context) (autostart.Status, error) {
	return autostart.Status{Supported: true, On: f.on, Method: "systemd user service"}, nil
}

func (f *fakeStartup) Enable(ctx context.Context) (autostart.Status, error) {
	if f.fail {
		return autostart.Status{}, errors.New("systemctl: permission denied")
	}
	f.on = true
	return f.Status(ctx)
}

func (f *fakeStartup) Disable(ctx context.Context) (autostart.Status, error) {
	f.on = false
	return f.Status(ctx)
}

func TestStartup(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	st, err := f.svc.StartupStatus(ctx, f.manager)
	require.NoError(t, err)
	assert.False(t, st.Supported, "a server that cannot change how it starts says so")
	_, err = f.svc.SetStartup(ctx, f.manager, true)
	assert.Equal(t, KindConflict, KindOf(err))

	fake := &fakeStartup{}
	f.svc.startup = fake
	_, err = f.svc.SetStartup(ctx, f.cook, true)
	assert.Equal(t, KindForbidden, KindOf(err), "managers only")

	st, err = f.svc.SetStartup(ctx, f.manager, true)
	require.NoError(t, err)
	assert.True(t, st.On)
	st, err = f.svc.SetStartup(ctx, f.manager, false)
	require.NoError(t, err)
	assert.False(t, st.On)

	fake.fail = true
	_, err = f.svc.SetStartup(ctx, f.manager, true)
	assert.ErrorContains(t, err, "permission denied", "the computer's own error is shown")

	acts, err := f.svc.Activity(ctx, f.manager, ActivityQuery{Kinds: []string{ActivityStartupChanged}})
	require.NoError(t, err)
	require.Len(t, acts, 2)
	assert.Equal(t, "true", acts[0].Detail["on"])
}
