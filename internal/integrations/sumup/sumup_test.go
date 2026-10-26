package sumup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
)

// fake is a stand-in for SumUp's API with synthetic sales.
type fake struct {
	t        *testing.T
	queries  []string
	retrieve map[string]map[string]any
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer sup_sk_synthetic" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"title": "Unauthorized", "detail": "Authentication credentials are missing or invalid."})
		return
	}
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch r.URL.Path {
	case "/v0.1/memberships":
		if r.URL.Query().Get("offset") == "0" {
			write(map[string]any{"total_count": 2, "items": []any{
				map[string]any{"resource_id": "MSYNTH01", "type": "merchant", "status": "accepted", "resource": map[string]any{"id": "MSYNTH01", "name": "The Synthetic Kitchen", "attributes": map[string]any{"sandbox": false}}},
			}})
			return
		}
		write(map[string]any{"total_count": 2, "items": []any{
			map[string]any{"resource_id": "MSYNTH02", "type": "merchant", "status": "accepted", "resource": map[string]any{"id": "MSYNTH02", "name": "Synthetic Test", "attributes": map[string]any{"sandbox": true}}},
		}})
	case "/v2.1/merchants/MSYNTH01/transactions/history":
		f.queries = append(f.queries, r.URL.RawQuery)
		q := r.URL.Query()
		if q.Get("oldest_ref") == "" {
			write(map[string]any{
				"items": []any{
					map[string]any{"id": "tx-1", "transaction_code": "TSYNTH1", "status": "SUCCESSFUL", "type": "PAYMENT", "timestamp": "2026-10-26T12:00:00Z"},
					map[string]any{"id": "tx-2", "transaction_code": "TSYNTH2", "status": "FAILED", "type": "PAYMENT", "timestamp": "2026-10-26T12:01:00Z"},
				},
				"links": []any{map[string]any{"rel": "next", "href": "limit=50&oldest_ref=tx-2&order=ascending"}},
			})
			return
		}
		write(map[string]any{
			"items": []any{
				map[string]any{"transaction_id": "tx-3", "transaction_code": "TSYNTH3", "status": "REFUNDED", "type": "PAYMENT", "timestamp": "2026-10-26T11:00:00Z"},
			},
			"links": []any{},
		})
	case "/v2.1/merchants/MSYNTH01/transactions":
		tx, ok := f.retrieve[r.URL.Query().Get("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			write(map[string]string{"message": "Resource not found", "error_code": "NOT_FOUND"})
			return
		}
		write(tx)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newFake(t *testing.T) (*Client, *fake) {
	t.Helper()
	f := &fake{t: t, retrieve: map[string]map[string]any{
		"tx-1": {"id": "tx-1", "transaction_code": "TSYNTH1", "status": "SUCCESSFUL", "timestamp": "2026-10-26T12:00:00Z",
			"products": []any{
				map[string]any{"name": "Burger", "quantity": 2, "price": 12.5},
				map[string]any{"name": "  Fries ", "quantity": 0},
				map[string]any{"name": "", "quantity": 1},
			},
			"events": []any{map[string]any{"timestamp": "2026-10-26T12:00:05Z"}}},
		"tx-3": {"id": "tx-3", "transaction_code": "TSYNTH3", "status": "REFUNDED", "timestamp": "2026-10-26T11:00:00Z",
			"products": []any{map[string]any{"name": "Garden salad", "quantity": 1}},
			"events":   []any{map[string]any{"timestamp": "2026-10-26T12:30:00Z"}}},
	}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "sup_sk_synthetic", 5*time.Second)
	require.NoError(t, err)
	return c, f
}

func TestLocations(t *testing.T) {
	c, _ := newFake(t)
	locs, err := c.Locations(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []integrations.Location{
		{ID: "MSYNTH01", Name: "The Synthetic Kitchen", Active: true},
		{ID: "MSYNTH02", Name: "Synthetic Test (test account)", Active: true},
	}, locs, "reads every page")
}

func TestOrdersSince(t *testing.T) {
	c, f := newFake(t)
	since := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	orders, err := c.OrdersSince(context.Background(), "MSYNTH01", since)
	require.NoError(t, err)

	require.Len(t, f.queries, 2, "follows the next link")
	assert.Contains(t, f.queries[0], "changes_since=2026-10-26T10%3A00%3A00Z")
	assert.Contains(t, f.queries[0], "order=ascending")
	assert.Contains(t, f.queries[0], "types%5B%5D=PAYMENT")
	assert.Equal(t, "limit=50&oldest_ref=tx-2&order=ascending", f.queries[1], "uses the link as SumUp gave it")

	require.Len(t, orders, 2, "the failed payment is not an order")
	sale := orders[0]
	assert.Equal(t, "tx-1", sale.ID)
	assert.Equal(t, int64(1), sale.Version)
	assert.Equal(t, integrations.OrderOpen, sale.State, "paid and waiting to be cooked")
	assert.True(t, sale.Takeaway)
	assert.Equal(t, "SumUp TSYNTH1", sale.Ticket)
	assert.Equal(t, time.Date(2026, 10, 26, 12, 0, 5, 0, time.UTC), sale.UpdatedAt, "the latest event")
	require.Len(t, sale.Lines, 2, "an item with no name is left out")
	assert.Equal(t, integrations.TillLine{UID: "0:name:burger", CatalogID: "name:burger", Name: "Burger", Quantity: 2}, sale.Lines[0])
	assert.Equal(t, "name:fries", sale.Lines[1].CatalogID)
	assert.Equal(t, 1, sale.Lines[1].Quantity, "a zero quantity counts as one")

	refund := orders[1]
	assert.Equal(t, "tx-3", refund.ID)
	assert.Equal(t, int64(2), refund.Version, "a refund is a newer version, so the pass hears of it")
	assert.Equal(t, integrations.OrderCanceled, refund.State)
	assert.Len(t, refund.Lines, 1, "keeps its items, so nothing looks removed")
	assert.True(t, refund.UpdatedAt.After(sale.UpdatedAt), "ordered by when it changed")

	late, err := c.OrdersSince(context.Background(), "MSYNTH01", time.Date(2026, 10, 26, 12, 30, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, late, 1, "a paid sale from before since is not imported, even if SumUp sends it")
	assert.Equal(t, "tx-3", late[0].ID, "a refund always is")
}

func TestCatalogFromRecentSales(t *testing.T) {
	c, f := newFake(t)
	f.retrieve["tx-2"] = map[string]any{"id": "tx-2", "status": "SUCCESSFUL", "products": []any{map[string]any{"name": "burger", "quantity": 1}, map[string]any{"name": "Soup of the day", "quantity": 1}}}
	entries, err := c.Catalog(context.Background(), "MSYNTH01")
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		assert.Equal(t, integrations.EntryItem, e.Kind)
		assert.Equal(t, CatalogID(e.Name), e.ID)
		names = append(names, e.Name)
	}
	assert.Contains(t, names, "Fries")
	assert.Contains(t, names, "Soup of the day")
	assert.Equal(t, 1, strings.Count(strings.ToLower(strings.Join(names, ",")), "burger"), "the same item in any case is one entry")
	assert.Contains(t, f.queries[0], "statuses%5B%5D=SUCCESSFUL")
	assert.Contains(t, f.queries[0], "order=descending")
}

func TestRefusedKey(t *testing.T) {
	c, _ := newFake(t)
	c.token = "sup_sk_wrong"
	_, err := c.Locations(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SumUp refused the API key (Authentication credentials are missing or invalid.)")
}

func TestNew(t *testing.T) {
	_, err := New("production", " ", time.Second)
	assert.Error(t, err)
	_, err = New("sandbox", "k", time.Second)
	assert.Error(t, err, "SumUp has no separate sandbox address")
	c, err := New("", "k", time.Second)
	require.NoError(t, err)
	assert.Equal(t, ProductionURL, c.base)
}

func TestCatalogID(t *testing.T) {
	assert.Equal(t, "name:soup of the day", CatalogID("  Soup  of the DAY "))
}

func TestMerchantNotChosenYet(t *testing.T) {
	c, _ := newFake(t)
	_, err := c.Catalog(context.Background(), "")
	assert.ErrorContains(t, err, "reaches 2 merchant accounts: choose", "two accounts: the manager chooses")
}
