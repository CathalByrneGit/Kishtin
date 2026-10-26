package web

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/service"
)

func TestKitchenOverHTTP(t *testing.T) {
	e := newEnv(t)
	key := e.mgrKey

	r := e.do(t, http.MethodPost, "/api/menu/stations", key, "", map[string]any{"name": "Grill"})
	require.Equal(t, http.StatusCreated, r.status, string(r.body))
	grill := decode[menu.Station](t, r)
	r = e.do(t, http.MethodPost, "/api/menu/equipment", key, "", map[string]any{"station_id": grill.ID, "name": "Flat-top", "slots": 3})
	require.Equal(t, http.StatusCreated, r.status, string(r.body))
	flat := decode[menu.Equipment](t, r)
	r = e.do(t, http.MethodPost, "/api/menu/dishes", key, "", map[string]any{
		"name": "Synthetic burger", "course": "main", "station_id": grill.ID, "equipment_id": flat.ID,
		"cook_seconds": 480, "rest_seconds": 120, "plate_seconds": 60, "hold_seconds": 180, "batchable": true,
	})
	require.Equal(t, http.StatusCreated, r.status, string(r.body))
	burger := decode[menu.Dish](t, r)

	r = e.do(t, http.MethodPost, "/api/menu/dishes", key, "", map[string]any{"name": "Bad", "course": "brunch", "station_id": grill.ID, "cook_seconds": 60})
	assert.Equal(t, http.StatusUnprocessableEntity, r.status, "an unknown course is refused")

	r = e.do(t, http.MethodPost, "/api/orders", key, "", map[string]any{
		"kind": "table", "table": "4", "items": []map[string]any{{"dish_id": burger.ID, "quantity": 2}},
	})
	require.Equal(t, http.StatusCreated, r.status, string(r.body))
	o := decode[orders.Order](t, r)
	require.Len(t, o.Items, 2)

	r = e.do(t, http.MethodPost, fmt.Sprintf("/api/orders/%s/courses/main/fire", o.ID), key, "", nil)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	r = e.do(t, http.MethodPost, fmt.Sprintf("/api/orders/%s/courses/main/fire", o.ID), key, "", nil)
	assert.Equal(t, http.StatusConflict, r.status, "firing twice is a conflict")

	r = e.do(t, http.MethodGet, "/api/kitchen", key, "", nil)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	view := decode[service.KitchenView](t, r)
	require.Len(t, view.Plan.Stations, 1)
	require.Len(t, view.Plan.Stations[0].Actions, 1)
	a := view.Plan.Stations[0].Actions[0]
	assert.Equal(t, 2, a.Quantity)
	assert.Contains(t, a.Text, "Start 2× Synthetic burger on the flat-top now for Table 4")
	assert.Contains(t, a.Reason, "start by")

	r = e.do(t, http.MethodPost, "/api/kitchen/items", key, "", map[string]any{"item_ids": a.ItemIDs})
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	moved := decode[struct {
		Orders []orders.Order `json:"orders"`
	}](t, r)
	require.Len(t, moved.Orders, 1)
	assert.Equal(t, orders.StateCooking, moved.Orders[0].Items[0].State)

	r = e.do(t, http.MethodPut, "/api/menu/dishes/"+burger.ID+"/availability", key, "", map[string]any{"available": false})
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	r = e.do(t, http.MethodPost, "/api/orders/"+o.ID+"/items", key, "", map[string]any{"items": []map[string]any{{"dish_id": burger.ID}}})
	assert.Equal(t, http.StatusConflict, r.status, "a sold-out dish cannot be ordered")

	r = e.do(t, http.MethodGet, "/api/orders/"+o.ID+"/events", key, "", nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Contains(t, string(r.body), `"kind":"fired"`)

	r = e.do(t, http.MethodGet, "/api/settings/scheduler", key, "", nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Contains(t, string(r.body), `"strategy":"backward"`)
}
