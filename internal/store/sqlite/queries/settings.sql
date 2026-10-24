-- name: InsertSettings :exec
INSERT INTO settings_versions (id, kind, supersedes, created_by, created_at, reason, plan_ref, body)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSettings :one
SELECT * FROM settings_versions WHERE id = ?;

-- name: ListSettings :many
SELECT * FROM settings_versions WHERE kind = ? ORDER BY created_at, id;
