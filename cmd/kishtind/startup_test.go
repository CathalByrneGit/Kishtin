package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvWarning(t *testing.T) {
	assert.Empty(t, envWarning([]string{"HOME=/home/synth", "KISHTIN_CONFIG=/srv/kishtin.toml", "KISHTIN_API_KEY=k"}), "passed on, or not for the server")
	w := envWarning([]string{"KISHTIN_SQUARE_TOKEN=x", "KISHTIN_DATA_DIR=/srv/kishtin"})
	assert.Contains(t, w, "KISHTIN_DATA_DIR, KISHTIN_SQUARE_TOKEN.")
	assert.Contains(t, w, "kishtin.toml")
}
