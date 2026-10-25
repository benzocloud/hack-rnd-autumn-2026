-- +goose Up
CREATE TABLE core_payment_reconciliation_groups (
 company_id uuid NOT NULL,
 account_id uuid NOT NULL,
 id uuid NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
 version bigint NOT NULL CHECK (version > 0),
 status text NOT NULL CHECK (status IN ('active','revoked')),
 cutoff date NOT NULL,
 horizon_start date NOT NULL,
 horizon_end date NOT NULL,
 confirmed_by text NOT NULL CHECK (confirmed_by <> ''),
 confirmed_at timestamptz NOT NULL,
 additional boolean NOT NULL,
 operation_refs jsonb NOT NULL,
 PRIMARY KEY(company_id,account_id,id),
 FOREIGN KEY(company_id,account_id) REFERENCES core_accounts(company_id,id),
 CHECK(horizon_start=cutoff+1 AND horizon_end=cutoff+30),
 CHECK(jsonb_typeof(operation_refs)='array'),
 CHECK((additional AND jsonb_array_length(operation_refs)=0) OR (NOT additional AND jsonb_array_length(operation_refs)>0))
);
ALTER TABLE core_expected_payments ADD COLUMN reconciliation_group_id uuid;
ALTER TABLE core_expected_payments ADD CONSTRAINT core_payments_reconciliation_group_fk FOREIGN KEY(company_id,account_id,reconciliation_group_id) REFERENCES core_payment_reconciliation_groups(company_id,account_id,id);
CREATE INDEX core_reconciliation_by_scope ON core_payment_reconciliation_groups(company_id,account_id,id);
-- +goose Down
ALTER TABLE core_expected_payments DROP COLUMN reconciliation_group_id;
DROP TABLE core_payment_reconciliation_groups;
