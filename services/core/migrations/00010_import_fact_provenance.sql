-- +goose Up
ALTER TABLE core_transactions ADD COLUMN import_id uuid REFERENCES core_imports(id);
-- +goose Down
ALTER TABLE core_transactions DROP COLUMN import_id;
