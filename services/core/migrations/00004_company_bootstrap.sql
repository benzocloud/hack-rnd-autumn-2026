-- +goose Up
-- Existing ID-only companies remain valid; no fictional profile is backfilled.
CREATE TABLE core_company_profiles (
    company_id uuid PRIMARY KEY REFERENCES core_companies(id),
    account_id uuid NOT NULL,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160 AND name = btrim(name)),
    inn text CHECK (inn ~ '^([0-9]{10}|[0-9]{12})$'),
    currency core_currency NOT NULL,
    timezone text NOT NULL CHECK (timezone = 'Europe/Moscow'),
    version bigint NOT NULL CHECK (version > 0),
    created_at timestamptz NOT NULL,
    UNIQUE (company_id, account_id),
    FOREIGN KEY (company_id, account_id) REFERENCES core_accounts(company_id, id)
);

CREATE TABLE core_company_bootstraps (
    command_id uuid PRIMARY KEY CHECK (command_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    company_id uuid NOT NULL UNIQUE,
    account_id uuid NOT NULL,
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160 AND name = btrim(name)),
    inn text CHECK (inn ~ '^([0-9]{10}|[0-9]{12})$'),
    currency core_currency NOT NULL,
    timezone text NOT NULL CHECK (timezone = 'Europe/Moscow'),
    committed_at timestamptz NOT NULL,
    -- Reserve the command before creating its company, in the SAME transaction.
    -- Incomplete reservations cannot commit or be read by another transaction.
    FOREIGN KEY (company_id, account_id) REFERENCES core_company_profiles(company_id, account_id)
        DEFERRABLE INITIALLY DEFERRED
);

-- +goose Down
DROP TABLE core_company_bootstraps;
DROP TABLE core_company_profiles;
