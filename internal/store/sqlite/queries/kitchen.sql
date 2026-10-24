-- name: InsertStation :exec
INSERT INTO stations (id, name, position, active, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: UpdateStation :execrows
UPDATE stations SET name = ?, position = ?, active = ?, updated_at = ? WHERE id = ?;

-- name: ListStations :many
SELECT * FROM stations ORDER BY position, name;

-- name: InsertEquipment :exec
INSERT INTO equipment (id, station_id, name, slots, active, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateEquipment :execrows
UPDATE equipment SET station_id = ?, name = ?, slots = ?, active = ?, updated_at = ? WHERE id = ?;

-- name: ListEquipment :many
SELECT * FROM equipment ORDER BY station_id, name;

-- name: InsertDish :exec
INSERT INTO dishes (id, name, course, station_id, equipment_id, batch_capacity, cook_seconds, rest_seconds,
    plate_seconds, hold_seconds, batchable, modifiers, food_category, available, active, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateDish :execrows
UPDATE dishes SET name = ?, course = ?, station_id = ?, equipment_id = ?, batch_capacity = ?, cook_seconds = ?,
    rest_seconds = ?, plate_seconds = ?, hold_seconds = ?, batchable = ?, modifiers = ?, food_category = ?,
    available = ?, active = ?, updated_at = ?
WHERE id = ?;

-- name: GetDish :one
SELECT * FROM dishes WHERE id = ?;

-- name: ListDishes :many
SELECT * FROM dishes ORDER BY name;

-- name: InsertOrder :exec
INSERT INTO orders (id, number, kind, table_name, name, covers, notes, allergy, status, created_by, created_at, closed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateOrder :execrows
UPDATE orders SET table_name = ?, name = ?, covers = ?, notes = ?, allergy = ?, status = ?, closed_at = ? WHERE id = ?;

-- name: GetOrder :one
SELECT * FROM orders WHERE id = ?;

-- name: ListOpenOrders :many
SELECT * FROM orders WHERE status = 'open' ORDER BY created_at, id;

-- name: ListOrdersBetween :many
SELECT * FROM orders WHERE created_at >= ? AND created_at < ? ORDER BY created_at, id;

-- name: MaxOrderNumberSince :one
SELECT CAST(COALESCE(MAX(number), 0) AS INTEGER) FROM orders WHERE created_at >= ?;

-- name: UpsertCourse :exec
INSERT INTO order_courses (order_id, course, state, fired_at, target_at, target_set, urgent, served_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (order_id, course) DO UPDATE SET state = excluded.state, fired_at = excluded.fired_at,
    target_at = excluded.target_at, target_set = excluded.target_set, urgent = excluded.urgent,
    served_at = excluded.served_at;

-- name: ListCourses :many
SELECT * FROM order_courses WHERE order_id = ?;

-- name: InsertItem :exec
INSERT INTO order_items (id, order_id, position, dish_id, dish_name, course, modifiers, notes, state, station_id,
    equipment_id, batch_capacity, hold_seconds, batchable, cook_ms, rest_ms, plate_ms, state_at, started_at,
    ready_at, served_at, void_reason)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateItem :execrows
UPDATE order_items SET course = ?, modifiers = ?, notes = ?, state = ?, state_at = ?, started_at = ?, ready_at = ?,
    served_at = ?, void_reason = ?
WHERE id = ?;

-- name: ListItems :many
SELECT * FROM order_items WHERE order_id = ? ORDER BY position;

-- name: InsertOrderEvent :exec
INSERT INTO order_events (id, order_id, item_id, course, kind, from_val, to_val, detail, at, by_user, by_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListOrderEvents :many
SELECT * FROM order_events WHERE order_id = ? ORDER BY at, id;

-- name: ListItemEventsBetween :many
SELECT * FROM order_events WHERE kind = 'item_state' AND at >= ? AND at < ? ORDER BY at, id;
