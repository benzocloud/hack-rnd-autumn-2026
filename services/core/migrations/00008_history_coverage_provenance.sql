-- +goose Up
-- NULL provenance means legacy coverage, never a newly certified fixture.
ALTER TABLE core_history_coverage
    ADD COLUMN kind text CHECK (kind IN ('certified','dataset_fixture_assumption')),
    ADD COLUMN origin_data_version bigint CHECK (origin_data_version > 0),
    ADD COLUMN artifact_sha256 text CHECK (artifact_sha256 ~ '^[0-9a-f]{64}$'),
    ADD COLUMN import_id uuid REFERENCES core_imports(id),
    ADD COLUMN limitations jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(limitations)='array');
ALTER TABLE core_history_coverage ADD CONSTRAINT core_history_coverage_fixture_provenance CHECK (
    kind IS DISTINCT FROM 'dataset_fixture_assumption' OR
    (origin_data_version IS NOT NULL AND artifact_sha256 IS NOT NULL AND import_id IS NOT NULL
     AND limitations @> '["dataset_coverage_assumed_not_bank_certified"]'::jsonb)
);
-- +goose Down
ALTER TABLE core_history_coverage DROP CONSTRAINT core_history_coverage_fixture_provenance;
ALTER TABLE core_history_coverage DROP COLUMN limitations, DROP COLUMN import_id,
    DROP COLUMN artifact_sha256, DROP COLUMN origin_data_version, DROP COLUMN kind;
