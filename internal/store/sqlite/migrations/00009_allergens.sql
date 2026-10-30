-- +goose Up

-- The 14 regulated allergens each dish contains, as a JSON list of codes,
-- and whether a manager has checked them: an unchecked dish with none
-- listed is unknown, not allergen-free.
ALTER TABLE dishes ADD COLUMN allergens TEXT NOT NULL DEFAULT '[]';
ALTER TABLE dishes ADD COLUMN allergens_checked INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE dishes DROP COLUMN allergens_checked;
ALTER TABLE dishes DROP COLUMN allergens;
