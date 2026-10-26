-- name: UpsertMapping :exec
INSERT INTO pos_mappings (pos, external_id, kind, name, dish_id, modifier, ignore, updated_at, updated_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (pos, external_id) DO UPDATE SET
    kind = excluded.kind, name = excluded.name, dish_id = excluded.dish_id, modifier = excluded.modifier,
    ignore = excluded.ignore, updated_at = excluded.updated_at, updated_by = excluded.updated_by;

-- name: ListMappings :many
SELECT * FROM pos_mappings WHERE pos = ? ORDER BY kind, name;

-- name: GetImported :one
SELECT * FROM pos_orders WHERE pos = ? AND external_id = ?;

-- name: UpsertImported :exec
INSERT INTO pos_orders (pos, external_id, version, state, order_id, label, added, unmapped, notices, payload, first_seen, last_seen)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (pos, external_id) DO UPDATE SET
    version = excluded.version, state = excluded.state, order_id = excluded.order_id, label = excluded.label,
    added = excluded.added, unmapped = excluded.unmapped, notices = excluded.notices, payload = excluded.payload,
    last_seen = excluded.last_seen;

-- name: ListImported :many
SELECT * FROM pos_orders WHERE pos = ? ORDER BY last_seen DESC, external_id LIMIT ?;

-- name: GetPOSState :one
SELECT * FROM pos_state WHERE pos = ?;

-- name: UpsertPOSState :exec
INSERT INTO pos_state (pos, since, last_poll_at, last_ok_at, last_error) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (pos) DO UPDATE SET
    since = excluded.since, last_poll_at = excluded.last_poll_at, last_ok_at = excluded.last_ok_at, last_error = excluded.last_error;

-- name: InsertImportedVersion :exec
INSERT INTO pos_order_versions (pos, external_id, version, seen_at, payload)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (pos, external_id, version) DO NOTHING;

-- name: ListImportedVersions :many
SELECT * FROM pos_order_versions WHERE pos = ? AND external_id = ? ORDER BY version;
