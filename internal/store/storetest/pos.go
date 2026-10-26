package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

func testPOS(t *testing.T, s store.Store) {
	ctx := context.Background()
	p := s.POS()

	m := integrations.Mapping{POS: "square", ExternalID: "var-burger", Kind: integrations.EntryItem, Name: "Synthetic burger", DishID: "d1", UpdatedAt: t0, UpdatedBy: "u1"}
	require.NoError(t, p.SaveMapping(ctx, m))
	require.NoError(t, p.SaveMapping(ctx, integrations.Mapping{POS: "square", ExternalID: "var-cola", Kind: integrations.EntryItem, Name: "Synthetic cola", Ignore: true, UpdatedAt: t0, UpdatedBy: "u1"}))
	require.NoError(t, p.SaveMapping(ctx, integrations.Mapping{POS: "other", ExternalID: "x", Kind: integrations.EntryItem, Name: "X", DishID: "d1", UpdatedAt: t0, UpdatedBy: "u1"}))
	m.DishID, m.UpdatedBy, m.UpdatedAt = "d2", "u2", t0.Add(time.Hour)
	require.NoError(t, p.SaveMapping(ctx, m), "a second save replaces")
	got, err := p.ListMappings(ctx, "square")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "d2", got[0].DishID)
	assert.Equal(t, "u2", got[0].UpdatedBy)
	assert.True(t, t0.Add(time.Hour).Equal(got[0].UpdatedAt))
	assert.True(t, got[1].Ignore)

	_, err = p.GetImported(ctx, "square", "sq-1")
	assert.ErrorIs(t, err, store.ErrNotFound)
	o := integrations.Imported{
		POS: "square", ExternalID: "sq-1", Version: 1, State: integrations.OrderOpen, OrderID: "o1", Label: "Table 4",
		Added: map[string]int{"l1": 2}, Unmapped: []string{"1 × Lamb special"}, FirstSeen: t0, LastSeen: t0,
		Payload: integrations.TillOrder{ID: "sq-1", Version: 1, State: integrations.OrderOpen, Ticket: "Table 4", Lines: []integrations.TillLine{{UID: "l1", Name: "Synthetic burger", Quantity: 2}}},
	}
	require.NoError(t, p.SaveImported(ctx, o))
	o2 := integrations.Imported{POS: "square", ExternalID: "sq-2", Version: 4, State: integrations.OrderCanceled, Label: "Takeaway", FirstSeen: t0, LastSeen: t0.Add(time.Minute)}
	require.NoError(t, p.SaveImported(ctx, o2))
	o.Version, o.Notices, o.LastSeen = 2, []string{"a line removed on the till"}, t0.Add(2*time.Minute)
	require.NoError(t, p.SaveImported(ctx, o))
	back, err := p.GetImported(ctx, "square", "sq-1")
	require.NoError(t, err)
	assert.Equal(t, int64(2), back.Version)
	assert.Equal(t, map[string]int{"l1": 2}, back.Added)
	assert.Equal(t, []string{"1 × Lamb special"}, back.Unmapped)
	assert.Equal(t, []string{"a line removed on the till"}, back.Notices)
	assert.True(t, t0.Equal(back.FirstSeen), "first seen is kept")
	assert.Equal(t, "Table 4", back.Payload.Ticket)
	assert.Equal(t, 2, back.Payload.Lines[0].Quantity)
	list, err := p.ListImported(ctx, "square", 10)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "sq-1", list[0].ExternalID, "most recently seen first")
	assert.Empty(t, list[1].Added)
	assert.NotNil(t, list[1].Notices)

	_, err = p.GetPOSState(ctx, "square")
	assert.ErrorIs(t, err, store.ErrNotFound)
	poll := t0.Add(time.Minute)
	st := integrations.SyncState{POS: "square", Since: t0, LastPollAt: &poll, LastError: "cannot reach Square"}
	require.NoError(t, p.SavePOSState(ctx, st))
	st.LastOKAt, st.LastError = &poll, ""
	require.NoError(t, p.SavePOSState(ctx, st))
	back2, err := p.GetPOSState(ctx, "square")
	require.NoError(t, err)
	assert.True(t, t0.Equal(back2.Since))
	require.NotNil(t, back2.LastOKAt)
	assert.True(t, poll.Equal(*back2.LastOKAt))
	assert.Empty(t, back2.LastError)
}
