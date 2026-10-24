package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLISettings(t *testing.T) {
	url, key := server(t)
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		return p
	}

	out, _, err := run(t, url, key, "", "settings", "show", "rules")
	require.NoError(t, err)
	assert.Contains(t, out, "[built in]")
	assert.Contains(t, out, `"borderline_needs_action": true`)

	out, _, err = run(t, url, key, "", "settings", "show", "guidance", "--value")
	require.NoError(t, err)
	assert.Contains(t, out, `"name": "FSAI"`)

	_, _, err = run(t, url, key, "", "settings", "show", "colours")
	assert.ErrorContains(t, err, "unknown settings group")

	rules := write("rules.json", `{"borderline_needs_action": false, "listed_names_only": true}`)
	_, errOut, err := run(t, url, key, "", "settings", "set", "rules", "-f", rules, "--reason", "Plan allows it", "--yes")
	require.Error(t, err)
	assert.Contains(t, errOut, "borderline_needs_action: true → false")
	assert.Contains(t, errOut, "WARNING: less strict")
	assert.Contains(t, err.Error(), "plan section")

	out, _, err = run(t, url, key, "y\n", "settings", "set", "rules", "-f", rules, "--reason", "Plan allows it", "--plan-ref", "HACCP plan 5.2 (synthetic)")
	require.NoError(t, err)
	assert.Contains(t, out, "Saved rules version")

	_, _, err = run(t, url, key, "", "haccp", "log", "delivery", "--supplier", "Synthetic Dairy", "--product", "Cream", "--kind", "chilled", "--temp", "6.5")
	require.NoError(t, err, "borderline needs no action now")

	lists := write("lists.json", `{"units": [{"name": "Walk-in", "kind": "fridge"}], "food_categories": [], "cleaning_tasks": []}`)
	_, errOut, err = run(t, url, key, "n\n", "settings", "set", "lists", "-f", lists, "--reason", "Our fridges")
	require.NoError(t, err)
	assert.Contains(t, errOut, `units[Walk-in].name: added "Walk-in"`)
	assert.Contains(t, errOut, "Not saved.")
	_, _, err = run(t, url, key, "", "settings", "set", "lists", "-f", lists, "--reason", "Our fridges", "--yes")
	require.NoError(t, err)

	_, _, err = run(t, url, key, "", "haccp", "log", "fridge", "--unit", "Walkin", "--temp", "3")
	assert.ErrorContains(t, err, "not on the fridge list")

	out, _, err = run(t, url, key, "", "settings", "history", "lists")
	require.NoError(t, err)
	assert.Contains(t, out, "[built in]")
	assert.Contains(t, out, "Our fridges")

	sys := write("system.json", `{"backup_interval_minutes": 60, "backup_keep": 3, "max_upload_mb": 5, "preview_records": 20}`)
	_, _, err = run(t, url, key, "", "settings", "set", "system", "-f", sys, "--reason", "Hourly backups", "--yes")
	require.NoError(t, err)
	out, _, err = run(t, url, key, "", "settings", "show", "system", "--value")
	require.NoError(t, err)
	assert.Contains(t, out, `"backup_keep": 3`)
}
