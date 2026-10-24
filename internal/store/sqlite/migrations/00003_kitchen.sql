-- +goose Up

-- Menu: stations, the equipment in them, and dishes. Rows are updated in
-- place; nothing is deleted, because orders refer to them. Closed value
-- lists (course, state, kind) are text checked in Go.
CREATE TABLE stations (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
    position   INTEGER NOT NULL DEFAULT 0,
    active     INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE equipment (
    id         TEXT PRIMARY KEY,
    station_id TEXT NOT NULL REFERENCES stations(id),
    name       TEXT NOT NULL,
    slots      INTEGER NOT NULL,
    active     INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (station_id, name COLLATE NOCASE)
);

CREATE TABLE dishes (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL UNIQUE COLLATE NOCASE,
    course         TEXT NOT NULL,
    station_id     TEXT NOT NULL REFERENCES stations(id),
    equipment_id   TEXT REFERENCES equipment(id),
    batch_capacity INTEGER NOT NULL DEFAULT 0,
    cook_seconds   INTEGER NOT NULL,
    rest_seconds   INTEGER NOT NULL,
    plate_seconds  INTEGER NOT NULL,
    hold_seconds   INTEGER NOT NULL,
    batchable      INTEGER NOT NULL DEFAULT 0,
    -- The modifiers as JSON, as written by Go.
    modifiers      TEXT NOT NULL DEFAULT '[]',
    food_category  TEXT NOT NULL DEFAULT '',
    available      INTEGER NOT NULL DEFAULT 1,
    active         INTEGER NOT NULL DEFAULT 1,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
);

-- Orders, their courses and items.
CREATE TABLE orders (
    id         TEXT PRIMARY KEY,
    number     INTEGER NOT NULL,
    kind       TEXT NOT NULL,
    table_name TEXT NOT NULL DEFAULT '',
    name       TEXT NOT NULL DEFAULT '',
    covers     INTEGER NOT NULL DEFAULT 0,
    notes      TEXT NOT NULL DEFAULT '',
    allergy    INTEGER NOT NULL DEFAULT 0,
    status     TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    closed_at  INTEGER
);
CREATE INDEX orders_status ON orders(status, created_at);
CREATE INDEX orders_created ON orders(created_at);

CREATE TABLE order_courses (
    order_id   TEXT NOT NULL REFERENCES orders(id),
    course     TEXT NOT NULL,
    state      TEXT NOT NULL,
    fired_at   INTEGER,
    target_at  INTEGER,
    target_set INTEGER NOT NULL DEFAULT 0,
    urgent     INTEGER NOT NULL DEFAULT 0,
    served_at  INTEGER,
    PRIMARY KEY (order_id, course)
);

CREATE TABLE order_items (
    id             TEXT PRIMARY KEY,
    order_id       TEXT NOT NULL REFERENCES orders(id),
    position       INTEGER NOT NULL,
    dish_id        TEXT NOT NULL REFERENCES dishes(id),
    dish_name      TEXT NOT NULL,
    course         TEXT NOT NULL,
    modifiers      TEXT NOT NULL DEFAULT '[]',
    notes          TEXT NOT NULL DEFAULT '',
    state          TEXT NOT NULL,
    station_id     TEXT NOT NULL,
    equipment_id   TEXT NOT NULL DEFAULT '',
    batch_capacity INTEGER NOT NULL DEFAULT 0,
    hold_seconds   INTEGER NOT NULL DEFAULT 0,
    batchable      INTEGER NOT NULL DEFAULT 0,
    cook_ms        INTEGER NOT NULL,
    rest_ms        INTEGER NOT NULL,
    plate_ms       INTEGER NOT NULL,
    state_at       INTEGER NOT NULL,
    started_at     INTEGER,
    ready_at       INTEGER,
    served_at      INTEGER,
    void_reason    TEXT NOT NULL DEFAULT '',
    UNIQUE (order_id, position)
);
CREATE INDEX order_items_order ON order_items(order_id);

-- Every change to an order: item state changes (the source of actual cook
-- times), fires, target changes and other expo overrides. Append-only.
CREATE TABLE order_events (
    id        TEXT PRIMARY KEY,
    order_id  TEXT NOT NULL REFERENCES orders(id),
    item_id   TEXT NOT NULL DEFAULT '',
    course    TEXT NOT NULL DEFAULT '',
    kind      TEXT NOT NULL,
    from_val  TEXT NOT NULL DEFAULT '',
    to_val    TEXT NOT NULL DEFAULT '',
    detail    TEXT NOT NULL DEFAULT '',
    at        INTEGER NOT NULL,
    by_user   TEXT NOT NULL,
    by_name   TEXT NOT NULL
);
CREATE INDEX order_events_order ON order_events(order_id, at);

-- +goose StatementBegin
CREATE TRIGGER order_events_no_update BEFORE UPDATE ON order_events
BEGIN SELECT RAISE(ABORT, 'order events are append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER order_events_no_delete BEFORE DELETE ON order_events
BEGIN SELECT RAISE(ABORT, 'order events are append-only'); END;
-- +goose StatementEnd

-- +goose Down
DROP TABLE order_events;
DROP TABLE order_items;
DROP TABLE order_courses;
DROP TABLE orders;
DROP TABLE dishes;
DROP TABLE equipment;
DROP TABLE stations;
