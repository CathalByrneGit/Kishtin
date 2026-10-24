-- name: InsertUser :exec
INSERT INTO users (id, name, role, active, pin_hash, password_hash, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateUser :execrows
UPDATE users
SET name = ?, role = ?, active = ?, pin_hash = ?, password_hash = ?, updated_at = ?
WHERE id = ?;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: ListUsers :many
SELECT * FROM users ORDER BY name COLLATE NOCASE, id;
