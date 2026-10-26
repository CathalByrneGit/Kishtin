-- name: InsertVoiceLog :exec
INSERT INTO voice_log (id, at, user_id, user_name, source, transcript, intent, outcome, reply, ref, device_id, audio_file)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListVoiceLog :many
SELECT * FROM voice_log WHERE at < ? ORDER BY at DESC, id DESC LIMIT ?;

-- name: GetVoiceLog :one
SELECT * FROM voice_log WHERE id = ?;
