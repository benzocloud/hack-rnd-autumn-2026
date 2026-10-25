-- +goose Up
-- The one-time demo seed inserted coverage before kind existed. Presence of
-- that exact fixture was the certification. Other NULL rows stay unproven.
UPDATE core_history_coverage
SET kind = 'certified'
WHERE kind IS NULL
  AND origin_data_version IS NULL
  AND artifact_sha256 IS NULL
  AND import_id IS NULL
  AND reference = 'Complete synthetic demo fixture: 180 known operations and explicit zero-missing source days';
-- +goose Down
UPDATE core_history_coverage
SET kind = NULL
WHERE kind = 'certified'
  AND origin_data_version IS NULL
  AND artifact_sha256 IS NULL
  AND import_id IS NULL
  AND reference = 'Complete synthetic demo fixture: 180 known operations and explicit zero-missing source days';
