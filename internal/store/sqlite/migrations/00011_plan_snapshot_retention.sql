-- +goose Up
-- Kept plans can be cleared after a number of days a manager sets (System
-- settings, plan_snapshot_days; off by default). The guard now lets plans
-- older than 30 days be deleted, and nothing newer.
DROP TRIGGER plan_snapshots_no_delete;
-- +goose StatementBegin
CREATE TRIGGER plan_snapshots_no_delete BEFORE DELETE ON plan_snapshots
WHEN OLD.at > (CAST(strftime('%s', 'now') AS INTEGER) - 30 * 86400) * 1000000
BEGIN SELECT RAISE(ABORT, 'plan snapshots are append-only'); END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER plan_snapshots_no_delete;
-- +goose StatementBegin
CREATE TRIGGER plan_snapshots_no_delete BEFORE DELETE ON plan_snapshots
BEGIN SELECT RAISE(ABORT, 'plan snapshots are append-only'); END;
-- +goose StatementEnd
