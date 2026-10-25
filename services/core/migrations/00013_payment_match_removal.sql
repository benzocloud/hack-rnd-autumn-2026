-- +goose Up
ALTER TABLE core_payment_matches ADD COLUMN removed_at timestamptz;
ALTER TABLE core_payment_matches ADD CONSTRAINT core_match_removal_time CHECK (removed_at IS NULL OR removed_at >= created_at);

-- +goose Down
-- Audit reversals cannot safely be downgraded into active allocations.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM core_payment_matches WHERE removed_at IS NOT NULL) THEN
  RAISE EXCEPTION 'cannot downgrade payment match audit while removed allocations exist';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE core_payment_matches DROP CONSTRAINT core_match_removal_time;
ALTER TABLE core_payment_matches DROP COLUMN removed_at;
