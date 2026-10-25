package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/speech"
)

func TestVoiceCLI(t *testing.T) {
	url, key := server(t)
	cli := func(args ...string) string {
		t.Helper()
		out, _, err := run(t, url, key, "", args...)
		require.NoError(t, err, "%v", args)
		return out
	}
	out := cli("voice", "status")
	assert.Contains(t, out, "Speech-to-text: not set up (speech.stt_url in kishtin.toml)")

	out = cli("say", "walk-in", "fridge", "9", "degrees")
	assert.Contains(t, out, "Heard: walk-in fridge 9 degrees")
	assert.Contains(t, out, "Walk-in fridge 9 degrees: Fail.")
	out = cli("voice", "status")
	assert.Contains(t, out, "Waiting for your answer:")
	out = cli("say", "binned the milk and called the engineer")
	assert.Contains(t, out, "Say confirm to save")
	assert.Contains(t, out, "kishtin say confirm")
	out = cli("say", "confirm")
	assert.Contains(t, out, "Saved.")

	out = cli("voice", "log")
	assert.Contains(t, out, "binned the milk and called the engineer")
	assert.Contains(t, out, "asked")

	_, _, err := run(t, url, key, "", "say")
	assert.ErrorContains(t, err, "say what")
	out = cli("--json", "haccp", "list")
	assert.Contains(t, out, `"corrective_action": "Binned the milk and called the engineer"`)
}

func TestSayHandsFreeCLI(t *testing.T) {
	heard := "can you pass the tongs"
	stt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": heard})
	}))
	t.Cleanup(stt.Close)
	sp, err := speech.New(stt.URL, "", time.Second)
	require.NoError(t, err)
	url, key := serverWith(t, func(o *service.Options) { o.Speech = sp })
	f := filepath.Join(t.TempDir(), "phrase.wav")
	require.NoError(t, os.WriteFile(f, []byte("RIFF synthetic"), 0o600))

	out, _, err := run(t, url, key, "", "say", "--audio", f, "--hands-free")
	require.NoError(t, err)
	assert.Contains(t, out, "No wake word heard: ignored, and not logged.")

	heard = "Kishtin, walk-in fridge 3 degrees"
	out, _, err = run(t, url, key, "", "say", "--audio", f, "--hands-free")
	require.NoError(t, err)
	assert.Contains(t, out, "Walk-in fridge 3 degrees. Pass. Saved.")

	out, _, err = run(t, url, key, "", "voice", "log")
	require.NoError(t, err)
	assert.NotContains(t, out, "tongs", "ignored speech is not logged")
	out, _, err = run(t, url, key, "", "voice", "status")
	require.NoError(t, err)
	assert.Contains(t, out, "Speech-to-text: set up")
}
