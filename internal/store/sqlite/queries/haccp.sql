-- name: InsertRecord :exec
INSERT INTO haccp_records (
    id, seq, type, logged_by, logged_by_name, logged_at, reading, outcome,
    outcome_reason, limit_version_id, limit_scope, corrective_action,
    corrects, correction_reason, prev_hash, hash
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: LastRecord :one
SELECT * FROM haccp_records ORDER BY seq DESC LIMIT 1;

-- name: GetRecord :one
SELECT * FROM haccp_records WHERE id = ?;

-- name: RecordsAfterSeq :many
SELECT * FROM haccp_records WHERE seq > ? ORDER BY seq LIMIT ?;

-- name: InsertLimitVersion :exec
INSERT INTO limit_versions (
    id, check_type, scope_kind, scope_ref, supersedes, created_by, created_at, body
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetLimitVersion :one
SELECT body FROM limit_versions WHERE id = ?;

-- name: ListLimitVersions :many
SELECT body FROM limit_versions ORDER BY created_at, id;

-- name: ListRecordsOldest :many
SELECT * FROM haccp_records
WHERE logged_at >= @from_us AND logged_at < @to_us
  -- types_csv is a comma-separated list of record types; empty means all.
  AND (CAST(@types_csv AS TEXT) = '' OR instr(',' || @types_csv || ',', ',' || type || ',') > 0)
ORDER BY logged_at, seq
LIMIT @max_rows;

-- name: ListRecordsNewest :many
SELECT * FROM haccp_records
WHERE logged_at >= @from_us AND logged_at < @to_us
  -- types_csv is a comma-separated list of record types; empty means all.
  AND (CAST(@types_csv AS TEXT) = '' OR instr(',' || @types_csv || ',', ',' || type || ',') > 0)
ORDER BY logged_at DESC, seq DESC
LIMIT @max_rows;
