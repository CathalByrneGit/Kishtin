-- +goose Up
-- A check kept on a tablet while Kishtin could not be reached is sent later
-- with an ID the tablet made up. If the tablet sends it twice (it never
-- heard back the first time), the second is answered with the record
-- already saved, so the check is saved once.
CREATE TABLE haccp_client_ids (
    client_id TEXT PRIMARY KEY,
    record_id TEXT NOT NULL,
    at        INTEGER NOT NULL
);

-- +goose Down
DROP TABLE haccp_client_ids;
