package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite/sqlitedb"
)

func (s *Store) Activity() store.ActivityStore { return s }

func (s *Store) AppendActivity(ctx context.Context, a store.Activity) error {
	if a.Detail == nil {
		a.Detail = map[string]string{}
	}
	b, err := json.Marshal(a.Detail)
	if err != nil {
		return fmt.Errorf("encode activity %s: %w", a.ID, err)
	}
	detail := string(b)
	return mapErr(s.w.InsertActivity(ctx, sqlitedb.InsertActivityParams{
		ID: a.ID, At: us(a.At), Kind: a.Kind, UserID: a.UserID, UserName: a.UserName,
		DeviceID: a.DeviceID, DeviceName: a.DeviceName, Subject: a.Subject, Detail: detail,
	}))
}

// bounds turns optional times into the microsecond range a query needs.
func bounds(from, to time.Time) (int64, int64) {
	lo, hi := int64(math.MinInt64), int64(math.MaxInt64)
	if !from.IsZero() {
		lo = us(from)
	}
	if !to.IsZero() {
		hi = us(to)
	}
	return lo, hi
}

func maxRows(limit int) int64 {
	if limit > 0 {
		return int64(limit)
	}
	return -1 // SQLite: no limit
}

func (s *Store) ListActivity(ctx context.Context, f store.ActivityFilter) ([]store.Activity, error) {
	lo, hi := bounds(f.From, f.To)
	rows, err := s.r.ListActivity(ctx, sqlitedb.ListActivityParams{FromUs: lo, ToUs: hi, KindsCsv: strings.Join(f.Kinds, ","), MaxRows: maxRows(f.Limit)})
	if err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	out := make([]store.Activity, 0, len(rows))
	for _, r := range rows {
		a := store.Activity{
			ID: r.ID, At: fromUS(r.At), Kind: r.Kind, UserID: r.UserID, UserName: r.UserName,
			DeviceID: r.DeviceID, DeviceName: r.DeviceName, Subject: r.Subject, Detail: map[string]string{},
		}
		if err := json.Unmarshal([]byte(r.Detail), &a.Detail); err != nil {
			return nil, fmt.Errorf("decode activity %s: %w", r.ID, err)
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *Store) AppendPlanSnapshot(ctx context.Context, p store.PlanSnapshot) error {
	return mapErr(s.w.InsertPlanSnapshot(ctx, sqlitedb.InsertPlanSnapshotParams{
		ID: p.ID, At: us(p.At), Trigger: p.Trigger, Strategy: p.Strategy, SettingsVersion: p.SettingsVersion, Hash: p.Hash, Body: string(p.Body),
	}))
}

func snapshotFromRow(r sqlitedb.PlanSnapshot) store.PlanSnapshot {
	return store.PlanSnapshot{
		ID: r.ID, At: fromUS(r.At), Trigger: r.Trigger, Strategy: r.Strategy, SettingsVersion: r.SettingsVersion, Hash: r.Hash, Body: json.RawMessage(r.Body),
	}
}

func (s *Store) LastPlanSnapshot(ctx context.Context) (store.PlanSnapshot, error) {
	r, err := s.r.LastPlanSnapshot(ctx)
	if err != nil {
		return store.PlanSnapshot{}, mapErr(err)
	}
	return snapshotFromRow(r), nil
}

func (s *Store) ListPlanSnapshots(ctx context.Context, from, to time.Time, limit int) ([]store.PlanSnapshot, error) {
	lo, hi := bounds(from, to)
	rows, err := s.r.ListPlanSnapshots(ctx, sqlitedb.ListPlanSnapshotsParams{FromUs: lo, ToUs: hi, MaxRows: maxRows(limit)})
	if err != nil {
		return nil, fmt.Errorf("list plan snapshots: %w", err)
	}
	out := make([]store.PlanSnapshot, 0, len(rows))
	for _, r := range rows {
		out = append(out, snapshotFromRow(r))
	}
	return out, nil
}

func (s *Store) CountUserMentions(ctx context.Context, userID string) (store.UserMentions, error) {
	r, err := s.r.CountUserMentions(ctx, userID)
	if err != nil {
		return store.UserMentions{}, mapErr(err)
	}
	return store.UserMentions{OrderEvents: r.OrderEvents, VoiceLog: r.VoiceLog, Activity: r.Activity, HACCPRecords: r.HaccpRecords}, nil
}

func (s *Store) AnonymiseUser(ctx context.Context, userID, name string) (store.UserMentions, error) {
	var m store.UserMentions
	var err error
	if m.OrderEvents, err = s.w.AnonymiseOrderEvents(ctx, sqlitedb.AnonymiseOrderEventsParams{Name: name, UserID: userID}); err != nil {
		return m, mapErr(err)
	}
	if m.VoiceLog, err = s.w.AnonymiseVoiceLog(ctx, sqlitedb.AnonymiseVoiceLogParams{Name: name, UserID: userID}); err != nil {
		return m, mapErr(err)
	}
	if m.Activity, err = s.w.AnonymiseActivity(ctx, sqlitedb.AnonymiseActivityParams{Name: name, UserID: userID}); err != nil {
		return m, mapErr(err)
	}
	return m, nil
}
