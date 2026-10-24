package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/files"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite"
)

type fixture struct {
	svc     *Service
	clk     *clock.Fixed
	dir     string
	manager *auth.Principal
	cook    *auth.Principal
	device  string
}

var start = time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "kishtin.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	fs, err := files.New(filepath.Join(dir, "files"), 1<<20)
	require.NoError(t, err)
	clk := clock.NewFixed(start)
	svc := New(Options{
		Store: st, Clock: clk, Files: fs,
		Backups: BackupConfig{
			Dir: filepath.Join(dir, "backups"), Keep: 2,
			Open: func(ctx context.Context, path string) (store.Store, error) { return sqlite.OpenSnapshot(ctx, path) },
		},
	})
	require.NoError(t, svc.Init(ctx))
	require.NoError(t, svc.Init(ctx), "Init is safe to run twice")

	res, err := svc.Setup(ctx, NewUser{Name: "Máire Manager", Password: "synthetic-password", PIN: "9999"})
	require.NoError(t, err)
	mgr := &auth.Principal{User: res.User, Method: auth.MethodKey}

	cookUser, err := svc.CreateUser(ctx, mgr, NewUser{Name: "Liam Cook", Role: auth.RoleLineCook, PIN: "1234"})
	require.NoError(t, err)
	dev, err := svc.CreateKey(ctx, mgr, auth.KeyDevice, "Pass tablet", "")
	require.NoError(t, err)

	return &fixture{
		svc: svc, clk: clk, dir: dir, manager: mgr,
		cook:   &auth.Principal{User: cookUser, Method: auth.MethodPIN},
		device: dev.Secret,
	}
}

func fp(v float64) *float64 { return &v }

func fridge(temp float64) NewRecord {
	return NewRecord{Type: haccp.RecordFridge, Reading: haccp.Reading{Unit: "Walk-in", TempC: &temp}}
}

func TestSetupOnlyOnce(t *testing.T) {
	f := setup(t)
	_, err := f.svc.Setup(context.Background(), NewUser{Name: "Second", Password: "synthetic-password"})
	assert.Equal(t, KindConflict, KindOf(err))
}

func TestAuthenticate(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	_, err := f.svc.Authenticate(ctx, "", "")
	assert.Equal(t, KindUnauthenticated, KindOf(err))
	_, err = f.svc.Authenticate(ctx, "kishtin_key_wrong", "")
	assert.Equal(t, KindUnauthenticated, KindOf(err))

	c, err := f.svc.Authenticate(ctx, f.device, "")
	require.NoError(t, err)
	assert.Nil(t, c.Principal, "a device key alone is not a staff member")

	_, err = f.svc.LoginPIN(ctx, c, f.cook.User.ID, "0000")
	assert.Equal(t, KindUnauthenticated, KindOf(err))
	login, err := f.svc.LoginPIN(ctx, c, f.cook.User.ID, "1234")
	require.NoError(t, err)

	c, err = f.svc.Authenticate(ctx, f.device, login.Token)
	require.NoError(t, err)
	require.NotNil(t, c.Principal)
	assert.Equal(t, "Liam Cook", c.Principal.User.Name)
	assert.Equal(t, auth.MethodPIN, c.Principal.Method)

	f.clk.Advance(auth.PINSessionTTL)
	_, err = f.svc.Authenticate(ctx, f.device, login.Token)
	assert.Equal(t, KindUnauthenticated, KindOf(err), "session expired")

	userKey, err := f.svc.CreateKey(ctx, f.manager, auth.KeyUser, "CLI", "")
	require.NoError(t, err)
	uc, err := f.svc.Authenticate(ctx, userKey.Secret, "")
	require.NoError(t, err)
	assert.Equal(t, auth.MethodKey, uc.Principal.Method)
	_, err = f.svc.LoginPIN(ctx, uc, f.cook.User.ID, "1234")
	assert.Equal(t, KindForbidden, KindOf(err), "PIN login needs a device key")

	require.NoError(t, f.svc.RevokeKey(ctx, f.manager, userKey.Key.ID))
	_, err = f.svc.Authenticate(ctx, userKey.Secret, "")
	assert.Equal(t, KindUnauthenticated, KindOf(err), "revoked key")
}

func TestLoginLockout(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, err := f.svc.Authenticate(ctx, f.device, "")
	require.NoError(t, err)
	for range 5 {
		_, err := f.svc.LoginPIN(ctx, c, f.cook.User.ID, "0000")
		require.Error(t, err)
	}
	_, err = f.svc.LoginPIN(ctx, c, f.cook.User.ID, "1234")
	assert.Equal(t, KindLocked, KindOf(err), "the right PIN is refused while locked")
	f.clk.Advance(5 * time.Minute)
	_, err = f.svc.LoginPIN(ctx, c, f.cook.User.ID, "1234")
	assert.NoError(t, err)
}

func TestLogRecord(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	ok, err := f.svc.LogRecord(ctx, f.cook, fridge(3.4))
	require.NoError(t, err)
	assert.Equal(t, haccp.OutcomePass, ok.Outcome)
	assert.Equal(t, int64(1), ok.Seq)
	assert.Equal(t, haccp.FSAIDefaultID(haccp.CheckFridge), ok.LimitVersionID)
	assert.Equal(t, "Liam Cook", ok.LoggedByName)
	assert.Equal(t, start, ok.LoggedAt)

	_, err = f.svc.LogRecord(ctx, f.cook, fridge(7.2))
	require.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "This reading failed: 7.2°C is above the 5.0°C limit. Add a corrective action")

	bad := fridge(7.2)
	bad.CorrectiveAction = "Moved stock to the reach-in; engineer called"
	failed, err := f.svc.LogRecord(ctx, f.cook, bad)
	require.NoError(t, err)
	assert.Equal(t, haccp.OutcomeFail, failed.Outcome)
	assert.Equal(t, ok.Hash, failed.PrevHash)

	check, err := f.svc.CheckRecord(ctx, f.cook, fridge(6))
	require.NoError(t, err)
	assert.True(t, check.NeedsCorrectiveAction)
	assert.Equal(t, haccp.CheckFridge, check.Limit.Check)

	_, err = f.svc.LogRecord(ctx, nil, fridge(3))
	assert.Equal(t, KindUnauthenticated, KindOf(err))
}

func TestCorrections(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	orig, err := f.svc.LogRecord(ctx, f.cook, fridge(3.4))
	require.NoError(t, err)

	c := fridge(4.3)
	c.Corrects = orig.ID
	_, err = f.svc.LogRecord(ctx, f.cook, c)
	assert.Equal(t, KindInvalid, KindOf(err), "a correction needs a reason")

	c.CorrectionReason = "Misread the probe"
	fixed, err := f.svc.LogRecord(ctx, f.cook, c)
	require.NoError(t, err)
	assert.Equal(t, orig.ID, fixed.Corrects)

	wrongType := NewRecord{Type: haccp.RecordHotHold, Reading: haccp.Reading{Food: "Soup", TempC: fp(70)}, Corrects: orig.ID, CorrectionReason: "x"}
	_, err = f.svc.LogRecord(ctx, f.cook, wrongType)
	assert.Equal(t, KindInvalid, KindOf(err))

	missing := fridge(3)
	missing.Corrects, missing.CorrectionReason = "nope", "x"
	_, err = f.svc.LogRecord(ctx, f.cook, missing)
	assert.Equal(t, KindNotFound, KindOf(err))

	got, err := f.svc.GetRecord(ctx, f.cook, orig.ID)
	require.NoError(t, err)
	assert.Equal(t, orig, got, "the original is unchanged")
}

func TestLimitChanges(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	old, err := f.svc.LogRecord(ctx, f.cook, fridge(4.5))
	require.NoError(t, err)

	tighter := NewLimit{Check: haccp.CheckFridge, Scope: haccp.Scope{Kind: haccp.ScopeUnit, Ref: "Walk-in"}, MaxC: fp(4), Reason: "Walk-in holds fish"}

	pinManager := &auth.Principal{User: f.manager.User, Method: auth.MethodPIN}
	_, err = f.svc.CreateLimit(ctx, pinManager, tighter)
	var se *Error
	require.ErrorAs(t, err, &se)
	assert.True(t, se.NeedPassword)
	_, err = f.svc.CreateLimit(ctx, f.cook, tighter)
	assert.Equal(t, KindForbidden, KindOf(err))

	pv, err := f.svc.PreviewLimit(ctx, f.manager, tighter)
	require.NoError(t, err)
	assert.Empty(t, pv.Problem)
	assert.Nil(t, pv.Replaces)
	require.Len(t, pv.Recent, 1)
	assert.Equal(t, 1, pv.Changed)
	assert.Equal(t, haccp.OutcomeFail, pv.Recent[0].Proposed)

	v1, err := f.svc.CreateLimit(ctx, f.manager, tighter)
	require.NoError(t, err)
	assert.Equal(t, f.manager.User.ID, v1.CreatedBy)

	again, err := f.svc.GetRecord(ctx, f.cook, old.ID)
	require.NoError(t, err)
	assert.Equal(t, haccp.OutcomePass, again.Outcome, "no silent re-grading")

	_, err = f.svc.LogRecord(ctx, f.cook, fridge(4.5))
	assert.Equal(t, KindInvalid, KindOf(err), "new records use the new limit")
	rec := fridge(3.9)
	saved, err := f.svc.LogRecord(ctx, f.cook, rec)
	require.NoError(t, err)
	assert.Equal(t, v1.ID, saved.LimitVersionID)
	assert.Equal(t, haccp.ScopeUnit, saved.LimitScope.Kind)

	tighter.MaxC, tighter.Reason = fp(3), "Even colder"
	v2, err := f.svc.CreateLimit(ctx, f.manager, tighter)
	require.NoError(t, err)
	assert.Equal(t, v1.ID, v2.Supersedes)

	looser := NewLimit{Check: haccp.CheckHotHold, Scope: haccp.Scope{Kind: haccp.ScopeDefault}, MinC: fp(60), Reason: "Trying it"}
	pv, err = f.svc.PreviewLimit(ctx, f.manager, looser)
	require.NoError(t, err)
	assert.NotEmpty(t, pv.BelowFSAI)
	assert.Contains(t, pv.Problem, "below FSAI guidance")
	_, err = f.svc.CreateLimit(ctx, f.manager, looser)
	assert.Equal(t, KindInvalid, KindOf(err))
	looser.PlanRef = "HACCP plan 3.1 (synthetic)"
	hh, err := f.svc.CreateLimit(ctx, f.manager, looser)
	require.NoError(t, err)
	assert.Equal(t, haccp.FSAIDefaultID(haccp.CheckHotHold), hh.Supersedes)

	limits, err := f.svc.ListLimits(ctx, f.cook)
	require.NoError(t, err)
	assert.Len(t, limits.History, len(haccp.CheckTypes)+3)
	assert.Len(t, limits.Current, len(haccp.CheckTypes)+1)
}

func TestVerifyFindsTamperedFile(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0}
	m, err := f.svc.UploadFile(ctx, f.cook, bytes.NewReader(png), "../docket.png")
	require.NoError(t, err)
	assert.Equal(t, "docket.png", m.OriginalName)

	d := NewRecord{Type: haccp.RecordDelivery, Reading: haccp.Reading{
		Supplier: "Synthetic Meats Ltd", Product: "Chicken", DeliveryKind: haccp.DeliveryChilled, TempC: fp(3), DocketFile: m.SHA256,
	}}
	_, err = f.svc.LogRecord(ctx, f.cook, d)
	require.NoError(t, err)
	d.Reading.DocketFile = strings.Repeat("0", 64)
	_, err = f.svc.LogRecord(ctx, f.cook, d)
	assert.Equal(t, KindNotFound, KindOf(err), "a docket must be uploaded first")

	rep, err := f.svc.Verify(ctx, f.cook)
	require.NoError(t, err)
	assert.True(t, rep.OK)
	assert.Equal(t, 1, rep.FilesChecked)

	p := filepath.Join(f.dir, "files", m.SHA256[:2], m.SHA256)
	require.NoError(t, os.Chmod(p, 0o640))
	require.NoError(t, os.WriteFile(p, []byte("changed"), 0o640))
	rep, err = f.svc.Verify(ctx, f.cook)
	require.NoError(t, err)
	assert.False(t, rep.OK)
	require.Len(t, rep.FileProblems, 1)
}

func TestExportCSV(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for _, temp := range []float64{3.4, 2.1} {
		_, err := f.svc.LogRecord(ctx, f.cook, fridge(temp))
		require.NoError(t, err)
		f.clk.Advance(time.Hour)
	}
	inject := NewRecord{Type: haccp.RecordCleaning, Reading: haccp.Reading{Task: "=HYPERLINK(\"x\")", Done: new(bool)}, CorrectiveAction: "Done at close"}
	_, err := f.svc.LogRecord(ctx, f.cook, inject)
	require.NoError(t, err)

	var buf bytes.Buffer
	_, err = f.svc.ExportRecordsCSV(ctx, f.cook, start, start.Add(24*time.Hour), &buf)
	assert.Equal(t, KindForbidden, KindOf(err), "line cooks cannot export")

	sum, err := f.svc.ExportRecordsCSV(ctx, f.manager, start, start.Add(90*time.Minute), &buf)
	require.NoError(t, err)
	assert.Equal(t, 2, sum.Records)
	assert.Equal(t, 3, sum.Chain.Checked)

	buf.Reset()
	_, err = f.svc.ExportRecordsCSV(ctx, f.manager, start, start.Add(24*time.Hour), &buf)
	require.NoError(t, err)
	rows, err := csv.NewReader(&buf).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 4)
	assert.Equal(t, recordColumns, rows[0])
	assert.Equal(t, "3.4", rows[1][8])
	assert.Equal(t, "2026-05-01 10:00:00", rows[1][4], "Dublin time is UTC+1 in May")
	assert.Equal(t, "'=HYPERLINK(\"x\")", rows[3][7], "formula text is escaped")

	_, err = f.svc.ExportRecordsCSV(ctx, f.manager, start, start, &buf)
	assert.Equal(t, KindInvalid, KindOf(err))

	buf.Reset()
	require.NoError(t, f.svc.ExportLimitsCSV(ctx, f.manager, &buf))
	rows, err = csv.NewReader(&buf).ReadAll()
	require.NoError(t, err)
	assert.Len(t, rows, len(haccp.CheckTypes)+1)
}

func TestBackups(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.svc.LogRecord(ctx, f.cook, fridge(3.4))
	require.NoError(t, err)

	_, err = f.svc.BackupNow(ctx, f.cook)
	assert.Equal(t, KindForbidden, KindOf(err))

	var last Backup
	for range 3 {
		last, err = f.svc.BackupNow(ctx, f.manager)
		require.NoError(t, err)
		f.clk.Advance(time.Hour)
	}
	assert.True(t, last.Verified)
	assert.Equal(t, 1, last.Records)
	assert.Len(t, last.SHA256, 64)
	sum, err := os.ReadFile(last.Path + ".sha256")
	require.NoError(t, err)
	assert.Contains(t, string(sum), last.Name)

	list, err := f.svc.ListBackups(ctx, f.manager)
	require.NoError(t, err)
	require.Len(t, list, 2, "only the newest two are kept")
	assert.Equal(t, last.Name, list[0].Name)
	assert.Equal(t, last.SHA256, list[0].SHA256)
}

func TestUsers(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.svc.CreateUser(ctx, f.manager, NewUser{Name: "liam cook", Role: auth.RoleLineCook, PIN: "5555"})
	assert.Equal(t, KindConflict, KindOf(err))
	_, err = f.svc.CreateUser(ctx, f.manager, NewUser{Name: "New Manager", Role: auth.RoleManager, PIN: "5555"})
	assert.Equal(t, KindInvalid, KindOf(err), "a manager needs a password")
	_, err = f.svc.CreateUser(ctx, f.manager, NewUser{Name: "Bad PIN", Role: auth.RoleLineCook, PIN: "12"})
	assert.Equal(t, KindInvalid, KindOf(err))

	off := false
	_, err = f.svc.UpdateUser(ctx, f.manager, f.manager.User.ID, UserChange{Active: &off})
	assert.Equal(t, KindInvalid, KindOf(err), "a manager cannot lock themselves out")

	u, err := f.svc.UpdateUser(ctx, f.manager, f.cook.User.ID, UserChange{Active: &off})
	require.NoError(t, err)
	assert.False(t, u.Active)
	staff, err := f.svc.StaffList(ctx)
	require.NoError(t, err)
	require.Len(t, staff, 1, "deactivated staff are not on the login screen")

	_, err = f.svc.LogRecord(ctx, &auth.Principal{User: u, Method: auth.MethodPIN}, fridge(3))
	assert.Equal(t, KindForbidden, KindOf(err))
}
