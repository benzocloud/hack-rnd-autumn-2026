-- +goose Up
CREATE TYPE core_currency AS ENUM ('RUB');

CREATE TABLE core_companies (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid)
);

CREATE TABLE core_accounts (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    company_id uuid NOT NULL UNIQUE REFERENCES core_companies(id),
    currency core_currency NOT NULL,
    data_version bigint NOT NULL CHECK (data_version > 0),
    factual_kind text CHECK (factual_kind IN ('manual', 'imported')),
    source_system text,
    source_company_id text,
    UNIQUE (company_id, id),
    CHECK ((
        (factual_kind IS NULL AND source_system IS NULL AND source_company_id IS NULL) OR
        (factual_kind = 'manual' AND source_system IS NULL AND source_company_id IS NULL) OR
        (factual_kind = 'imported' AND source_system IS NOT NULL AND source_system <> '' AND
         source_company_id IS NOT NULL AND source_company_id <> '')
    ) IS TRUE)
);

CREATE TABLE core_source_bindings (
    company_id uuid NOT NULL,
    account_id uuid NOT NULL,
    source_system text NOT NULL CHECK (source_system <> ''),
    source_company_id text NOT NULL CHECK (source_company_id <> ''),
    PRIMARY KEY (company_id, account_id, source_system),
    UNIQUE (company_id, account_id, source_system, source_company_id),
    FOREIGN KEY (company_id, account_id) REFERENCES core_accounts(company_id, id)
);

ALTER TABLE core_accounts ADD CONSTRAINT core_accounts_source_binding_fkey
    FOREIGN KEY (company_id, id, source_system, source_company_id)
    REFERENCES core_source_bindings(company_id, account_id, source_system, source_company_id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE core_categories (
    company_id uuid NOT NULL REFERENCES core_companies(id),
    id uuid NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    name text NOT NULL CHECK (name <> ''),
    PRIMARY KEY (company_id, id)
);

CREATE TABLE core_transactions (
    company_id uuid NOT NULL,
    account_id uuid NOT NULL,
    id uuid NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    operation_date date NOT NULL,
    direction text NOT NULL CHECK (direction IN ('inflow', 'outflow')),
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    currency core_currency NOT NULL,
    source_kind text NOT NULL CHECK (source_kind IN ('manual', 'imported')),
    source_system text,
    source_company_id text,
    external_id text,
    imported_at timestamptz,
    category_id uuid,
    counterparty_inn text NOT NULL,
    counterparty_name text NOT NULL,
    purpose text NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    created_data_version bigint NOT NULL CHECK (created_data_version > 0),
    PRIMARY KEY (company_id, account_id, id),
    FOREIGN KEY (company_id, account_id) REFERENCES core_accounts(company_id, id),
    FOREIGN KEY (company_id, category_id) REFERENCES core_categories(company_id, id),
    FOREIGN KEY (company_id, account_id, source_system, source_company_id)
        REFERENCES core_source_bindings(company_id, account_id, source_system, source_company_id),
    CHECK (
        (source_kind = 'manual' AND source_system IS NULL AND source_company_id IS NULL AND
         external_id IS NULL AND imported_at IS NULL) OR
        (source_kind = 'imported' AND source_system IS NOT NULL AND source_system <> '' AND
         source_company_id IS NOT NULL AND source_company_id <> '' AND
         external_id IS NOT NULL AND external_id <> '' AND imported_at IS NOT NULL)
    )
);
CREATE UNIQUE INDEX core_transactions_external_identity_key
    ON core_transactions(company_id, account_id, source_system, source_company_id, external_id)
    WHERE source_kind = 'imported';
CREATE INDEX core_transactions_by_date ON core_transactions(company_id, account_id, operation_date, id);

CREATE TABLE core_balances (
    company_id uuid NOT NULL,
    account_id uuid NOT NULL,
    balance_date date NOT NULL,
    amount_minor bigint NOT NULL,
    currency core_currency NOT NULL,
    source_kind text NOT NULL CHECK (source_kind IN ('manual', 'statement_eod')),
    source_system text,
    source_company_id text,
    recorded_at timestamptz NOT NULL,
    note text NOT NULL,
    created_data_version bigint NOT NULL CHECK (created_data_version > 0),
    PRIMARY KEY (company_id, account_id, balance_date),
    FOREIGN KEY (company_id, account_id) REFERENCES core_accounts(company_id, id),
    FOREIGN KEY (company_id, account_id, source_system, source_company_id)
        REFERENCES core_source_bindings(company_id, account_id, source_system, source_company_id),
    CHECK (
        (source_kind = 'manual' AND source_system IS NULL AND source_company_id IS NULL) OR
        (source_kind = 'statement_eod' AND source_system IS NOT NULL AND source_system <> '' AND
         source_company_id IS NOT NULL AND source_company_id <> '')
    )
);

-- No account FK here: reservation precedes the account lock. Taking a KEY SHARE
-- account lock during reservation would introduce lock-upgrade deadlocks.
CREATE TABLE core_command_results (
    actor_id text NOT NULL CHECK (actor_id <> ''),
    company_id uuid NOT NULL,
    account_id uuid NOT NULL,
    operation text NOT NULL,
    resource_id text NOT NULL,
    idempotency_key text NOT NULL CHECK (idempotency_key <> ''),
    digest bytea NOT NULL CHECK (octet_length(digest) = 32),
    result jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '24 hours',
    PRIMARY KEY (actor_id, company_id, account_id, operation, resource_id, idempotency_key)
);
CREATE INDEX core_command_results_expiry ON core_command_results(expires_at);

-- +goose Down
DROP TABLE core_command_results;
DROP TABLE core_balances;
DROP TABLE core_transactions;
DROP TABLE core_categories;
ALTER TABLE core_accounts DROP CONSTRAINT core_accounts_source_binding_fkey;
DROP TABLE core_source_bindings;
DROP TABLE core_accounts;
DROP TABLE core_companies;
DROP TYPE core_currency;
