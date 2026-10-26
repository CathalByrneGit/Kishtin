-- +goose Up

-- What the scheduler recommended, kept each time the plan changes, so a
-- service can be looked back on: what each station was told to start and
-- when, and why each course was at risk or late. Append-only.
CREATE TABLE plan_snapshots (
    id               TEXT PRIMARY KEY,
    at               INTEGER NOT NULL,
    trigger          TEXT NOT NULL,
    strategy         TEXT NOT NULL,
    settings_version TEXT NOT NULL,
    hash             TEXT NOT NULL,
    body             TEXT NOT NULL
);
CREATE INDEX plan_snapshots_at ON plan_snapshots (at);

-- Things staff and screens did that are not HACCP records or order events:
-- dishes sold out and back on, logins, screens connecting and dropping,
-- how each HACCP record was entered. Append-only, except that a leaver's
-- name is replaced when their details are anonymised.
CREATE TABLE activity (
    id          TEXT PRIMARY KEY,
    at          INTEGER NOT NULL,
    kind        TEXT NOT NULL,
    user_id     TEXT NOT NULL DEFAULT '',
    user_name   TEXT NOT NULL DEFAULT '',
    device_id   TEXT NOT NULL DEFAULT '',
    device_name TEXT NOT NULL DEFAULT '',
    subject     TEXT NOT NULL DEFAULT '',
    detail      TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX activity_at ON activity (at);
CREATE INDEX activity_kind_at ON activity (kind, at);

-- Every version of every till order seen, not only the latest.
CREATE TABLE pos_order_versions (
    pos         TEXT NOT NULL,
    external_id TEXT NOT NULL,
    version     INTEGER NOT NULL,
    seen_at     INTEGER NOT NULL,
    payload     TEXT NOT NULL,
    PRIMARY KEY (pos, external_id, version)
);

-- Which device (API key) each order event and voice command came from.
ALTER TABLE order_events ADD COLUMN device_id TEXT NOT NULL DEFAULT '';
ALTER TABLE voice_log ADD COLUMN device_id TEXT NOT NULL DEFAULT '';
-- The recording of a voice command, when the restaurant keeps them.
ALTER TABLE voice_log ADD COLUMN audio_file TEXT NOT NULL DEFAULT '';
-- When a leaver's details were anonymised.
ALTER TABLE users ADD COLUMN anonymised_at INTEGER;

-- The logs stay append-only. The one change allowed is replacing a
-- leaver's name when their details are anonymised; nothing else about an
-- entry can change, and nothing can be deleted.
DROP TRIGGER order_events_no_update;
-- +goose StatementBegin
CREATE TRIGGER order_events_no_update BEFORE UPDATE ON order_events
WHEN NEW.id IS NOT OLD.id OR NEW.order_id IS NOT OLD.order_id OR NEW.item_id IS NOT OLD.item_id
  OR NEW.course IS NOT OLD.course OR NEW.kind IS NOT OLD.kind OR NEW.from_val IS NOT OLD.from_val
  OR NEW.to_val IS NOT OLD.to_val OR NEW.detail IS NOT OLD.detail OR NEW.at IS NOT OLD.at
  OR NEW.by_user IS NOT OLD.by_user OR NEW.device_id IS NOT OLD.device_id
BEGIN SELECT RAISE(ABORT, 'order events are append-only'); END;
-- +goose StatementEnd
DROP TRIGGER voice_log_no_update;
-- +goose StatementBegin
CREATE TRIGGER voice_log_no_update BEFORE UPDATE ON voice_log
WHEN NEW.id IS NOT OLD.id OR NEW.at IS NOT OLD.at OR NEW.user_id IS NOT OLD.user_id
  OR NEW.source IS NOT OLD.source OR NEW.transcript IS NOT OLD.transcript OR NEW.intent IS NOT OLD.intent
  OR NEW.outcome IS NOT OLD.outcome OR NEW.reply IS NOT OLD.reply OR NEW.ref IS NOT OLD.ref
  OR NEW.device_id IS NOT OLD.device_id OR NEW.audio_file IS NOT OLD.audio_file
BEGIN SELECT RAISE(ABORT, 'the voice log is append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER activity_no_update BEFORE UPDATE ON activity
WHEN NEW.id IS NOT OLD.id OR NEW.at IS NOT OLD.at OR NEW.kind IS NOT OLD.kind OR NEW.user_id IS NOT OLD.user_id
  OR NEW.device_id IS NOT OLD.device_id OR NEW.device_name IS NOT OLD.device_name
  OR NEW.subject IS NOT OLD.subject OR NEW.detail IS NOT OLD.detail
BEGIN SELECT RAISE(ABORT, 'the activity log is append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER activity_no_delete BEFORE DELETE ON activity
BEGIN SELECT RAISE(ABORT, 'the activity log is append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER plan_snapshots_no_update BEFORE UPDATE ON plan_snapshots
BEGIN SELECT RAISE(ABORT, 'plan snapshots are append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER plan_snapshots_no_delete BEFORE DELETE ON plan_snapshots
BEGIN SELECT RAISE(ABORT, 'plan snapshots are append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER pos_order_versions_no_update BEFORE UPDATE ON pos_order_versions
BEGIN SELECT RAISE(ABORT, 'till order versions are append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER pos_order_versions_no_delete BEFORE DELETE ON pos_order_versions
BEGIN SELECT RAISE(ABORT, 'till order versions are append-only'); END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER pos_order_versions_no_delete;
DROP TRIGGER pos_order_versions_no_update;
DROP TRIGGER plan_snapshots_no_delete;
DROP TRIGGER plan_snapshots_no_update;
DROP TRIGGER activity_no_delete;
DROP TRIGGER activity_no_update;
DROP TRIGGER voice_log_no_update;
-- +goose StatementBegin
CREATE TRIGGER voice_log_no_update BEFORE UPDATE ON voice_log
BEGIN SELECT RAISE(ABORT, 'the voice log is append-only'); END;
-- +goose StatementEnd
DROP TRIGGER order_events_no_update;
-- +goose StatementBegin
CREATE TRIGGER order_events_no_update BEFORE UPDATE ON order_events
BEGIN SELECT RAISE(ABORT, 'order events are append-only'); END;
-- +goose StatementEnd
ALTER TABLE users DROP COLUMN anonymised_at;
ALTER TABLE voice_log DROP COLUMN audio_file;
ALTER TABLE voice_log DROP COLUMN device_id;
ALTER TABLE order_events DROP COLUMN device_id;
DROP TABLE pos_order_versions;
DROP INDEX activity_kind_at;
DROP INDEX activity_at;
DROP TABLE activity;
DROP INDEX plan_snapshots_at;
DROP TABLE plan_snapshots;
