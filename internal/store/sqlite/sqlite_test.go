package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/storetest"
)

func open(t *testing.T, path string) store.Store {
	t.Helper()
	s, err := Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func newStore(t *testing.T) store.Store {
	return open(t, filepath.Join(t.TempDir(), "kishtin.db"))
}

func TestConformance(t *testing.T) { storetest.Run(t, newStore) }

func TestBackup(t *testing.T) {
	storetest.Backup(t, newStore, func(t *testing.T, path string) store.Store {
		s, err := OpenSnapshot(context.Background(), path)
		require.NoError(t, err)
		t.Cleanup(func() { s.Close() })
		return s
	})
}

func TestSnapshotIsReadOnly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "kishtin.db")
	s, err := Open(ctx, path)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	snap, err := OpenSnapshot(ctx, path)
	require.NoError(t, err)
	defer snap.Close()
	require.Error(t, snap.AppendLimitVersion(ctx, haccp.FSAIDefault(haccp.CheckFridge)))
	_, err = OpenSnapshot(ctx, filepath.Join(t.TempDir(), "missing.db"))
	require.Error(t, err)
}

func TestTriggersBlockEdits(t *testing.T) {
	ctx := context.Background()
	s := newStore(t).(*Store)
	r := haccp.Seal(haccp.Record{ID: "r1", Type: haccp.RecordCleaning, Outcome: haccp.OutcomePass}, nil)
	require.NoError(t, s.AppendRecord(ctx, r))
	require.NoError(t, s.AppendSettings(ctx, store.SettingsVersion{ID: "s1", Kind: "rules", Body: []byte("{}"), CreatedBy: "x", Reason: "x"}))
	for _, q := range []string{
		"UPDATE haccp_records SET outcome = 'fail'",
		"DELETE FROM haccp_records",
		"UPDATE settings_versions SET body = '{}'",
		"DELETE FROM settings_versions",
	} {
		_, err := s.writer.ExecContext(ctx, q)
		require.ErrorContains(t, err, "append-only", q)
	}
}

func TestReopenKeepsData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "kishtin.db")
	s, err := Open(ctx, path)
	require.NoError(t, err)
	require.NoError(t, s.AppendLimitVersion(ctx, haccp.FSAIDefault(haccp.CheckFridge)))
	require.NoError(t, s.Close())

	s2 := open(t, path)
	lv, err := s2.HACCP().GetLimitVersion(ctx, haccp.FSAIDefaultID(haccp.CheckFridge))
	require.NoError(t, err)
	require.Equal(t, haccp.CheckFridge, lv.Check)
}
