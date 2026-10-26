// Package sumup reads paid sales from SumUp over its REST API, with plain
// net/http. SumUp's API types stay in this package; the rest of Kishtin
// sees integrations types only. Kishtin only reads from SumUp: it never
// takes payments, refunds or card data.
//
// SumUp's public API is about payments, not orders: it has no tables,
// courses or open tabs, and a sale reaches it once it is paid. So each
// paid sale (a "transaction") with its items becomes a takeaway-style
// kitchen order. That suits counter service and takeaways, where people
// pay first. SumUp items have names but no catalog IDs or modifiers, so
// they are matched to dishes by name.
package sumup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
)

// ProductionURL is SumUp's API.
const ProductionURL = "https://api.sumup.com"

// maxPages caps how many pages one call reads, so a first poll after a
// long gap cannot run on.
const maxPages = 20

// catalogSales is how many recent sales the catalog reads items from.
const catalogSales = 60

// Client is a SumUp API client.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New returns a client for SumUp's API ("production", or a base URL for
// testing against a local stand-in) with an API key.
func New(environment, token string, timeout time.Duration) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("sumup: no API key: create one in SumUp's dashboard, Settings, For Developers, Toolkit, API Keys")
	}
	base := ProductionURL
	switch environment {
	case "", "production":
	default:
		u, err := url.Parse(environment)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("sumup environment must be production, not %q", environment)
		}
		base = strings.TrimRight(environment, "/")
	}
	return &Client{base: base, token: strings.TrimSpace(token), http: &http.Client{Timeout: timeout}}, nil
}

// Name is "sumup".
func (c *Client) Name() string { return "sumup" }

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach SumUp: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("reading SumUp's answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// SumUp answers errors as a problem document (title, detail) or
		// its older error object (message, error_code).
		var e struct {
			Title     string `json:"title"`
			Detail    string `json:"detail"`
			Message   string `json:"message"`
			ErrorCode string `json:"error_code"`
		}
		_ = json.Unmarshal(b, &e)
		detail := resp.Status
		for _, d := range []string{e.Detail, e.Message, e.Title, e.ErrorCode} {
			if d != "" {
				detail = d
				break
			}
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("SumUp refused the API key (%s): check it in SumUp's dashboard", detail) //nolint:staticcheck // SumUp is a name
		}
		return fmt.Errorf("SumUp answered %d: %s", resp.StatusCode, detail) //nolint:staticcheck // SumUp is a name
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("unexpected answer from SumUp: %w", err)
	}
	return nil
}

type membership struct {
	ResourceID string `json:"resource_id"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Resource   struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Attributes struct {
			Sandbox bool `json:"sandbox"`
		} `json:"attributes"`
	} `json:"resource"`
}

// Locations lists the merchant accounts the key can read. A merchant
// account's code is its location ID. This is also how a new key is
// checked.
func (c *Client) Locations(ctx context.Context) ([]integrations.Location, error) {
	var out []integrations.Location
	for offset := 0; offset < 25*maxPages; offset += 25 {
		var res struct {
			Items      []membership `json:"items"`
			TotalCount int          `json:"total_count"`
		}
		q := url.Values{"kind": {"merchant"}, "offset": {fmt.Sprint(offset)}, "limit": {"25"}}
		if err := c.get(ctx, "/v0.1/memberships", q, &res); err != nil {
			return nil, err
		}
		for _, m := range res.Items {
			if m.Type != "" && m.Type != "merchant" {
				continue
			}
			id := m.Resource.ID
			if id == "" {
				id = m.ResourceID
			}
			name := m.Resource.Name
			if name == "" {
				name = id
			}
			if m.Resource.Attributes.Sandbox {
				name += " (test account)"
			}
			out = append(out, integrations.Location{ID: id, Name: name, Active: m.Status == "" || m.Status == "accepted"})
		}
		if len(res.Items) == 0 || offset+len(res.Items) >= res.TotalCount {
			break
		}
	}
	return out, nil
}

// merchantOr returns the merchant code to read from: the one chosen, or,
// when none is chosen yet, the account's only merchant account.
func (c *Client) merchantOr(ctx context.Context, merchant string) (string, error) {
	if merchant != "" {
		return merchant, nil
	}
	locs, err := c.Locations(ctx)
	if err != nil {
		return "", err
	}
	if len(locs) != 1 {
		return "", fmt.Errorf("this SumUp key reaches %d merchant accounts: choose which one to import from first", len(locs))
	}
	return locs[0].ID, nil
}

// historyItem is one sale in SumUp's transaction history.
type historyItem struct {
	ID            string `json:"id"`
	TransactionID string `json:"transaction_id"`
	Code          string `json:"transaction_code"`
	Status        string `json:"status"`
	Type          string `json:"type"`
	Timestamp     string `json:"timestamp"`
}

func (h historyItem) id() string {
	if h.ID != "" {
		return h.ID
	}
	return h.TransactionID
}

type historyPage struct {
	Items []historyItem `json:"items"`
	Links []struct {
		Rel  string `json:"rel"`
		Href string `json:"href"`
	} `json:"links"`
}

// history reads every page of the transaction history for q.
func (c *Client) history(ctx context.Context, merchant string, q url.Values, limit int) ([]historyItem, error) {
	path := "/v2.1/merchants/" + url.PathEscape(merchant) + "/transactions/history"
	var out []historyItem
	for range maxPages {
		var page historyPage
		if err := c.get(ctx, path, q, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		if limit > 0 && len(out) >= limit {
			return out[:limit], nil
		}
		next := ""
		for _, l := range page.Links {
			if l.Rel == "next" {
				next = l.Href
			}
		}
		if next == "" || len(page.Items) == 0 {
			break
		}
		// The link is a query string to use as it is.
		nq, err := url.ParseQuery(strings.TrimPrefix(next, "?"))
		if err != nil {
			return nil, fmt.Errorf("unexpected page link from SumUp: %w", err)
		}
		q = nq
	}
	return out, nil
}

// transaction is one sale in full, with its items.
type transaction struct {
	ID        string `json:"id"`
	Code      string `json:"transaction_code"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Products  []struct {
		Name     string `json:"name"`
		Quantity int    `json:"quantity"`
	} `json:"products"`
	Events []struct {
		Timestamp string `json:"timestamp"`
	} `json:"events"`
}

func (c *Client) transaction(ctx context.Context, merchant, id string) (transaction, error) {
	var t transaction
	err := c.get(ctx, "/v2.1/merchants/"+url.PathEscape(merchant)+"/transactions", url.Values{"id": {id}}, &t)
	return t, err
}

// CatalogID is how a SumUp item is known to Kishtin: SumUp items have no
// IDs in its API, so by name, in lower case with spaces tidied.
func CatalogID(name string) string {
	return "name:" + strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// statusVersion turns a sale's status into an order state and version.
// Sales that did not go through are not orders.
func statusVersion(status string) (integrations.OrderState, int64, bool) {
	switch status {
	case "SUCCESSFUL":
		// Paid, and waiting to be cooked: open to the kitchen.
		return integrations.OrderOpen, 1, true
	case "REFUNDED", "CANCELLED", "CHARGE_BACK":
		return integrations.OrderCanceled, 2, true
	}
	return "", 0, false
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// toOrder turns a paid sale into a till order: a takeaway named by its
// SumUp code, one line per item.
func toOrder(t transaction, state integrations.OrderState, version int64) integrations.TillOrder {
	o := integrations.TillOrder{ID: t.ID, Version: version, State: state, Takeaway: true, Ticket: "SumUp " + t.Code, Lines: []integrations.TillLine{}}
	updated := parseTime(t.Timestamp)
	for _, e := range t.Events {
		if et := parseTime(e.Timestamp); et.After(updated) {
			updated = et
		}
	}
	o.UpdatedAt = updated
	for i, p := range t.Products {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		q := p.Quantity
		if q < 1 {
			q = 1
		}
		o.Lines = append(o.Lines, integrations.TillLine{UID: fmt.Sprintf("%d:%s", i, CatalogID(name)), CatalogID: CatalogID(name), Name: name, Quantity: q})
	}
	return o
}

// OrdersSince returns paid, refunded and cancelled sales changed at or
// after since, each read in full for its items, oldest change first.
func (c *Client) OrdersSince(ctx context.Context, merchant string, since time.Time) ([]integrations.TillOrder, error) {
	merchant, err := c.merchantOr(ctx, merchant)
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"changes_since": {since.UTC().Format(time.RFC3339)},
		"order":         {"ascending"},
		"limit":         {"50"},
		"types[]":       {"PAYMENT"},
	}
	items, err := c.history(ctx, merchant, q, 0)
	if err != nil {
		return nil, err
	}
	var out []integrations.TillOrder
	seen := map[string]bool{}
	for _, it := range items {
		if _, _, ok := statusVersion(it.Status); !ok || seen[it.id()] {
			continue
		}
		seen[it.id()] = true
		t, err := c.transaction(ctx, merchant, it.id())
		if err != nil {
			return nil, fmt.Errorf("sale %s: %w", it.Code, err)
		}
		state, version, ok := statusVersion(t.Status)
		if !ok {
			continue
		}
		if t.ID == "" {
			t.ID = it.id()
		}
		o := toOrder(t, state, version)
		// SumUp filters on changes_since, but a paid sale from before
		// import started must never reach the kitchen, so check again.
		// Refunds and cancellations always pass: the pass needs to hear.
		if state == integrations.OrderOpen && o.UpdatedAt.Before(since.Truncate(time.Second)) {
			continue
		}
		out = append(out, o)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	return out, nil
}

// Catalog lists the items sold in recent paid sales. SumUp's public API
// has no product list, so this is the items the till has actually sold;
// an item not sold yet appears once it is.
func (c *Client) Catalog(ctx context.Context, merchant string) ([]integrations.CatalogEntry, error) {
	merchant, err := c.merchantOr(ctx, merchant)
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"order":       {"descending"},
		"limit":       {"50"},
		"types[]":     {"PAYMENT"},
		"statuses[]":  {"SUCCESSFUL"},
		"oldest_time": {time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339)},
	}
	items, err := c.history(ctx, merchant, q, catalogSales)
	if err != nil {
		return nil, err
	}
	var (
		mu    sync.Mutex
		names = map[string]string{}
		first error
		wg    sync.WaitGroup
		slots = make(chan struct{}, 4)
	)
	for _, it := range items {
		wg.Add(1)
		slots <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-slots }()
			t, err := c.transaction(ctx, merchant, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if first == nil {
					first = err
				}
				return
			}
			for _, p := range t.Products {
				if n := strings.TrimSpace(p.Name); n != "" {
					if _, ok := names[CatalogID(n)]; !ok {
						names[CatalogID(n)] = n
					}
				}
			}
		}(it.id())
	}
	wg.Wait()
	if first != nil && len(names) == 0 {
		return nil, first
	}
	out := make([]integrations.CatalogEntry, 0, len(names))
	for id, n := range names {
		out = append(out, integrations.CatalogEntry{ID: id, Kind: integrations.EntryItem, Name: n})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}
