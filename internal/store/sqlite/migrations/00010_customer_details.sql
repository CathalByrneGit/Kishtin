-- +goose Up
-- Customers' names and order notes are cleared a set number of days after
-- an order closes (System settings, customer_details_days). The append-only
-- guards on the order history now let those words be blanked, and nothing
-- else: which events happened, when, by whom and on which device stay fixed,
-- and nothing can be deleted.
DROP TRIGGER order_events_no_update;
-- +goose StatementBegin
CREATE TRIGGER order_events_no_update BEFORE UPDATE ON order_events
WHEN NEW.id IS NOT OLD.id OR NEW.order_id IS NOT OLD.order_id OR NEW.item_id IS NOT OLD.item_id
  OR NEW.course IS NOT OLD.course OR NEW.kind IS NOT OLD.kind OR NEW.at IS NOT OLD.at
  OR NEW.by_user IS NOT OLD.by_user OR NEW.device_id IS NOT OLD.device_id
BEGIN SELECT RAISE(ABORT, 'order events are append-only'); END;
-- +goose StatementEnd
DROP TRIGGER voice_log_no_update;
-- +goose StatementBegin
CREATE TRIGGER voice_log_no_update BEFORE UPDATE ON voice_log
WHEN NEW.id IS NOT OLD.id OR NEW.at IS NOT OLD.at OR NEW.user_id IS NOT OLD.user_id
  OR NEW.source IS NOT OLD.source OR NEW.transcript IS NOT OLD.transcript OR NEW.intent IS NOT OLD.intent
  OR NEW.outcome IS NOT OLD.outcome OR NEW.ref IS NOT OLD.ref
  OR NEW.device_id IS NOT OLD.device_id OR NEW.audio_file IS NOT OLD.audio_file
BEGIN SELECT RAISE(ABORT, 'the voice log is append-only'); END;
-- +goose StatementEnd
DROP TRIGGER plan_snapshots_no_update;
-- +goose StatementBegin
CREATE TRIGGER plan_snapshots_no_update BEFORE UPDATE ON plan_snapshots
WHEN NEW.id IS NOT OLD.id OR NEW.at IS NOT OLD.at OR NEW.trigger IS NOT OLD.trigger
  OR NEW.strategy IS NOT OLD.strategy OR NEW.settings_version IS NOT OLD.settings_version
  OR NEW.hash IS NOT OLD.hash
BEGIN SELECT RAISE(ABORT, 'plan snapshots are append-only'); END;
-- +goose StatementEnd
DROP TRIGGER pos_order_versions_no_update;
-- +goose StatementBegin
CREATE TRIGGER pos_order_versions_no_update BEFORE UPDATE ON pos_order_versions
WHEN NEW.pos IS NOT OLD.pos OR NEW.external_id IS NOT OLD.external_id
  OR NEW.version IS NOT OLD.version OR NEW.seen_at IS NOT OLD.seen_at
BEGIN SELECT RAISE(ABORT, 'till order versions are append-only'); END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER pos_order_versions_no_update;
-- +goose StatementBegin
CREATE TRIGGER pos_order_versions_no_update BEFORE UPDATE ON pos_order_versions
BEGIN SELECT RAISE(ABORT, 'till order versions are append-only'); END;
-- +goose StatementEnd
DROP TRIGGER plan_snapshots_no_update;
-- +goose StatementBegin
CREATE TRIGGER plan_snapshots_no_update BEFORE UPDATE ON plan_snapshots
BEGIN SELECT RAISE(ABORT, 'plan snapshots are append-only'); END;
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
DROP TRIGGER order_events_no_update;
-- +goose StatementBegin
CREATE TRIGGER order_events_no_update BEFORE UPDATE ON order_events
WHEN NEW.id IS NOT OLD.id OR NEW.order_id IS NOT OLD.order_id OR NEW.item_id IS NOT OLD.item_id
  OR NEW.course IS NOT OLD.course OR NEW.kind IS NOT OLD.kind OR NEW.from_val IS NOT OLD.from_val
  OR NEW.to_val IS NOT OLD.to_val OR NEW.detail IS NOT OLD.detail OR NEW.at IS NOT OLD.at
  OR NEW.by_user IS NOT OLD.by_user OR NEW.device_id IS NOT OLD.device_id
BEGIN SELECT RAISE(ABORT, 'order events are append-only'); END;
-- +goose StatementEnd
