-- +goose Up
CREATE TABLE core_financial_snapshots (
    company_id uuid NOT NULL,
    account_id uuid NOT NULL,
    id uuid NOT NULL,
    data_version bigint NOT NULL CHECK (data_version > 0),
    cutoff date NOT NULL,
    created_at timestamptz NOT NULL,
    payload_version integer NOT NULL CHECK (payload_version = 1),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    PRIMARY KEY (company_id, account_id, id),
    FOREIGN KEY (company_id, account_id) REFERENCES core_accounts(company_id, id)
);

-- Application storage exposes INSERT/SELECT only; corrections create new snapshots.
-- +goose Down
DROP TABLE core_financial_snapshots;
