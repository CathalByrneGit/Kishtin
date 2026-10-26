-- name: InsertFile :exec
INSERT INTO files (sha256, mime, size, original_name, uploaded_by, uploaded_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (sha256) DO NOTHING;

-- name: GetFile :one
SELECT * FROM files WHERE sha256 = ?;

-- name: ListFiles :many
SELECT * FROM files ORDER BY uploaded_at, sha256;
