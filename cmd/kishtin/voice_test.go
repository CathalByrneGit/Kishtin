package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
