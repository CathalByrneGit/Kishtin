-- name: InsertSession :exec
INSERT INTO sessions (token_hash, user_id, method, created_at, expires_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetSession :one
SELECT * FROM sessions WHERE token_hash = ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = ?;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at < ?;

-- name: InsertAPIKey :exec
INSERT INTO api_keys (id, kind, user_id, name, token_hash, created_by, created_at, revoked_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys WHERE token_hash = ?;

-- name: ListAPIKeys :many
SELECT * FROM api_keys ORDER BY created_at, id;

-- name: RevokeAPIKey :execrows
UPDATE api_keys SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL;
