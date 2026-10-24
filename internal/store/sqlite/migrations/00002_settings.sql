-- +goose Up

-- Versioned settings: reference guidance, food-safety rules, managed lists
-- and system settings. Each change appends a version.
CREATE TABLE settings_versions (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,
    -- A version is superseded at most once, so settings never fork.
    supersedes TEXT UNIQUE REFERENCES settings_versions(id),
    created_by TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    reason     TEXT NOT NULL,
    plan_ref   TEXT NOT NULL DEFAULT '',
    -- The settings as JSON, as written by Go.
    body       TEXT NOT NULL
);
CREATE INDEX settings_versions_kind ON settings_versions(kind, created_at);

-- +goose StatementBegin
CREATE TRIGGER settings_versions_no_update BEFORE UPDATE ON settings_versions
BEGIN SELECT RAISE(ABORT, 'settings versions are append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER settings_versions_no_delete BEFORE DELETE ON settings_versions
BEGIN SELECT RAISE(ABORT, 'settings versions are append-only'); END;
-- +goose StatementEnd

-- The food-safety rules version in force when a record was saved.
ALTER TABLE haccp_records ADD COLUMN rules_version_id TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down migration: settings history is never dropped.
