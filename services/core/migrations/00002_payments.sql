-- +goose Up
CREATE TABLE core_expected_payments (
    company_id uuid NOT NULL,
    account_id uuid NOT NULL,
    id uuid NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    direction text NOT NULL CHECK (direction IN ('inflow', 'outflow')),
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    remaining_minor bigint NOT NULL CHECK (remaining_minor >= 0 AND remaining_minor <= amount_minor),
    currency core_currency NOT NULL,
    scheduled_date date NOT NULL,
    source_kind text NOT NULL CHECK (source_kind IN ('manual', 'imported')),
    source_system text,
    source_company_id text,
    external_id text,
    imported_at timestamptz,
    category_id uuid,
    counterparty_name text NOT NULL,
    description text NOT NULL,
    status text NOT NULL CHECK (status IN ('planned', 'partially_matched', 'matched', 'cancelled')),
    version bigint NOT NULL CHECK (version > 0),
    created_at timestamptz NOT NULL,
    created_data_version bigint NOT NULL CHECK (created_data_version > 0),
    PRIMARY KEY (company_id, account_id, id),
    FOREIGN KEY (company_id, account_id) REFERENCES core_accounts(company_id, id),
    FOREIGN KEY (company_id, category_id) REFERENCES core_categories(company_id, id),
    FOREIGN KEY (company_id, account_id, source_system, source_company_id)
        REFERENCES core_source_bindings(company_id, account_id, source_system, source_company_id),
    CHECK ((status = 'planned' AND remaining_minor = amount_minor) OR
           (status = 'partially_matched' AND remaining_minor > 0 AND remaining_minor < amount_minor) OR
           (status = 'matched' AND remaining_minor = 0) OR
           (status = 'cancelled' AND remaining_minor > 0)),
    CHECK (
        (source_kind = 'manual' AND source_system IS NULL AND source_company_id IS NULL AND
         external_id IS NULL AND imported_at IS NULL) OR
        (source_kind = 'imported' AND source_system IS NOT NULL AND source_system <> '' AND
         source_company_id IS NOT NULL AND source_company_id <> '' AND
         external_id IS NOT NULL AND external_id <> '' AND imported_at IS NOT NULL)
    )
);
CREATE UNIQUE INDEX core_payments_external_identity_key
    ON core_expected_payments(company_id, account_id, source_system, source_company_id, external_id)
    WHERE source_kind = 'imported';
CREATE INDEX core_payments_by_date ON core_expected_payments(company_id, account_id, scheduled_date, id);

CREATE TABLE core_payment_matches (
    company_id uuid NOT NULL,
    account_id uuid NOT NULL,
    id uuid NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    payment_id uuid NOT NULL,
    transaction_id uuid NOT NULL,
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    currency core_currency NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (company_id, account_id, id),
    FOREIGN KEY (company_id, account_id, payment_id) REFERENCES core_expected_payments(company_id, account_id, id),
    FOREIGN KEY (company_id, account_id, transaction_id) REFERENCES core_transactions(company_id, account_id, id)
);
CREATE INDEX core_matches_by_transaction ON core_payment_matches(company_id, account_id, transaction_id);
CREATE INDEX core_matches_by_payment ON core_payment_matches(company_id, account_id, payment_id);

-- +goose Down
DROP TABLE core_payment_matches;
DROP TABLE core_expected_payments;
