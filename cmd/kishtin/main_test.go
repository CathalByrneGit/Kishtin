package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite"
	"github.com/cathalbyrnegit/kishtin/internal/web"
)

func server(t *testing.T) (url, key string) {
	t.Helper()
	return serverWith(t, nil)
}

// serverWith starts a server, letting the test change its options.
func serverWith(t *testing.T, opts func(*service.Options)) (url, key string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "kishtin.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	fs, err := files.New(filepath.Join(dir, "files"))
	require.NoError(t, err)
	o := service.Options{Store: st, Files: fs, Backups: service.BackupConfig{
		Dir:  filepath.Join(dir, "backups"),
		Open: func(ctx context.Context, p string) (store.Store, error) { return sqlite.OpenSnapshot(ctx, p) },
	}}
	if opts != nil {
		opts(&o)
	}
	svc := service.New(o)
	require.NoError(t, svc.Init(ctx))
	res, err := svc.Setup(ctx, service.NewUser{Name: "Máire Manager", Password: "synthetic-password"})
	require.NoError(t, err)
	h, err := web.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL, res.Secret
}

func run(t *testing.T, url, key, stdin string, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("KISHTIN_API_KEY", key)
	var out, errOut bytes.Buffer
	root := newRoot(&out, &errOut, strings.NewReader(stdin))
	root.SetArgs(append([]string{"--url", url}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestCLI(t *testing.T) {
	url, key := server(t)

	out, _, err := run(t, url, key, "", "whoami")
	require.NoError(t, err)
	assert.Contains(t, out, "Máire Manager (manager)")

	out, _, err = run(t, url, key, "", "haccp", "log", "fridge", "--unit", "Walk-in fridge", "--temp", "3.4")
	require.NoError(t, err)
	assert.Contains(t, out, "PASS  3.4°C is at or below the 5.0°C limit.")
	assert.Contains(t, out, "Saved record 1")

	out, _, err = run(t, url, key, "", "haccp", "log", "fridge", "--unit", "Walk-in fridge", "--temp", "7.1", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "FAIL")
	assert.Contains(t, out, "Not saved (dry run).")

	_, _, err = run(t, url, key, "", "haccp", "log", "fridge", "--unit", "Walk-in fridge", "--temp", "7.1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Add a corrective action to save it.")

	_, _, err = run(t, url, key, "", "haccp", "log", "fridge", "--unit", "Walk-in fridge", "--temp", "7.1", "--action", "Moved stock, called engineer")
	require.NoError(t, err)

	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0}
	docket := filepath.Join(t.TempDir(), "docket.png")
	require.NoError(t, os.WriteFile(docket, png, 0o600))
	_, errOut, err := run(t, url, key, "", "haccp", "log", "delivery", "--supplier", "Synthetic Meats Ltd", "--product", "Chicken",
		"--kind", "chilled", "--packaging-ok", "--in-date", "--temp", "6.5", "--action", "Accepted, used same day", "--docket", docket)
	require.NoError(t, err)
	assert.Contains(t, errOut, "Attached docket.png")

	_, _, err = run(t, url, key, "", "haccp", "log", "cleaning", "--task", "Floors swept & mopped", "--done")
	require.NoError(t, err)

	out, _, err = run(t, url, key, "", "haccp", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "Synthetic Meats Ltd: Chicken")
	assert.Contains(t, out, "BORDERLINE")

	out, _, err = run(t, url, key, "", "--json", "haccp", "list", "--type", "fridge", "--oldest")
	require.NoError(t, err)
	var list struct {
		Records []struct {
			ID  string `json:"id"`
			Seq int64  `json:"seq"`
		} `json:"records"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &list))
	require.Len(t, list.Records, 2)

	out, _, err = run(t, url, key, "", "haccp", "show", list.Records[0].ID)
	require.NoError(t, err)
	assert.Contains(t, out, "Walk-in fridge")
	assert.Contains(t, out, "Hash:")

	out, _, err = run(t, url, key, "", "haccp", "today")
	require.NoError(t, err)
	assert.Contains(t, out, "checks done")
	assert.Contains(t, out, "Walk-in fridge")
	assert.Regexp(t, `Floors swept & mopped +done .* by Máire Manager`, out)

	_, _, err = run(t, url, key, "", "haccp", "log", "cleaning", "--task", "Floors swept & mopped", "--done")
	assert.ErrorContains(t, err, "already done today")

	out, _, err = run(t, url, key, "", "haccp", "verify")
	require.NoError(t, err)
	assert.Contains(t, out, "Hash chain OK: 4 records")
	assert.Contains(t, out, "Files checked: 1")

	csvPath := filepath.Join(t.TempDir(), "export.csv")
	_, errOut, err = run(t, url, key, "", "haccp", "export", "--from", "2000-01-01", "--to", "2100-01-01", "-o", csvPath)
	require.NoError(t, err)
	assert.Contains(t, errOut, "4 records exported. Hash chain OK")
	data, err := os.ReadFile(csvPath)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "seq,record_id,"))

	pdfPath := filepath.Join(t.TempDir(), "pack.pdf")
	_, errOut, err = run(t, url, key, "", "haccp", "export", "--pdf", "-o", pdfPath)
	require.NoError(t, err)
	assert.Contains(t, errOut, "Inspection pack with")
	pdf, err := os.ReadFile(pdfPath)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(pdf), "%PDF-"))
	_, _, err = run(t, url, key, "", "haccp", "export", "--pdf")
	assert.ErrorContains(t, err, "needs -o")

	_, errOut, err = run(t, url, key, "n\n", "limits", "set", "--check", "fridge", "--scope", "unit:Walk-in fridge", "--max", "3", "--reason", "Fish in the walk-in")
	require.NoError(t, err)
	assert.Contains(t, errOut, "Would score differently: 1.")
	assert.Contains(t, errOut, "Not saved.")

	_, _, err = run(t, url, key, "", "limits", "set", "--check", "hot_hold", "--min", "60", "--reason", "Testing", "--yes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "below your reference guidance")

	out, _, err = run(t, url, key, "y\n", "limits", "set", "--check", "cook", "--scope", "category:poultry", "--combo", "75", "--combo", "70:2m", "--reason", "Time-temperature equivalent", "--plan-ref", "Plan 4.1 (synthetic)")
	require.NoError(t, err)
	assert.Contains(t, out, "Saved cook (category poultry)")

	out, _, err = run(t, url, key, "", "limits", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "core 70.0°C for 2m")

	out, _, err = run(t, url, key, "", "staff", "add", "--name", "Liam Cook", "--pin", "1234")
	require.NoError(t, err)
	assert.Contains(t, out, "Added Liam Cook (line_cook)")

	out, _, err = run(t, url, key, "", "keys", "create", "--name", "Pass tablet")
	require.NoError(t, err)
	assert.Contains(t, out, "kishtin_key_")
	assert.Contains(t, out, url+"/#pair=kishtin_key_")

	out, _, err = run(t, url, key, "", "backup", "now")
	require.NoError(t, err)
	assert.Contains(t, out, "Verified: 4 HACCP records")

	_, _, err = run(t, url, "kishtin_key_wrong", "", "whoami")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
}
