-- +goose Up

-- Timestamps are INTEGER microseconds since the Unix epoch, UTC.
-- Closed value lists (record types, outcomes, roles) are TEXT and are
-- validated in Go, so every backend behaves the same.

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE COLLATE NOCASE,
    role          TEXT NOT NULL,
    active        INTEGER NOT NULL,
    pin_hash      TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    method     TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX sessions_expires_at ON sessions(expires_at);

CREATE TABLE api_keys (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,
    user_id    TEXT REFERENCES users(id),
    name       TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_by TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    revoked_at INTEGER
);

CREATE TABLE limit_versions (
    id          TEXT PRIMARY KEY,
    check_type  TEXT NOT NULL,
    scope_kind  TEXT NOT NULL,
    scope_ref   TEXT NOT NULL,
    -- A version is superseded at most once, so limits never fork.
    supersedes  TEXT UNIQUE REFERENCES limit_versions(id),
    created_by  TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    -- The full version as JSON, as written by Go.
    body        TEXT NOT NULL
);

CREATE TABLE haccp_records (
    id                TEXT PRIMARY KEY,
    seq               INTEGER NOT NULL UNIQUE,
    type              TEXT NOT NULL,
    logged_by         TEXT NOT NULL,
    logged_by_name    TEXT NOT NULL,
    logged_at         INTEGER NOT NULL,
    reading           TEXT NOT NULL,
    outcome           TEXT NOT NULL,
    outcome_reason    TEXT NOT NULL,
    limit_version_id  TEXT NOT NULL DEFAULT '',
    limit_scope       TEXT NOT NULL DEFAULT '',
    corrective_action TEXT NOT NULL DEFAULT '',
    corrects          TEXT NOT NULL DEFAULT '',
    correction_reason TEXT NOT NULL DEFAULT '',
    prev_hash         TEXT NOT NULL,
    hash              TEXT NOT NULL UNIQUE
);
CREATE INDEX haccp_records_logged_at ON haccp_records(logged_at);

CREATE TABLE files (
    sha256        TEXT PRIMARY KEY,
    mime          TEXT NOT NULL,
    size          INTEGER NOT NULL,
    original_name TEXT NOT NULL DEFAULT '',
    uploaded_by   TEXT NOT NULL,
    uploaded_at   INTEGER NOT NULL
);

-- The append-only rule is enforced in Go on every backend. These triggers
-- are a second guard for anyone editing the SQLite file by hand.
-- +goose StatementBegin
CREATE TRIGGER haccp_records_no_update BEFORE UPDATE ON haccp_records
BEGIN SELECT RAISE(ABORT, 'HACCP records are append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER haccp_records_no_delete BEFORE DELETE ON haccp_records
BEGIN SELECT RAISE(ABORT, 'HACCP records are append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER limit_versions_no_update BEFORE UPDATE ON limit_versions
BEGIN SELECT RAISE(ABORT, 'limit versions are append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER limit_versions_no_delete BEFORE DELETE ON limit_versions
BEGIN SELECT RAISE(ABORT, 'limit versions are append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER files_no_update BEFORE UPDATE ON files
BEGIN SELECT RAISE(ABORT, 'file records are append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER files_no_delete BEFORE DELETE ON files
BEGIN SELECT RAISE(ABORT, 'file records are append-only'); END;
-- +goose StatementEnd

-- +goose Down
-- No down migration: HACCP history is never dropped.
