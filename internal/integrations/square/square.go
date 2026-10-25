// Package square reads the catalog and orders from Square over its REST
// API, with plain net/http. Square's API types stay in this package; the
// rest of Kishtin sees integrations types only. Kishtin only reads from
// Square: it never writes orders, payments or card data.
package square

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/integrations"
)

// APIVersion is the Square API version Kishtin is written against.
const APIVersion = "2025-01-23"

// Base URLs for Square's production and sandbox environments.
const (
	ProductionURL = "https://connect.squareup.com"
	SandboxURL    = "https://connect.squareupsandbox.com"
)

// Client is a Square API client.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New returns a client for an environment ("production" or "sandbox"),
// or a base URL for testing against a local stand-in, with an access
// token.
func New(environment, token string, timeout time.Duration) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("square: no access token: set KISHTIN_SQUARE_TOKEN")
	}
	base := ProductionURL
	switch environment {
	case "", "production":
	case "sandbox":
		base = SandboxURL
	default:
		u, err := url.Parse(environment)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("square.environment must be production or sandbox, not %q", environment)
		}
		base = strings.TrimRight(environment, "/")
	}
	return &Client{base: base, token: strings.TrimSpace(token), http: &http.Client{Timeout: timeout}}, nil
}

// Name is "square".
func (c *Client) Name() string { return "square" }

// apiError is Square's error response.
type apiError struct {
	Errors []struct {
		Category string `json:"category"`
		Code     string `json:"code"`
		Detail   string `json:"detail"`
	} `json:"errors"`
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Square-Version", APIVersion)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach Square: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("reading Square's answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var e apiError
		_ = json.Unmarshal(b, &e)
		detail := resp.Status
		if len(e.Errors) > 0 {
			detail = e.Errors[0].Detail
			if detail == "" {
				detail = e.Errors[0].Code
			}
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("Square refused the access token (%s): check KISHTIN_SQUARE_TOKEN", detail) //nolint:staticcheck // Square is a name
		}
		return fmt.Errorf("Square answered %d: %s", resp.StatusCode, detail) //nolint:staticcheck // Square is a name
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("unexpected answer from Square: %w", err)
	}
	return nil
}

type catalogObject struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	IsDeleted bool   `json:"is_deleted"`
	ItemData  *struct {
		Name       string          `json:"name"`
		Variations []catalogObject `json:"variations"`
	} `json:"item_data"`
	ItemVariationData *struct {
		Name string `json:"name"`
	} `json:"item_variation_data"`
	ModifierListData *struct {
		Name      string          `json:"name"`
		Modifiers []catalogObject `json:"modifiers"`
	} `json:"modifier_list_data"`
	ModifierData *struct {
		Name string `json:"name"`
	} `json:"modifier_data"`
}

// maxPages caps paging, so a fault cannot loop forever.
const maxPages = 50

// Catalog lists every item variation and modifier.
func (c *Client) Catalog(ctx context.Context, _ string) ([]integrations.CatalogEntry, error) {
	var out []integrations.CatalogEntry
	cursor := ""
	for range maxPages {
		q := url.Values{"types": {"ITEM,MODIFIER_LIST"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var res struct {
			Objects []catalogObject `json:"objects"`
			Cursor  string          `json:"cursor"`
		}
		if err := c.do(ctx, http.MethodGet, "/v2/catalog/list?"+q.Encode(), nil, &res); err != nil {
			return nil, err
		}
		for _, o := range res.Objects {
			if o.IsDeleted {
				continue
			}
			switch {
			case o.Type == "ITEM" && o.ItemData != nil:
				for _, v := range o.ItemData.Variations {
					if v.IsDeleted || v.ItemVariationData == nil {
						continue
					}
					out = append(out, integrations.CatalogEntry{ID: v.ID, Kind: integrations.EntryItem, Name: v.ItemVariationData.Name, Group: o.ItemData.Name})
				}
			case o.Type == "MODIFIER_LIST" && o.ModifierListData != nil:
				for _, m := range o.ModifierListData.Modifiers {
					if m.IsDeleted || m.ModifierData == nil {
						continue
					}
					out = append(out, integrations.CatalogEntry{ID: m.ID, Kind: integrations.EntryModifier, Name: m.ModifierData.Name, Group: o.ModifierListData.Name})
				}
			}
		}
		if res.Cursor == "" {
			return out, nil
		}
		cursor = res.Cursor
	}
	return nil, errors.New("Square's catalog has too many pages") //nolint:staticcheck // Square is a name
}

type recipient struct {
	DisplayName string `json:"display_name"`
}

type order struct {
	ID         string `json:"id"`
	Version    int64  `json:"version"`
	State      string `json:"state"`
	TicketName string `json:"ticket_name"`
	UpdatedAt  string `json:"updated_at"`
	LineItems  []struct {
		UID             string `json:"uid"`
		CatalogObjectID string `json:"catalog_object_id"`
		Name            string `json:"name"`
		VariationName   string `json:"variation_name"`
		Quantity        string `json:"quantity"`
		Note            string `json:"note"`
		Modifiers       []struct {
			CatalogObjectID string `json:"catalog_object_id"`
			Name            string `json:"name"`
		} `json:"modifiers"`
	} `json:"line_items"`
	Fulfillments []struct {
		Type          string `json:"type"`
		PickupDetails *struct {
			Recipient recipient `json:"recipient"`
		} `json:"pickup_details"`
		DeliveryDetails *struct {
			Recipient recipient `json:"recipient"`
		} `json:"delivery_details"`
		ShipmentDetails *struct {
			Recipient recipient `json:"recipient"`
		} `json:"shipment_details"`
	} `json:"fulfillments"`
}

func (o order) toOrder() (integrations.TillOrder, bool) {
	out := integrations.TillOrder{ID: o.ID, Version: o.Version, Ticket: strings.TrimSpace(o.TicketName), Lines: []integrations.TillLine{}}
	switch o.State {
	case "OPEN":
		out.State = integrations.OrderOpen
	case "COMPLETED":
		out.State = integrations.OrderCompleted
	case "CANCELED":
		out.State = integrations.OrderCanceled
	default:
		// DRAFT orders are not real orders yet.
		return out, false
	}
	if t, err := time.Parse(time.RFC3339, o.UpdatedAt); err == nil {
		out.UpdatedAt = t.UTC()
	}
	for _, f := range o.Fulfillments {
		var r *recipient
		switch f.Type {
		case "PICKUP":
			if f.PickupDetails != nil {
				r = &f.PickupDetails.Recipient
			}
		case "DELIVERY":
			if f.DeliveryDetails != nil {
				r = &f.DeliveryDetails.Recipient
			}
		case "SHIPMENT":
			if f.ShipmentDetails != nil {
				r = &f.ShipmentDetails.Recipient
			}
		default:
			continue
		}
		out.Takeaway = true
		if r != nil && out.Customer == "" {
			out.Customer = strings.TrimSpace(r.DisplayName)
		}
	}
	for _, l := range o.LineItems {
		q, err := strconv.ParseFloat(l.Quantity, 64)
		if err != nil || q <= 0 {
			continue
		}
		name := l.Name
		if l.VariationName != "" && !strings.EqualFold(l.VariationName, "regular") {
			name += " (" + l.VariationName + ")"
		}
		line := integrations.TillLine{UID: l.UID, CatalogID: l.CatalogObjectID, Name: name, Quantity: int(math.Ceil(q)), Note: l.Note}
		for _, m := range l.Modifiers {
			line.Modifiers = append(line.Modifiers, integrations.TillModifier{CatalogID: m.CatalogObjectID, Name: m.Name})
		}
		out.Lines = append(out.Lines, line)
	}
	return out, true
}

// OrdersSince searches a location's orders updated at or after since,
// oldest change first.
func (c *Client) OrdersSince(ctx context.Context, locationID string, since time.Time) ([]integrations.TillOrder, error) {
	var out []integrations.TillOrder
	cursor := ""
	for range maxPages {
		body := map[string]any{
			"location_ids": []string{locationID},
			"limit":        100,
			"query": map[string]any{
				"filter": map[string]any{
					"date_time_filter": map[string]any{"updated_at": map[string]any{"start_at": since.UTC().Format(time.RFC3339)}},
					"state_filter":     map[string]any{"states": []string{"OPEN", "COMPLETED", "CANCELED"}},
				},
				"sort": map[string]any{"sort_field": "UPDATED_AT", "sort_order": "ASC"},
			},
		}
		if cursor != "" {
			body["cursor"] = cursor
		}
		var res struct {
			Orders []order `json:"orders"`
			Cursor string  `json:"cursor"`
		}
		if err := c.do(ctx, http.MethodPost, "/v2/orders/search", body, &res); err != nil {
			return nil, err
		}
		for _, o := range res.Orders {
			if x, ok := o.toOrder(); ok {
				out = append(out, x)
			}
		}
		if res.Cursor == "" {
			return out, nil
		}
		cursor = res.Cursor
	}
	return nil, errors.New("too many changed orders from Square in one go") //nolint:staticcheck // Square is a name
}
