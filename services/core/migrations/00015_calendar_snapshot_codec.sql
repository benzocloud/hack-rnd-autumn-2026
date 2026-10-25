-- +goose Up
ALTER TABLE core_financial_snapshots DROP CONSTRAINT core_financial_snapshots_payload_version_check;
ALTER TABLE core_financial_snapshots ADD CONSTRAINT core_financial_snapshots_payload_version_check CHECK (payload_version IN (1,2));

-- +goose Down
-- Existing v2 snapshots must be retained. Downgrade is deliberately refused
-- until operators have a decoder that preserves those immutable records.
ALTER TABLE core_financial_snapshots DROP CONSTRAINT core_financial_snapshots_payload_version_check;
ALTER TABLE core_financial_snapshots ADD CONSTRAINT core_financial_snapshots_payload_version_check CHECK (payload_version = 1);
