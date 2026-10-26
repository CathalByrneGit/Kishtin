package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemValidate(t *testing.T) {
	ok := System{BackupIntervalMinutes: 360, BackupKeep: 28, MaxUploadMB: 15, PreviewRecords: 50}
	require.NoError(t, ok.Validate())
	off := ok
	off.BackupIntervalMinutes = 0
	require.NoError(t, off.Validate(), "0 turns backups off")

	bad := map[string]func(*System){
		"interval too short": func(s *System) { s.BackupIntervalMinutes = 14 },
		"negative keep":      func(s *System) { s.BackupKeep = -1 },
		"no uploads":         func(s *System) { s.MaxUploadMB = 0 },
		"huge uploads":       func(s *System) { s.MaxUploadMB = MaxUploadMBLimit + 1 },
		"no preview":         func(s *System) { s.PreviewRecords = 0 },
		"huge preview":       func(s *System) { s.PreviewRecords = PreviewRecordsLimit + 1 },
	}
	for name, mut := range bad {
		t.Run(name, func(t *testing.T) {
			s := ok
			mut(&s)
			assert.Error(t, s.Validate())
		})
	}
}

func TestChanges(t *testing.T) {
	type entry struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	type doc struct {
		Flag    bool     `json:"flag"`
		Entries []entry  `json:"entries"`
		Tags    []string `json:"tags"`
	}
	before := doc{Flag: true, Entries: []entry{{"Walk-in", "fridge"}, {"Chest", "freezer"}}, Tags: []string{"fish"}}
	after := doc{Flag: false, Entries: []entry{{"Chest", "freezer"}, {"Walk-in", "fridge"}, {"Bar", "fridge"}}, Tags: []string{"fish", "poultry"}}
	got, err := Changes(before, after)
	require.NoError(t, err)
	assert.Equal(t, []string{
		`entries[Bar].kind: added "fridge"`,
		`entries[Bar].name: added "Bar"`,
		`flag: true → false`,
		`tags[poultry]: added "poultry"`,
	}, got, "reordering is not a change; lists are compared by name")

	none, err := Changes(before, before)
	require.NoError(t, err)
	assert.Empty(t, none)

	type preset struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	presets, err := Changes([]preset{{"fridge", "Moved food"}}, []preset{{"freezer", "Moved food"}, {"fridge", "Moved food"}})
	require.NoError(t, err)
	assert.Equal(t, []string{`[freezer: Moved food].text: added "Moved food"`, `[freezer: Moved food].type: added "freezer"`}, presets)
	amp, err := Changes(map[string]string{"a": "x"}, map[string]string{"a": "Bins & bags"})
	require.NoError(t, err)
	assert.Equal(t, []string{`a: "x" → "Bins & bags"`}, amp, "no HTML escaping")

	removed, err := Changes(after, before)
	require.NoError(t, err)
	assert.Contains(t, removed, `entries[Bar].name: removed (was "Bar")`)
}
