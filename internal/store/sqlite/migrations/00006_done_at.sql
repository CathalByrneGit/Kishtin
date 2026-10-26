-- +goose Up

-- When each record's check was done: the time given for a check logged
-- later (reading.taken_at), otherwise when it was logged. Kept in its own
-- column so records can be found by the day of the check. It is derived
-- from the record, so it is not part of the record's hash.
ALTER TABLE haccp_records ADD COLUMN done_at INTEGER NOT NULL DEFAULT 0;

UPDATE haccp_records SET done_at = COALESCE(
    CAST(round((julianday(json_extract(reading, '$.taken_at')) - 2440587.5) * 86400000000) AS INTEGER),
    logged_at
);

CREATE INDEX haccp_records_done_at ON haccp_records (done_at);

-- +goose Down
DROP INDEX haccp_records_done_at;
ALTER TABLE haccp_records DROP COLUMN done_at;
