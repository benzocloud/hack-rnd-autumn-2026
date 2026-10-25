-- +goose Up
CREATE TABLE invitations (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id) IS NOT DISTINCT FROM 4),
 company_id uuid NOT NULL CHECK (uuid_extract_version(company_id) IS NOT DISTINCT FROM 7),
 email text NOT NULL CHECK (char_length(email) BETWEEN 3 AND 254),
 company_name_snapshot text NOT NULL CHECK (char_length(company_name_snapshot) BETWEEN 1 AND 160),
 role text NOT NULL CHECK (role = 'financier'),
 status text NOT NULL CHECK (status IN ('pending','accepted','declined','revoked','expired')),
 version bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
 created_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 terminal_at timestamptz,
 created_by uuid NOT NULL REFERENCES users(id),
 create_idempotency_key text CHECK (create_idempotency_key IS NULL OR (octet_length(create_idempotency_key) BETWEEN 8 AND 128 AND create_idempotency_key ~ '^[A-Za-z0-9._:-]+$')),
 create_request_sha256 text CHECK (create_request_sha256 IS NULL OR create_request_sha256 ~ '^[a-f0-9]{64}$'),
 accept_idempotency_key text CHECK (accept_idempotency_key IS NULL OR (octet_length(accept_idempotency_key) BETWEEN 8 AND 128 AND accept_idempotency_key ~ '^[A-Za-z0-9._:-]+$')),
 accept_request_sha256 text CHECK (accept_request_sha256 IS NULL OR accept_request_sha256 ~ '^[a-f0-9]{64}$'),
 CHECK ((create_idempotency_key IS NULL) = (create_request_sha256 IS NULL)),
 CHECK ((accept_idempotency_key IS NULL) = (accept_request_sha256 IS NULL)),
 CHECK ((status = 'pending' AND terminal_at IS NULL) OR (status <> 'pending' AND terminal_at IS NOT NULL))
);
CREATE UNIQUE INDEX invitations_pending_company_email_idx ON invitations(company_id, email) WHERE status = 'pending';
CREATE UNIQUE INDEX invitations_create_idempotency_idx ON invitations(created_by, company_id, create_idempotency_key) WHERE create_idempotency_key IS NOT NULL;
CREATE INDEX invitations_company_created_idx ON invitations(company_id, created_at DESC, id DESC);
CREATE INDEX invitations_email_created_idx ON invitations(email, created_at DESC, id DESC);
CREATE INDEX invitations_terminal_cleanup_idx ON invitations(terminal_at) WHERE terminal_at IS NOT NULL;

-- +goose Down
DROP TABLE invitations;
