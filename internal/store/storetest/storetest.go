// Package storetest is the conformance suite every storage backend must
// pass. Backends call Run from their own tests.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// Factory returns a new, empty store. It registers its own cleanup.
type Factory func(t *testing.T) store.Store

// Run runs the whole suite against a backend.
func Run(t *testing.T, newStore Factory) {
	tests := map[string]func(*testing.T, store.Store){
		"RecordRoundTrip":         testRecordRoundTrip,
		"RecordSeqAndHashUnique":  testRecordSeqAndHashUnique,
		"ListRecordsFilters":      testListRecordsFilters,
		"EachRecordPagesInOrder":  testEachRecordPagesInOrder,
		"ChainVerifiesAfterStore": testChainVerifiesAfterStore,
		"TxRollback":              testTxRollback,
		"ConcurrentAppendsChain":  testConcurrentAppendsChain,
		"LimitVersions":           testLimitVersions,
		"Users":                   testUsers,
		"SessionsAndKeys":         testSessionsAndKeys,
		"Files":                   testFiles,
		"NoMutationMethods":       testNoMutationMethods,
		"Settings":                testSettings,
		"VoiceLog":                testVoiceLog,
		"POS":                     testPOS,
	}
	for name, fn := range kitchenTests() {
		tests[name] = fn
	}
	for name, fn := range tests {
		t.Run(name, func(t *testing.T) { fn(t, newStore(t)) })
	}
}

// Backup runs the backup check for backends that implement store.Backuper.
// reopen opens the backup file as a store.
func Backup(t *testing.T, newStore Factory, reopen func(t *testing.T, path string) store.Store) {
	s := newStore(t)
	b, ok := s.(store.Backuper)
	require.True(t, ok, "backend does not implement store.Backuper")
	ctx := context.Background()
	recs := appendChain(t, s, 3)

	dest := filepath.Join(t.TempDir(), "backup.db")
	require.NoError(t, b.Backup(ctx, dest))
	assert.Error(t, b.Backup(ctx, dest), "backup refuses to overwrite")

	copyStore := reopen(t, dest)
	last, err := copyStore.HACCP().LastRecord(ctx)
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.Equal(t, recs[len(recs)-1].Hash, last.Hash)
}

var t0 = time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)

func fp(v float64) *float64 { return &v }

func sampleRecord(k int) haccp.Record {
	cooked := t0.Add(time.Duration(k) * time.Hour)
	return haccp.Record{
		ID:           fmt.Sprintf("rec-%03d", k),
		Type:         haccp.RecordCooling,
		LoggedBy:     "user-1",
		LoggedByName: "Synthetic Chef",
		LoggedAt:     t0.Add(time.Duration(k) * time.Minute).Add(123 * time.Microsecond),
		Reading: haccp.Reading{
			Food: "Beef stew", CookedAt: &cooked, TempAtCutoffC: fp(9.5), Notes: "Synthetic",
		},
		Outcome:          haccp.OutcomeBorderline,
		OutcomeReason:    "late but cool",
		LimitVersionID:   haccp.FSAIDefaultID(haccp.CheckCooling),
		LimitScope:       &haccp.Scope{Kind: haccp.ScopeDefault},
		RulesVersionID:   haccp.BuiltinRulesID,
		CorrectiveAction: "Moved to blast chiller",
	}
}

// appendChain appends n sealed records the way the service does.
func appendChain(t *testing.T, s store.Store, n int) []haccp.Record {
	t.Helper()
	ctx := context.Background()
	var out []haccp.Record
	for k := range n {
		var sealed haccp.Record
		err := s.WithTx(ctx, func(tx store.Store) error {
			prev, err := tx.HACCP().LastRecord(ctx)
			if err != nil {
				return err
			}
			sealed = haccp.Seal(sampleRecord(k), prev)
			return tx.HACCP().AppendRecord(ctx, sealed)
		})
		require.NoError(t, err)
		out = append(out, sealed)
	}
	return out
}

func testRecordRoundTrip(t *testing.T, s store.Store) {
	ctx := context.Background()
	last, err := s.HACCP().LastRecord(ctx)
	require.NoError(t, err)
	assert.Nil(t, last)

	recs := appendChain(t, s, 2)
	got, err := s.HACCP().GetRecord(ctx, recs[1].ID)
	require.NoError(t, err)
	assert.Equal(t, recs[1], got)
	assert.Equal(t, time.UTC, got.LoggedAt.Location())
	assert.Equal(t, recs[1].Hash, haccp.ComputeHash(got), "the record hashes the same after a round trip")

	last, err = s.HACCP().LastRecord(ctx)
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.Equal(t, recs[1].ID, last.ID)

	_, err = s.HACCP().GetRecord(ctx, "missing")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func testRecordSeqAndHashUnique(t *testing.T, s store.Store) {
	ctx := context.Background()
	recs := appendChain(t, s, 1)

	dupSeq := haccp.Seal(sampleRecord(5), nil) // seq 1 again
	assert.ErrorIs(t, s.HACCP().AppendRecord(ctx, dupSeq), store.ErrConflict)

	dupID := haccp.Seal(sampleRecord(0), &recs[0])
	assert.ErrorIs(t, s.HACCP().AppendRecord(ctx, dupID), store.ErrConflict)
}

func testListRecordsFilters(t *testing.T, s store.Store) {
	ctx := context.Background()
	appendChain(t, s, 5)
	fridge := sampleRecord(9)
	fridge.Type = haccp.RecordFridge
	fridge.Reading = haccp.Reading{Unit: "Walk-in", TempC: fp(3)}
	require.NoError(t, s.WithTx(ctx, func(tx store.Store) error {
		prev, err := tx.HACCP().LastRecord(ctx)
		if err != nil {
			return err
		}
		return tx.HACCP().AppendRecord(ctx, haccp.Seal(fridge, prev))
	}))

	all, err := s.HACCP().ListRecords(ctx, store.RecordFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 6)
	assert.Equal(t, "rec-000", all[0].ID)

	window, err := s.HACCP().ListRecords(ctx, store.RecordFilter{
		From: t0.Add(1 * time.Minute), To: t0.Add(3 * time.Minute),
	})
	require.NoError(t, err)
	require.Len(t, window, 2, "From is inclusive at its minute, To is exclusive")
	assert.Equal(t, "rec-001", window[0].ID)
	assert.Equal(t, "rec-002", window[1].ID)

	byType, err := s.HACCP().ListRecords(ctx, store.RecordFilter{Types: []haccp.RecordType{haccp.RecordFridge}})
	require.NoError(t, err)
	require.Len(t, byType, 1)
	assert.Equal(t, "rec-009", byType[0].ID)

	fails, err := s.HACCP().ListRecords(ctx, store.RecordFilter{Outcomes: []haccp.Outcome{haccp.OutcomeBorderline, haccp.OutcomeFail}})
	require.NoError(t, err)
	assert.Len(t, fails, 6, "every sample record is borderline")
	passes, err := s.HACCP().ListRecords(ctx, store.RecordFilter{Outcomes: []haccp.Outcome{haccp.OutcomePass}})
	require.NoError(t, err)
	assert.Empty(t, passes)

	newest, err := s.HACCP().ListRecords(ctx, store.RecordFilter{Newest: true, Limit: 2})
	require.NoError(t, err)
	require.Len(t, newest, 2)
	assert.Equal(t, "rec-009", newest[0].ID)
	assert.Equal(t, "rec-004", newest[1].ID)
}

func testEachRecordPagesInOrder(t *testing.T, s store.Store) {
	ctx := context.Background()
	appendChain(t, s, 7)
	var seqs []int64
	require.NoError(t, s.HACCP().EachRecord(ctx, func(r haccp.Record) error {
		seqs = append(seqs, r.Seq)
		return nil
	}))
	assert.Equal(t, []int64{1, 2, 3, 4, 5, 6, 7}, seqs)

	stop := errors.New("stop")
	n := 0
	err := s.HACCP().EachRecord(ctx, func(haccp.Record) error {
		n++
		if n == 3 {
			return stop
		}
		return nil
	})
	assert.ErrorIs(t, err, stop)
	assert.Equal(t, 3, n)
}

func testChainVerifiesAfterStore(t *testing.T, s store.Store) {
	ctx := context.Background()
	appendChain(t, s, 4)
	var v haccp.ChainVerifier
	require.NoError(t, s.HACCP().EachRecord(ctx, func(r haccp.Record) error {
		v.Add(r)
		return nil
	}))
	rep := v.Report()
	assert.True(t, rep.OK, "%+v", rep.Break)
	assert.Equal(t, 4, rep.Checked)
}

func testTxRollback(t *testing.T, s store.Store) {
	ctx := context.Background()
	boom := errors.New("boom")
	err := s.WithTx(ctx, func(tx store.Store) error {
		if err := tx.HACCP().AppendRecord(ctx, haccp.Seal(sampleRecord(0), nil)); err != nil {
			return err
		}
		inside, err := tx.HACCP().LastRecord(ctx)
		if err != nil {
			return err
		}
		if inside == nil {
			return errors.New("transaction cannot read its own write")
		}
		return boom
	})
	assert.ErrorIs(t, err, boom)
	last, err := s.HACCP().LastRecord(ctx)
	require.NoError(t, err)
	assert.Nil(t, last, "rolled-back record is gone")
}

func testConcurrentAppendsChain(t *testing.T, s store.Store) {
	ctx := context.Background()
	const workers, each = 4, 5
	var wg sync.WaitGroup
	errs := make(chan error, workers*each)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := range each {
				err := s.WithTx(ctx, func(tx store.Store) error {
					prev, err := tx.HACCP().LastRecord(ctx)
					if err != nil {
						return err
					}
					r := sampleRecord(w*each + k)
					return tx.HACCP().AppendRecord(ctx, haccp.Seal(r, prev))
				})
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var v haccp.ChainVerifier
	require.NoError(t, s.HACCP().EachRecord(ctx, func(r haccp.Record) error {
		v.Add(r)
		return nil
	}))
	rep := v.Report()
	assert.True(t, rep.OK, "%+v", rep.Break)
	assert.Equal(t, workers*each, rep.Checked)
}

func testLimitVersions(t *testing.T, s store.Store) {
	ctx := context.Background()
	for _, lv := range haccp.FSAIDefaults() {
		require.NoError(t, s.HACCP().AppendLimitVersion(ctx, lv))
	}
	assert.ErrorIs(t, s.HACCP().AppendLimitVersion(ctx, haccp.FSAIDefault(haccp.CheckFridge)), store.ErrConflict)

	v2 := haccp.FSAIDefault(haccp.CheckFridge)
	v2.ID, v2.Supersedes, v2.MaxC = "fridge-v2", v2.ID, fp(4)
	v2.CreatedBy, v2.CreatedAt, v2.Reason = "user-1", t0.Add(time.Hour), "Tighter"
	require.NoError(t, s.HACCP().AppendLimitVersion(ctx, v2))

	fork := v2
	fork.ID = "fridge-v2b"
	assert.ErrorIs(t, s.HACCP().AppendLimitVersion(ctx, fork), store.ErrConflict, "a version is superseded only once")

	got, err := s.HACCP().GetLimitVersion(ctx, "fridge-v2")
	require.NoError(t, err)
	assert.Equal(t, v2, got)

	all, err := s.HACCP().ListLimitVersions(ctx)
	require.NoError(t, err)
	assert.Len(t, all, len(haccp.CheckTypes)+1)
	assert.Equal(t, "fridge-v2", haccp.Resolve(all, haccp.CheckFridge, nil).ID)

	_, err = s.HACCP().GetLimitVersion(ctx, "missing")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func testUsers(t *testing.T, s store.Store) {
	ctx := context.Background()
	u := auth.User{
		ID: "u1", Name: "Siobhán (synthetic)", Role: auth.RoleLineCook, Active: true,
		PINHash: "pinhash", CreatedAt: t0, UpdatedAt: t0,
	}
	require.NoError(t, s.Users().CreateUser(ctx, u))
	dup := u
	dup.ID = "u2"
	dup.Name = "siobhán (SYNTHETIC)"
	assert.ErrorIs(t, s.Users().CreateUser(ctx, dup), store.ErrConflict, "names are unique, ignoring case")

	got, err := s.Users().GetUser(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, u, got)

	u.Role, u.Active, u.PasswordHash, u.UpdatedAt = auth.RoleManager, false, "pw", t0.Add(time.Hour)
	require.NoError(t, s.Users().UpdateUser(ctx, u))
	got, err = s.Users().GetUser(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, u, got)

	assert.ErrorIs(t, s.Users().UpdateUser(ctx, auth.User{ID: "missing", Name: "x"}), store.ErrNotFound)
	_, err = s.Users().GetUser(ctx, "missing")
	assert.ErrorIs(t, err, store.ErrNotFound)

	require.NoError(t, s.Users().CreateUser(ctx, auth.User{ID: "u0", Name: "Aoife", Role: auth.RoleHeadChef, CreatedAt: t0, UpdatedAt: t0}))
	list, err := s.Users().ListUsers(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "Aoife", list[0].Name)
}

func testSessionsAndKeys(t *testing.T, s store.Store) {
	ctx := context.Background()
	require.NoError(t, s.Users().CreateUser(ctx, auth.User{ID: "u1", Name: "Synthetic", Role: auth.RoleManager, Active: true, CreatedAt: t0, UpdatedAt: t0}))

	se := auth.Session{TokenHash: "h1", UserID: "u1", Method: auth.MethodPIN, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)}
	require.NoError(t, s.Auth().CreateSession(ctx, se))
	got, err := s.Auth().GetSession(ctx, "h1")
	require.NoError(t, err)
	assert.Equal(t, se, got)

	old := auth.Session{TokenHash: "h0", UserID: "u1", Method: auth.MethodPIN, CreatedAt: t0, ExpiresAt: t0.Add(time.Minute)}
	require.NoError(t, s.Auth().CreateSession(ctx, old))
	n, err := s.Auth().DeleteExpiredSessions(ctx, t0.Add(30*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	_, err = s.Auth().GetSession(ctx, "h0")
	assert.ErrorIs(t, err, store.ErrNotFound)

	require.NoError(t, s.Auth().DeleteSession(ctx, "h1"))
	_, err = s.Auth().GetSession(ctx, "h1")
	assert.ErrorIs(t, err, store.ErrNotFound)

	k := auth.APIKey{ID: "k1", Kind: auth.KeyUser, UserID: "u1", Name: "CLI", TokenHash: "kh1", CreatedBy: "u1", CreatedAt: t0}
	require.NoError(t, s.Auth().CreateAPIKey(ctx, k))
	dev := auth.APIKey{ID: "k2", Kind: auth.KeyDevice, Name: "Pass tablet", TokenHash: "kh2", CreatedBy: "u1", CreatedAt: t0.Add(time.Second)}
	require.NoError(t, s.Auth().CreateAPIKey(ctx, dev))
	gotKey, err := s.Auth().GetAPIKeyByHash(ctx, "kh1")
	require.NoError(t, err)
	assert.Equal(t, k, gotKey)

	require.NoError(t, s.Auth().RevokeAPIKey(ctx, "k1", t0.Add(time.Hour)))
	assert.ErrorIs(t, s.Auth().RevokeAPIKey(ctx, "k1", t0.Add(2*time.Hour)), store.ErrNotFound, "already revoked")
	gotKey, err = s.Auth().GetAPIKeyByHash(ctx, "kh1")
	require.NoError(t, err)
	require.NotNil(t, gotKey.RevokedAt)
	assert.Equal(t, t0.Add(time.Hour), *gotKey.RevokedAt)

	keys, err := s.Auth().ListAPIKeys(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	assert.Empty(t, keys[1].UserID)
}

func testFiles(t *testing.T, s store.Store) {
	ctx := context.Background()
	m := store.FileMeta{SHA256: "abc", MIME: "image/jpeg", Size: 10, OriginalName: "docket.jpg", UploadedBy: "u1", UploadedAt: t0}
	require.NoError(t, s.Files().PutFile(ctx, m))
	again := m
	again.OriginalName, again.UploadedAt = "other.jpg", t0.Add(time.Hour)
	require.NoError(t, s.Files().PutFile(ctx, again), "the same file twice is fine")
	got, err := s.Files().GetFile(ctx, "abc")
	require.NoError(t, err)
	assert.Equal(t, m, got, "the first upload's metadata is kept")
	list, err := s.Files().ListFiles(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	_, err = s.Files().GetFile(ctx, "missing")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// testNoMutationMethods guards the append-only rule: a backend must not
// expose a way to change or remove HACCP records or limit versions.
func testNoMutationMethods(t *testing.T, s store.Store) {
	h := s.HACCP()
	for _, v := range []any{h, s.Files(), s.Settings()} {
		typ := reflect.TypeOf(v)
		for _, name := range []string{"UpdateRecord", "DeleteRecord", "UpdateLimitVersion", "DeleteLimitVersion", "DeleteFile", "UpdateSettings", "DeleteSettings"} {
			_, has := typ.MethodByName(name)
			assert.False(t, has, "the store must not have %s", name)
		}
	}
}

func testSettings(t *testing.T, s store.Store) {
	ctx := context.Background()
	v1 := store.SettingsVersion{
		ID: "rules-1", Kind: settings.KindRules, Body: []byte(`{"borderline_needs_action":true}`),
		CreatedBy: "system", CreatedAt: t0, Reason: "Built in",
	}
	require.NoError(t, s.Settings().AppendSettings(ctx, v1))
	assert.ErrorIs(t, s.Settings().AppendSettings(ctx, v1), store.ErrConflict)

	v2 := v1
	v2.ID, v2.Supersedes, v2.Body = "rules-2", "rules-1", []byte(`{"borderline_needs_action":false}`)
	v2.CreatedBy, v2.CreatedAt, v2.Reason, v2.PlanRef = "u1", t0.Add(time.Hour), "Plan says so", "HACCP plan 2.3"
	require.NoError(t, s.Settings().AppendSettings(ctx, v2))
	fork := v2
	fork.ID = "rules-2b"
	assert.ErrorIs(t, s.Settings().AppendSettings(ctx, fork), store.ErrConflict, "a version is superseded only once")

	other := store.SettingsVersion{ID: "sys-1", Kind: settings.KindSystem, Body: []byte(`{}`), CreatedBy: "u1", CreatedAt: t0, Reason: "x"}
	require.NoError(t, s.Settings().AppendSettings(ctx, other))

	got, err := s.Settings().GetSettings(ctx, "rules-2")
	require.NoError(t, err)
	assert.Equal(t, v2, got)

	list, err := s.Settings().ListSettings(ctx, settings.KindRules)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "rules-1", list[0].ID)
	assert.JSONEq(t, `{"borderline_needs_action":false}`, string(list[1].Body))

	_, err = s.Settings().GetSettings(ctx, "missing")
	assert.ErrorIs(t, err, store.ErrNotFound)
}
