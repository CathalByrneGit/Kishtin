package main

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKitchenCLI(t *testing.T) {
	url, key := server(t)
	cli := func(args ...string) string {
		t.Helper()
		out, _, err := run(t, url, key, "", args...)
		require.NoError(t, err, "%v", args)
		return out
	}

	cli("menu", "station", "add", "--name", "Grill", "--position", "1")
	cli("menu", "station", "add", "--name", "Cold section", "--position", "2")
	cli("menu", "equipment", "add", "--station", "grill", "--name", "Flat-top", "--slots", "3")
	cli("menu", "dish", "add", "--name", "Synthetic burger", "--station", "Grill", "--equipment", "flat-top",
		"--cook", "8m", "--rest", "2m", "--plate", "1m", "--hold", "3m", "--batchable", "--modifier", "Well done=2m")
	cli("menu", "dish", "add", "--name", "Synthetic salad", "--course", "starter", "--station", "Cold section", "--plate", "3m", "--hold", "5m")
	out := cli("menu", "dish", "update", "synthetic salad", "--hold", "4m")
	assert.Contains(t, out, "Saved Synthetic salad")

	out = cli("menu")
	assert.Contains(t, out, "equipment: Flat-top, 3 slots")
	assert.Contains(t, out, "Synthetic burger")
	assert.Contains(t, out, "Well done +120s")
	assert.Contains(t, out, "4m", "the update kept the other fields and changed hold")

	out = cli("order", "new", "--table", "4", "--covers", "2", "--item", "Synthetic salad", "--item", "2x synthetic burger: well done")
	assert.Contains(t, out, "Table 4  #1  open")
	assert.Contains(t, out, "Synthetic burger (Well done)")
	assert.Contains(t, out, "main: held")

	_, _, err := run(t, url, key, "", "order", "new", "--table", "5", "--item", "Soup")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no dish called "Soup"`)

	out = cli("order", "fire", "table 4", "main")
	assert.Contains(t, out, "main: fired, due")

	out = cli("pass", "--why")
	assert.Contains(t, out, "Table 4")
	assert.Contains(t, out, "on track")
	assert.Contains(t, out, "Held: Table 4 starter")

	out = cli("station", "grill")
	assert.Contains(t, out, "Start 2× Synthetic burger (Well done) on the flat-top now for Table 4")
	assert.Contains(t, out, "Why: Table 4 mains are due at")
	ids := regexp.MustCompile(`Items: (\S+) (\S+)`).FindStringSubmatch(out)
	require.Len(t, ids, 3)

	out = cli("items", "next", ids[1], ids[2])
	assert.Contains(t, out, "Table 4 Synthetic burger: cooking")
	out = cli("station", "grill")
	assert.Contains(t, out, "In progress:")

	out = cli("order", "urgent", "#1", "main")
	assert.Contains(t, out, "urgent")
	out = cli("menu", "86", "Synthetic burger")
	assert.Contains(t, out, "is 86")
	_, _, err = run(t, url, key, "", "order", "add", "4", "--item", "Synthetic burger")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sold out")

	_, _, err = run(t, url, key, "", "items", "move", "--to", "voided", ids[1])
	require.Error(t, err)
	assert.Contains(t, err.Error(), "say why")

	out = cli("order", "show", "4", "--history")
	assert.Contains(t, out, "fired → cooking")
	assert.Contains(t, out, "Máire Manager")

	raw := cli("--json", "pass")
	var k struct {
		Plan struct {
			Strategy string `json:"strategy"`
		} `json:"plan"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &k))
	assert.Equal(t, "backward", k.Plan.Strategy)

	out = cli("settings", "show", "scheduler")
	assert.Contains(t, out, "backward")
}
