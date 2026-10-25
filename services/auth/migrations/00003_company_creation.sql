-- +goose Up
-- No user FK: unresolved commands must survive initiator deletion for quarantine.
CREATE TABLE company_creation_commands (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id) IS NOT DISTINCT FROM 4),
 company_id uuid NOT NULL UNIQUE CHECK (uuid_extract_version(company_id) IS NOT DISTINCT FROM 7),
 initiator_id uuid NOT NULL CHECK (uuid_extract_version(initiator_id) IS NOT DISTINCT FROM 4),
 idempotency_key text NOT NULL CHECK (octet_length(idempotency_key) BETWEEN 8 AND 128 AND idempotency_key ~ '^[A-Za-z0-9._:-]+$'),
 name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160),
 inn text CHECK (inn ~ '^([0-9]{10}|[0-9]{12})$'),
 currency text NOT NULL CHECK (currency='RUB'),
 timezone text NOT NULL CHECK (timezone='Europe/Moscow'),
 request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[a-f0-9]{64}$'),
 state text NOT NULL CHECK (state IN ('PREPARED','COMPLETED','QUARANTINED')),
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 next_attempt_at timestamptz NOT NULL,
 attempts bigint NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 lease_token uuid,
 lease_expires_at timestamptz,
 account_id uuid CHECK (account_id IS NULL OR uuid_extract_version(account_id) IS NOT DISTINCT FROM 7),
 committed_at timestamptz,
 UNIQUE(initiator_id,idempotency_key),
 CHECK ((lease_token IS NULL) = (lease_expires_at IS NULL)),
 CHECK (state='PREPARED' OR lease_token IS NULL),
 CHECK ((state='COMPLETED') = (account_id IS NOT NULL AND committed_at IS NOT NULL)),
 CHECK (state='COMPLETED' OR (account_id IS NULL AND committed_at IS NULL)),
 CHECK (account_id IS NULL OR account_id <> company_id)
);
CREATE INDEX company_creation_recovery_idx ON company_creation_commands(next_attempt_at,created_at,id) WHERE state='PREPARED';

-- +goose Down
DROP TABLE company_creation_commands;
