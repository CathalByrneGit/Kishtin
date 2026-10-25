-- +goose Up

-- The voice command log: what was heard, what it was taken to mean, and
-- what happened. Append-only, so mistakes can be traced.
CREATE TABLE voice_log (
    id         TEXT PRIMARY KEY,
    at         INTEGER NOT NULL,
    user_id    TEXT NOT NULL,
    user_name  TEXT NOT NULL,
    source     TEXT NOT NULL,
    transcript TEXT NOT NULL,
    intent     TEXT NOT NULL DEFAULT '',
    outcome    TEXT NOT NULL,
    reply      TEXT NOT NULL,
    ref        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX voice_log_at ON voice_log(at);

-- +goose StatementBegin
CREATE TRIGGER voice_log_no_update BEFORE UPDATE ON voice_log
BEGIN SELECT RAISE(ABORT, 'the voice log is append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER voice_log_no_delete BEFORE DELETE ON voice_log
BEGIN SELECT RAISE(ABORT, 'the voice log is append-only'); END;
-- +goose StatementEnd

-- +goose Down
DROP TABLE voice_log;
