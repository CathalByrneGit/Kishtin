package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite/sqlitedb"
)

func (s *Store) POS() store.POSStore { return s }

func (s *Store) SaveMapping(ctx context.Context, m integrations.Mapping) error {
	return mapErr(s.w.UpsertMapping(ctx, sqlitedb.UpsertMappingParams{
		Pos: m.POS, ExternalID: m.ExternalID, Kind: string(m.Kind), Name: m.Name, DishID: m.DishID, Modifier: m.Modifier,
		Ignore: boolInt(m.Ignore), UpdatedAt: us(m.UpdatedAt), UpdatedBy: m.UpdatedBy,
	}))
}

func (s *Store) ListMappings(ctx context.Context, pos string) ([]integrations.Mapping, error) {
	rows, err := s.r.ListMappings(ctx, pos)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]integrations.Mapping, 0, len(rows))
	for _, r := range rows {
		out = append(out, integrations.Mapping{
			POS: r.Pos, ExternalID: r.ExternalID, Kind: integrations.EntryKind(r.Kind), Name: r.Name, DishID: r.DishID,
			Modifier: r.Modifier, Ignore: r.Ignore != 0, UpdatedAt: fromUS(r.UpdatedAt), UpdatedBy: r.UpdatedBy,
		})
	}
	return out, nil
}

func importedFromRow(r sqlitedb.PosOrder) (integrations.Imported, error) {
	o := integrations.Imported{
		POS: r.Pos, ExternalID: r.ExternalID, Version: r.Version, State: integrations.OrderState(r.State), OrderID: r.OrderID,
		Label: r.Label, FirstSeen: fromUS(r.FirstSeen), LastSeen: fromUS(r.LastSeen),
	}
	for _, f := range []struct {
		raw string
		v   any
	}{{r.Added, &o.Added}, {r.Unmapped, &o.Unmapped}, {r.Notices, &o.Notices}, {r.Payload, &o.Payload}} {
		if err := json.Unmarshal([]byte(f.raw), f.v); err != nil {
			return o, fmt.Errorf("decode till order %s: %w", r.ExternalID, err)
		}
	}
	if o.Added == nil {
		o.Added = map[string]int{}
	}
	if o.Unmapped == nil {
		o.Unmapped = []string{}
	}
	if o.Notices == nil {
		o.Notices = []string{}
	}
	return o, nil
}

func (s *Store) GetImported(ctx context.Context, pos, externalID string) (integrations.Imported, error) {
	r, err := s.r.GetImported(ctx, sqlitedb.GetImportedParams{Pos: pos, ExternalID: externalID})
	if err != nil {
		return integrations.Imported{}, mapErr(err)
	}
	return importedFromRow(r)
}

func (s *Store) SaveImported(ctx context.Context, o integrations.Imported) error {
	enc := func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	}
	if o.Added == nil {
		o.Added = map[string]int{}
	}
	if o.Unmapped == nil {
		o.Unmapped = []string{}
	}
	if o.Notices == nil {
		o.Notices = []string{}
	}
	added, err := enc(o.Added)
	if err != nil {
		return err
	}
	unmapped, err := enc(o.Unmapped)
	if err != nil {
		return err
	}
	notices, err := enc(o.Notices)
	if err != nil {
		return err
	}
	payload, err := enc(o.Payload)
	if err != nil {
		return err
	}
	return mapErr(s.w.UpsertImported(ctx, sqlitedb.UpsertImportedParams{
		Pos: o.POS, ExternalID: o.ExternalID, Version: o.Version, State: string(o.State), OrderID: o.OrderID, Label: o.Label,
		Added: added, Unmapped: unmapped, Notices: notices, Payload: payload, FirstSeen: us(o.FirstSeen), LastSeen: us(o.LastSeen),
	}))
}

func (s *Store) ListImported(ctx context.Context, pos string, limit int) ([]integrations.Imported, error) {
	rows, err := s.r.ListImported(ctx, sqlitedb.ListImportedParams{Pos: pos, Limit: int64(limit)})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]integrations.Imported, 0, len(rows))
	for _, r := range rows {
		o, err := importedFromRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func (s *Store) GetPOSState(ctx context.Context, pos string) (integrations.SyncState, error) {
	r, err := s.r.GetPOSState(ctx, pos)
	if err != nil {
		return integrations.SyncState{}, mapErr(err)
	}
	return integrations.SyncState{POS: r.Pos, Since: fromUS(r.Since), LastPollAt: timePtr(r.LastPollAt), LastOKAt: timePtr(r.LastOkAt), LastError: r.LastError}, nil
}

func (s *Store) SavePOSState(ctx context.Context, st integrations.SyncState) error {
	return mapErr(s.w.UpsertPOSState(ctx, sqlitedb.UpsertPOSStateParams{
		Pos: st.POS, Since: us(st.Since), LastPollAt: nullTime(st.LastPollAt), LastOkAt: nullTime(st.LastOKAt), LastError: st.LastError,
	}))
}
