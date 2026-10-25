-- +goose Up
-- One row serializes reservations/cleanup, not network transfer or parsing.
CREATE TABLE core_import_quota (id integer PRIMARY KEY CHECK(id=1));
INSERT INTO core_import_quota VALUES(1);
CREATE TABLE core_imports (
    id uuid PRIMARY KEY,
    company_id uuid NOT NULL,
    account_id uuid NOT NULL,
    actor_id text NOT NULL,
    idempotency_key text NOT NULL,
    status text NOT NULL CHECK(status IN ('receiving','queued','processing','preview_ready','applied','failed')),
    reserved_bytes bigint NOT NULL CHECK(reserved_bytes BETWEEN 1 AND 104857600),
    raw_deleted boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    lease_until timestamptz,
    expires_at timestamptz,
    terminal_at timestamptz,
    token uuid,
    payload jsonb NOT NULL,
    UNIQUE(company_id,account_id,actor_id,idempotency_key),
    FOREIGN KEY(company_id,account_id) REFERENCES core_accounts(company_id,id)
);
CREATE INDEX core_imports_listing ON core_imports(company_id,account_id,created_at DESC,id DESC) WHERE status<>'receiving';
CREATE INDEX core_imports_work ON core_imports(created_at,id) WHERE status IN ('queued','processing','receiving');
CREATE TABLE core_import_chunks (
    import_id uuid NOT NULL REFERENCES core_imports(id) ON DELETE CASCADE,
    ordinal integer NOT NULL CHECK(ordinal>=0),
    contents bytea NOT NULL CHECK(octet_length(contents) BETWEEN 1 AND 1048576),
    PRIMARY KEY(import_id,ordinal)
);
CREATE TABLE core_import_rows (
    import_id uuid NOT NULL REFERENCES core_imports(id) ON DELETE CASCADE,
    row_number bigint NOT NULL CHECK(row_number>=2),
    external_id text NOT NULL,
    operation_date date NOT NULL,
    amount_minor bigint NOT NULL,
    direction text NOT NULL,
    balance_minor bigint NOT NULL,
    content_hash text NOT NULL,
    economy jsonb NOT NULL,
    payload jsonb NOT NULL,
    PRIMARY KEY(import_id,row_number)
);
CREATE INDEX core_import_rows_identity ON core_import_rows(import_id,external_id);
CREATE INDEX core_import_rows_content ON core_import_rows(import_id,content_hash);
CREATE TABLE core_import_issues (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    import_id uuid NOT NULL REFERENCES core_imports(id) ON DELETE CASCADE,
    payload jsonb NOT NULL
);
CREATE INDEX core_import_issues_list ON core_import_issues(import_id,id);
-- +goose Down
DROP TABLE core_import_issues;
DROP TABLE core_import_rows;
DROP TABLE core_import_chunks;
DROP TABLE core_imports;
DROP TABLE core_import_quota;
