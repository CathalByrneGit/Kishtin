package voice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripWake(t *testing.T) {
	for in, want := range map[string]string{
		"Kishtin, fire table 4 mains":       "fire table 4 mains",
		"Kish tin fire table 4":             "fire table 4",
		"kishteen walk in fridge 3":         "walk in fridge 3",
		"Hey Kishtin, what's next on grill": "whats next on grill",
		"Kish-tin.":                         "",
		"KISHTIN confirm void":              "confirm void",
	} {
		got, ok := StripWake(in, DefaultWakeWords)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{
		"fire table 4 mains", "kitchen is busy tonight", "Christine, can you grab the plates", "confirm", "", "the kish is ready",
	} {
		_, ok := StripWake(in, DefaultWakeWords)
		assert.False(t, ok, in)
	}
	got, ok := StripWake("chef 86 salmon", []string{"chef"})
	assert.True(t, ok)
	assert.Equal(t, "86 salmon", got)
	_, ok = StripWake("cheese on", []string{"chef"})
	assert.False(t, ok, "short wake words must match exactly")
}
