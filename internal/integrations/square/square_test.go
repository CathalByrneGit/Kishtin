package square

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return b
}

func TestCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer synthetic-token", r.Header.Get("Authorization"))
		assert.Equal(t, APIVersion, r.Header.Get("Square-Version"))
		assert.Equal(t, "/v2/catalog/list", r.URL.Path)
		assert.Equal(t, "ITEM,MODIFIER_LIST", r.URL.Query().Get("types"))
		if r.URL.Query().Get("cursor") == "page2" {
			_, _ = w.Write(fixture(t, "catalog2.json"))
			return
		}
		_, _ = w.Write(fixture(t, "catalog.json"))
	}))
	defer srv.Close()
	c, err := New(srv.URL, "synthetic-token", time.Second)
	require.NoError(t, err)
	got, err := c.Catalog(t.Context(), "loc-1")
	require.NoError(t, err)
	assert.Equal(t, []integrations.CatalogEntry{
		{ID: "var-burger", Kind: integrations.EntryItem, Name: "Regular", Group: "Synthetic burger"},
		{ID: "var-burger-dbl", Kind: integrations.EntryItem, Name: "Double", Group: "Synthetic burger"},
		{ID: "mod-welldone", Kind: integrations.EntryModifier, Name: "Well done", Group: "Cooking"},
		{ID: "var-cola", Kind: integrations.EntryItem, Name: "Regular", Group: "Synthetic cola"},
	}, got)
}

func TestOrdersSince(t *testing.T) {
	since := time.Date(2026, 10, 25, 18, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/orders/search", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		require.NoError(t, json.Unmarshal(b, &body))
		assert.Equal(t, []any{"loc-1"}, body["location_ids"])
		q := body["query"].(map[string]any)
		f := q["filter"].(map[string]any)
		assert.Equal(t, "2026-10-25T18:00:00Z", f["date_time_filter"].(map[string]any)["updated_at"].(map[string]any)["start_at"])
		assert.Equal(t, "UPDATED_AT", q["sort"].(map[string]any)["sort_field"])
		_, _ = w.Write(fixture(t, "orders.json"))
	}))
	defer srv.Close()
	c, err := New(srv.URL, "synthetic-token", time.Second)
	require.NoError(t, err)
	got, err := c.OrdersSince(t.Context(), "loc-1", since)
	require.NoError(t, err)
	require.Len(t, got, 2, "drafts are skipped")

	o := got[0]
	assert.Equal(t, "sq-order-1", o.ID)
	assert.Equal(t, int64(3), o.Version)
	assert.Equal(t, integrations.OrderOpen, o.State)
	assert.Equal(t, "Table 4", o.Ticket)
	assert.False(t, o.Takeaway)
	assert.Equal(t, time.Date(2026, 10, 25, 18, 1, 2, 0, time.UTC), o.UpdatedAt)
	require.Len(t, o.Lines, 3)
	assert.Equal(t, integrations.TillLine{UID: "l1", CatalogID: "var-burger", Name: "Synthetic burger", Quantity: 2, Note: "no onion",
		Modifiers: []integrations.TillModifier{{CatalogID: "mod-welldone", Name: "Well done"}}}, o.Lines[0])
	assert.Equal(t, 1, o.Lines[2].Quantity, "fractions round up")

	assert.True(t, got[1].Takeaway)
	assert.Equal(t, "Synthetic customer", got[1].Customer)
	assert.Equal(t, "Synthetic burger (Double)", got[1].Lines[0].Name)
}

func TestErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":[{"category":"AUTHENTICATION_ERROR","code":"UNAUTHORIZED","detail":"This request could not be authorized."}]}`))
	}))
	defer srv.Close()
	c, err := New(srv.URL, "wrong", time.Second)
	require.NoError(t, err)
	_, err = c.OrdersSince(t.Context(), "loc-1", time.Now())
	assert.ErrorContains(t, err, "Square refused the access token (This request could not be authorized.)")

	_, err = New("production", "", time.Second)
	assert.ErrorContains(t, err, "KISHTIN_SQUARE_TOKEN")
	_, err = New("staging", "x", time.Second)
	assert.ErrorContains(t, err, "production or sandbox")
	c, err = New("sandbox", "x", time.Second)
	require.NoError(t, err)
	assert.Equal(t, SandboxURL, c.base)
}
