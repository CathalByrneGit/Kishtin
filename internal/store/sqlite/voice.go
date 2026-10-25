package sqlite

import (
	"context"
	"math"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite/sqlitedb"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
)

func (s *Store) Voice() store.VoiceStore { return s }

func (s *Store) AppendVoiceLog(ctx context.Context, e voice.LogEntry) error {
	return mapErr(s.w.InsertVoiceLog(ctx, sqlitedb.InsertVoiceLogParams{
		ID: e.ID, At: us(e.At), UserID: e.UserID, UserName: e.UserName, Source: string(e.Source), Transcript: e.Transcript,
		Intent: string(e.Intent), Outcome: string(e.Outcome), Reply: e.Reply, Ref: e.Ref,
	}))
}

func (s *Store) ListVoiceLog(ctx context.Context, before time.Time, limit int) ([]voice.LogEntry, error) {
	at := int64(math.MaxInt64)
	if !before.IsZero() {
		at = us(before)
	}
	rows, err := s.r.ListVoiceLog(ctx, sqlitedb.ListVoiceLogParams{At: at, Limit: int64(limit)})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]voice.LogEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, voice.LogEntry{
			ID: r.ID, At: fromUS(r.At), UserID: r.UserID, UserName: r.UserName, Source: voice.Source(r.Source),
			Transcript: r.Transcript, Intent: voice.Intent(r.Intent), Outcome: voice.Outcome(r.Outcome), Reply: r.Reply, Ref: r.Ref,
		})
	}
	return out, nil
}
