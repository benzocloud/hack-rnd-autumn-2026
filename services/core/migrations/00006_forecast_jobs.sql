-- +goose Up
-- Coverage is certified by an ingestion/preparation boundary, never by filling
-- absent transaction days with zeros. A later account revision needs new proof.
CREATE TABLE core_history_coverage (
    company_id uuid NOT NULL,
    account_id uuid NOT NULL,
    data_version bigint NOT NULL CHECK (data_version > 0),
    from_date date NOT NULL,
    through_date date NOT NULL,
    reference text NOT NULL CHECK (char_length(reference) BETWEEN 1 AND 2048 AND btrim(reference) <> ''),
    PRIMARY KEY (company_id, account_id, data_version),
    FOREIGN KEY (company_id, account_id) REFERENCES core_accounts(company_id, id),
    CHECK (from_date <= through_date)
);

CREATE TABLE core_forecast_jobs (
    company_id uuid NOT NULL,
    account_id uuid NOT NULL,
    id uuid NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    data_version bigint NOT NULL CHECK (data_version > 0),
    status text NOT NULL CHECK (status IN ('queued','running','succeeded','failed')),
    created_at timestamptz NOT NULL,
    completed_at timestamptz,
    lease_token uuid,
    lease_expires_at timestamptz,
    input jsonb NOT NULL CHECK (jsonb_typeof(input) = 'object'),
    result jsonb CHECK (jsonb_typeof(result) = 'object'),
    error_code text,
    request_id text,
    PRIMARY KEY (company_id, account_id, id),
    FOREIGN KEY (company_id, account_id) REFERENCES core_accounts(company_id, id),
    CHECK (completed_at IS NULL OR completed_at >= created_at),
    CHECK (
        (status='queued' AND lease_token IS NULL AND lease_expires_at IS NULL AND completed_at IS NULL AND result IS NULL AND error_code IS NULL AND request_id IS NULL) OR
        (status='running' AND lease_token IS NOT NULL AND lease_expires_at IS NOT NULL AND completed_at IS NULL AND result IS NULL AND error_code IS NULL AND request_id IS NULL) OR
        (status='succeeded' AND lease_token IS NULL AND lease_expires_at IS NULL AND completed_at IS NOT NULL AND result IS NOT NULL AND error_code IS NULL AND request_id IS NULL) OR
        (status='failed' AND lease_token IS NULL AND lease_expires_at IS NULL AND completed_at IS NOT NULL AND result IS NULL AND error_code IS NOT NULL AND error_code <> '' AND request_id IS NOT NULL AND request_id <> '')
    )
);

CREATE INDEX core_forecast_jobs_listing ON core_forecast_jobs(company_id, account_id, created_at DESC, id DESC);
CREATE INDEX core_forecast_jobs_pending ON core_forecast_jobs(created_at, id) WHERE status IN ('queued','running');

-- +goose Down
DROP TABLE core_forecast_jobs;
DROP TABLE core_history_coverage;
