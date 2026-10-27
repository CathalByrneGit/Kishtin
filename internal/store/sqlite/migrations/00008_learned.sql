-- +goose Up

-- When a manager last reset a dish's learned cook time: cooks before then
-- are not learned from.
CREATE TABLE learned_resets (
    dish_id  TEXT PRIMARY KEY REFERENCES dishes(id),
    reset_at INTEGER NOT NULL,
    reset_by TEXT NOT NULL
);

-- Whether an item's cook time was learned from past cooks rather than set
-- on the menu.
ALTER TABLE order_items ADD COLUMN cook_learned INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE order_items DROP COLUMN cook_learned;
DROP TABLE learned_resets;
