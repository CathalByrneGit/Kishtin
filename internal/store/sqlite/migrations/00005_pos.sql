-- +goose Up

-- Point-of-sale integration: which till catalog entries make which dishes,
-- the till orders Kishtin has seen (deduped on the till's order ID and
-- version), and where polling has got to. The access token is never
-- stored here.
CREATE TABLE pos_mappings (
    pos         TEXT NOT NULL,
    external_id TEXT NOT NULL,
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL,
    dish_id     TEXT NOT NULL DEFAULT '',
    modifier    TEXT NOT NULL DEFAULT '',
    ignore      INTEGER NOT NULL DEFAULT 0,
    updated_at  INTEGER NOT NULL,
    updated_by  TEXT NOT NULL,
    PRIMARY KEY (pos, external_id)
);

CREATE TABLE pos_orders (
    pos         TEXT NOT NULL,
    external_id TEXT NOT NULL,
    version     INTEGER NOT NULL,
    state       TEXT NOT NULL,
    order_id    TEXT NOT NULL DEFAULT '',
    label       TEXT NOT NULL,
    added       TEXT NOT NULL,
    unmapped    TEXT NOT NULL,
    notices     TEXT NOT NULL,
    payload     TEXT NOT NULL,
    first_seen  INTEGER NOT NULL,
    last_seen   INTEGER NOT NULL,
    PRIMARY KEY (pos, external_id)
);
CREATE INDEX pos_orders_seen ON pos_orders(pos, last_seen);

CREATE TABLE pos_state (
    pos          TEXT PRIMARY KEY,
    since        INTEGER NOT NULL,
    last_poll_at INTEGER,
    last_ok_at   INTEGER,
    last_error   TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE pos_state;
DROP TABLE pos_orders;
DROP TABLE pos_mappings;
