-- +goose Up
ALTER TABLE core_categories ADD COLUMN direction text
    CHECK (direction IN ('inflow','outflow'));
ALTER TABLE core_categories ADD COLUMN semantic_kind text NOT NULL DEFAULT 'other'
    CHECK (semantic_kind IN ('operating_expense','sales_receipt','tax','financing','own_transfer','other'));
ALTER TABLE core_categories ADD COLUMN catalogue_code text;
CREATE UNIQUE INDEX core_categories_catalogue ON core_categories(company_id,catalogue_code)
    WHERE catalogue_code IS NOT NULL;

-- +goose Down
DROP INDEX core_categories_catalogue;
ALTER TABLE core_categories DROP COLUMN catalogue_code;
ALTER TABLE core_categories DROP COLUMN semantic_kind;
ALTER TABLE core_categories DROP COLUMN direction;
