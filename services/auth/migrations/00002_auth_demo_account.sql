-- +goose Up
ALTER TABLE users ADD COLUMN email_verification_method text NOT NULL DEFAULT 'strict' CHECK (email_verification_method IN ('strict','demo'));
ALTER TABLE users ADD COLUMN verification_failures smallint NOT NULL DEFAULT 0 CHECK (verification_failures BETWEEN 0 AND 5);
ALTER TABLE users ADD COLUMN verification_window_started_at timestamptz;
CREATE TABLE consent_documents (
 singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
 document_version text NOT NULL CHECK (char_length(document_version) BETWEEN 1 AND 128),
 document_sha256 text NOT NULL CHECK (document_sha256 ~ '^[a-f0-9]{64}$'),
 document_text text NOT NULL
);
CREATE TABLE consent_evidence (
 id uuid PRIMARY KEY CHECK (uuid_extract_version(id) IS NOT DISTINCT FROM 4),
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 document_version text NOT NULL CHECK (char_length(document_version) BETWEEN 1 AND 128),
 document_sha256 text NOT NULL CHECK (document_sha256 ~ '^[a-f0-9]{64}$'),
 document_text text NOT NULL,
 accepted_at timestamptz NOT NULL,
 UNIQUE(user_id,document_version,document_sha256)
);
CREATE TABLE memberships (
 company_id uuid NOT NULL CHECK (uuid_extract_version(company_id) IS NOT DISTINCT FROM 7),
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 role text NOT NULL CHECK (role IN ('owner','financier')),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(company_id,user_id)
);
CREATE UNIQUE INDEX memberships_owner_idx ON memberships(company_id) WHERE role='owner';

-- +goose Down
DROP TABLE memberships;
DROP TABLE consent_evidence;
DROP TABLE consent_documents;
ALTER TABLE users DROP COLUMN verification_window_started_at;
ALTER TABLE users DROP COLUMN verification_failures;
ALTER TABLE users DROP COLUMN email_verification_method;
