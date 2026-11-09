package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/service"
)

func run(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := rootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

func TestRestore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KISHTIN_DATA_DIR", dir)
	t.Setenv("KISHTIN_CONFIG", "")
	t.Setenv("KISHTIN_TABLETS", "0")
	ctx := context.Background()
	cfg, err := loadConfig("")
	require.NoError(t, err)

	_, _, err = run(t, "", "restore")
	require.ErrorContains(t, err, "there are no backups")

	// A kitchen with one record, backed up, then a second record.
	a, err := open(ctx, cfg, false, nil)
	require.NoError(t, err)
	res, err := a.svc.SetupFromScreen(ctx, service.FirstRun{Restaurant: "The Synthetic Kitchen", Name: "Máire Manager", Password: "synthetic-password", Device: "Office"})
	require.NoError(t, err)
	mgr := &auth.Principal{User: res.User, Method: auth.MethodPassword}
	temp := 3.4
	log := func() {
		_, err := a.svc.LogRecord(ctx, mgr, service.NewRecord{Type: haccp.RecordFridge, Reading: haccp.Reading{Unit: "Walk-in fridge", TempC: &temp}})
		require.NoError(t, err)
	}
	log()
	b, err := a.svc.BackupNow(ctx, mgr)
	require.NoError(t, err)
	assert.Equal(t, service.BackupScheduled, b.Kind)
	log()

	// Refused while Kishtin holds the data folder.
	_, _, err = run(t, "", "restore", "--yes")
	require.ErrorContains(t, err, "stop it first")
	require.NoError(t, a.close())

	// The plan warns about what would be lost, and "no" changes nothing.
	_, errOut, err := run(t, "no\n", "restore")
	require.NoError(t, err)
	assert.Contains(t, errOut, "1 HACCP records")
	assert.Contains(t, errOut, "1 more records than the backup")
	assert.Contains(t, errOut, "Nothing changed.")
	var plan restorePlan
	out, _, err := run(t, "", "restore", "--json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(out), &plan))
	assert.False(t, plan.Restored)
	assert.True(t, plan.Checksum)
	require.NotNil(t, plan.CurrentRecords)
	assert.Equal(t, 2, *plan.CurrentRecords)

	// A damaged backup is refused.
	bad := filepath.Join(cfg.Backup.Dir, "kishtin-20200101T000000Z.db")
	require.NoError(t, os.WriteFile(bad, []byte("not a database"), 0o600))
	_, _, err = run(t, "", "restore", "--yes", filepath.Base(bad))
	require.ErrorContains(t, err, "is not a Kishtin backup")
	require.NoError(t, os.Remove(bad))
	require.NoError(t, os.WriteFile(b.Path+".sha256", []byte("0000  "+b.Name+"\n"), 0o600))
	_, _, err = run(t, "", "restore", "--yes")
	require.ErrorContains(t, err, "does not match its checksum")
	_, err = service.SealBackup(b.Path)
	require.NoError(t, err)

	out, _, err = run(t, "yes\n", "restore")
	require.NoError(t, err)
	assert.Contains(t, out, "Done: restored 1 HACCP records")

	a, err = open(ctx, cfg, false, nil)
	require.NoError(t, err)
	rep, err := a.svc.Verify(ctx, mgr)
	require.NoError(t, err)
	assert.True(t, rep.OK)
	assert.Equal(t, 1, rep.Chain.Checked, "back to the one record in the backup")
	list, err := a.svc.ListBackups(ctx, mgr)
	require.NoError(t, err)
	kinds := map[string]int{}
	for _, l := range list {
		kinds[l.Kind]++
	}
	assert.Equal(t, 1, kinds[service.BackupBeforeRestore], "the replaced database is kept")
	require.NoError(t, a.close())

	// And the replaced database can be put back.
	var kept string
	for _, l := range list {
		if l.Kind == service.BackupBeforeRestore {
			kept = l.Name
		}
	}
	out, _, err = run(t, "", "restore", "--yes", kept)
	require.NoError(t, err)
	assert.Contains(t, out, "Done: restored 2 HACCP records")
}
