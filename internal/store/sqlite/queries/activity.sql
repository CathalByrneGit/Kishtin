-- name: InsertActivity :exec
INSERT INTO activity (id, at, kind, user_id, user_name, device_id, device_name, subject, detail)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListActivity :many
SELECT * FROM activity
WHERE at >= @from_us AND at < @to_us
  -- kinds_csv is a comma-separated list of kinds; empty means all.
  AND (CAST(@kinds_csv AS TEXT) = '' OR instr(',' || @kinds_csv || ',', ',' || kind || ',') > 0)
ORDER BY at, id
LIMIT @max_rows;

-- name: InsertPlanSnapshot :exec
INSERT INTO plan_snapshots (id, at, trigger, strategy, settings_version, hash, body)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: LastPlanSnapshot :one
SELECT * FROM plan_snapshots ORDER BY at DESC, id DESC LIMIT 1;

-- name: ListPlanSnapshots :many
SELECT * FROM plan_snapshots WHERE at >= @from_us AND at < @to_us ORDER BY at, id LIMIT @max_rows;

-- name: CountUserMentions :one
SELECT
    (SELECT count(*) FROM order_events WHERE by_user = @user_id) AS order_events,
    (SELECT count(*) FROM voice_log WHERE voice_log.user_id = @user_id) AS voice_log,
    (SELECT count(*) FROM activity WHERE activity.user_id = @user_id) AS activity,
    (SELECT count(*) FROM haccp_records WHERE logged_by = @user_id) AS haccp_records;

-- name: AnonymiseOrderEvents :execrows
UPDATE order_events SET by_name = @name WHERE by_user = @user_id;

-- name: AnonymiseVoiceLog :execrows
UPDATE voice_log SET user_name = @name WHERE user_id = @user_id;

-- name: AnonymiseActivity :execrows
UPDATE activity SET user_name = @name WHERE user_id = @user_id;
