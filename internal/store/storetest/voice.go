package storetest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
)

func testVoiceLog(t *testing.T, s store.Store) {
	ctx := context.Background()
	got, err := s.Voice().ListVoiceLog(ctx, time.Time{}, 10)
	require.NoError(t, err)
	assert.Empty(t, got)

	for i, say := range []string{"fire table 4 mains", "walk in fridge 3.4", "lovely weather"} {
		e := voice.LogEntry{
			ID: "v" + string(rune('1'+i)), At: t0.Add(time.Duration(i) * time.Minute), UserID: "u1", UserName: "Synthetic Chef",
			Source: voice.SourceSpoken, Transcript: say, Intent: voice.IntentFire, Outcome: voice.OutcomeDone, Reply: "Done.", Ref: "o-4",
		}
		if i == 2 {
			e.Intent, e.Outcome, e.Ref, e.Source = "", voice.OutcomeNotUnderstood, "", voice.SourceTyped
		}
		require.NoError(t, s.Voice().AppendVoiceLog(ctx, e))
	}
	assert.ErrorIs(t, s.Voice().AppendVoiceLog(ctx, voice.LogEntry{ID: "v1", At: t0, Source: voice.SourceTyped, Outcome: voice.OutcomeDone}), store.ErrConflict)

	got, err = s.Voice().ListVoiceLog(ctx, time.Time{}, 10)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "lovely weather", got[0].Transcript, "newest first")
	assert.Equal(t, voice.OutcomeNotUnderstood, got[0].Outcome)
	assert.Equal(t, voice.SourceTyped, got[0].Source)
	assert.Empty(t, got[0].Intent)
	last := got[2]
	assert.Equal(t, "v1", last.ID)
	assert.True(t, t0.Equal(last.At))
	assert.Equal(t, "Synthetic Chef", last.UserName)
	assert.Equal(t, voice.IntentFire, last.Intent)
	assert.Equal(t, "o-4", last.Ref)

	got, err = s.Voice().ListVoiceLog(ctx, t0.Add(2*time.Minute), 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "walk in fridge 3.4", got[0].Transcript, "before is exclusive, limit applies")

	typ := reflect.TypeOf(s.Voice())
	for _, name := range []string{"UpdateVoiceLog", "DeleteVoiceLog"} {
		_, has := typ.MethodByName(name)
		assert.False(t, has, "the voice store must not have %s", name)
	}
}
