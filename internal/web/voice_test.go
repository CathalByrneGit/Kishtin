package web

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/service"
)

func TestVoiceOverHTTP(t *testing.T) {
	e := newEnv(t)
	key := e.mgrKey

	r := e.do(t, http.MethodGet, "/api/voice", key, "", nil)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	st := decode[service.VoiceStatus](t, r)
	assert.False(t, st.Listen)
	assert.False(t, st.Speak)

	r = e.do(t, http.MethodPost, "/api/voice/command", key, "", map[string]any{"text": "walk-in fridge 3 degrees"})
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	rep := decode[service.VoiceReply](t, r)
	assert.Equal(t, "Walk-in fridge 3 degrees. Pass. Saved.", rep.Say)
	assert.Equal(t, "walk-in fridge 3 degrees", rep.Heard)

	r = e.do(t, http.MethodPost, "/api/voice/command", key, "", map[string]any{"text": ""})
	assert.Equal(t, http.StatusUnprocessableEntity, r.status)

	// Without a speech-to-text sidecar, audio is refused with what to do.
	for _, ct := range []string{"audio/webm;codecs=opus", "audio/ogg", "audio/wav"} {
		req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/api/voice/audio", bytes.NewReader([]byte("synthetic audio")))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", ct)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		assert.Equal(t, http.StatusConflict, res.StatusCode, ct)
		assert.Contains(t, string(b), "Speech-to-text is not set up", ct)
	}

	r = e.do(t, http.MethodPost, "/api/voice/speak", key, "", map[string]any{"text": "Hello"})
	assert.Equal(t, http.StatusConflict, r.status)

	r = e.do(t, http.MethodGet, "/api/voice/log?limit=5", key, "", nil)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	log := decode[voiceLogBody](t, r)
	require.Len(t, log.Entries, 1)
	assert.Equal(t, "walk-in fridge 3 degrees", log.Entries[0].Transcript)

	r = e.do(t, http.MethodGet, "/api/voice", "", "", nil)
	assert.Equal(t, http.StatusUnauthorized, r.status)
}
