package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/storetest"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
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
	require.NoError(t, s.CreateOrder(ctx, orders.Order{ID: "o1", Number: 1, Kind: orders.KindTakeaway, Status: orders.StatusOpen, CreatedBy: "x"}))
	require.NoError(t, s.AppendEvent(ctx, orders.Event{ID: "e1", OrderID: "o1", Kind: orders.EventCreated, By: "x", ByName: "x"}))
	require.NoError(t, s.AppendVoiceLog(ctx, voice.LogEntry{ID: "v1", UserID: "x", UserName: "x", Source: voice.SourceTyped, Transcript: "x", Outcome: voice.OutcomeDone, Reply: "x"}))
	require.NoError(t, s.AppendActivity(ctx, store.Activity{ID: "a1", Kind: "login", UserID: "x", UserName: "x"}))
	require.NoError(t, s.AppendPlanSnapshot(ctx, store.PlanSnapshot{ID: "p1", Trigger: "timer", Body: []byte("{}")}))
	// A leaver's name is the one thing that can change.
	for _, q := range []string{
		"UPDATE voice_log SET user_name = 'Former staff 1'",
		"UPDATE order_events SET by_name = 'Former staff 1'",
		"UPDATE activity SET user_name = 'Former staff 1'",
	} {
		_, err := s.writer.ExecContext(ctx, q)
		require.NoError(t, err, q)
	}
	for _, q := range []string{
		"UPDATE voice_log SET user_name = 'y', transcript = 'edited'",
		"UPDATE order_events SET by_name = 'y', by_user = 'y'",
		"UPDATE activity SET detail = '{\"edited\":true}'",
		"DELETE FROM activity",
		"UPDATE plan_snapshots SET body = '{}'",
		"DELETE FROM plan_snapshots",
		"UPDATE voice_log SET transcript = 'edited'",
		"DELETE FROM voice_log",
		"UPDATE order_events SET detail = 'edited'",
		"DELETE FROM order_events",
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
