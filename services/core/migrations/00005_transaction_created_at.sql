-- +goose Up
-- Previous rows have no recorded creation time; leave them unknown.
ALTER TABLE core_transactions ADD COLUMN created_at timestamptz;
ALTER TABLE core_transactions ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP;

-- +goose Down
ALTER TABLE core_transactions DROP COLUMN created_at;
