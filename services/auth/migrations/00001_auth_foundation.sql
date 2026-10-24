-- +goose Up
CREATE TABLE users (
    id uuid PRIMARY KEY CHECK (uuid_extract_version(id) IS NOT DISTINCT FROM 4),
    email text COLLATE "C" NOT NULL UNIQUE,
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 120),
    email_verified boolean NOT NULL DEFAULT false,
    credential_version bigint NOT NULL DEFAULT 1 CHECK (credential_version > 0),
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (octet_length(email) BETWEEN 3 AND 254 AND email = btrim(email)
           AND email = lower(email COLLATE "C") AND email !~ '[^!-~]'
           AND position('@' IN email) > 1)
);

CREATE TABLE credentials (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    password_hash text NOT NULL CHECK (octet_length(password_hash) BETWEEN 16 AND 1024 AND password_hash LIKE '$argon2id$%'),
    changed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE security_contexts (
    id uuid PRIMARY KEY CHECK (uuid_extract_version(id) IS NOT DISTINCT FROM 4),
    kind text NOT NULL CHECK (kind IN ('pre_auth', 'session')),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    csrf_nonce bytea NOT NULL CHECK (octet_length(csrf_nonce) = 32),
    csrf_key_id text NOT NULL CHECK (char_length(csrf_key_id) BETWEEN 1 AND 128),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    UNIQUE (id, kind),
    CHECK (expires_at > created_at),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);
CREATE INDEX security_contexts_expiry_idx ON security_contexts (expires_at);

CREATE TABLE sessions (
    context_id uuid PRIMARY KEY,
    context_kind text NOT NULL DEFAULT 'session' CHECK (context_kind = 'session'),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_version bigint NOT NULL CHECK (credential_version > 0),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (context_id, context_kind) REFERENCES security_contexts(id, kind) ON DELETE CASCADE
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE verification_challenges (
    id uuid PRIMARY KEY CHECK (uuid_extract_version(id) IS NOT DISTINCT FROM 4),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose text NOT NULL DEFAULT 'email_verification' CHECK (purpose = 'email_verification'),
    code_hmac bytea NOT NULL CHECK (octet_length(code_hmac) = 32),
    code_key_id text NOT NULL CHECK (char_length(code_key_id) BETWEEN 1 AND 128),
    failed_attempts smallint NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    revoked_at timestamptz,
    CHECK (expires_at > created_at AND expires_at <= created_at + interval '10 minutes'),
    CHECK (consumed_at IS NULL OR (consumed_at >= created_at AND consumed_at < expires_at)),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);
CREATE UNIQUE INDEX verification_challenges_pending_idx ON verification_challenges(user_id)
    WHERE consumed_at IS NULL AND revoked_at IS NULL AND failed_attempts < 5;
CREATE INDEX verification_challenges_expiry_idx ON verification_challenges (expires_at);

CREATE TABLE reset_tokens (
    id uuid PRIMARY KEY CHECK (uuid_extract_version(id) IS NOT DISTINCT FROM 4),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose text NOT NULL DEFAULT 'password_reset' CHECK (purpose = 'password_reset'),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    revoked_at timestamptz,
    CHECK (expires_at > created_at AND expires_at <= created_at + interval '15 minutes'),
    CHECK (consumed_at IS NULL OR (consumed_at >= created_at AND consumed_at < expires_at)),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);
CREATE INDEX reset_tokens_user_idx ON reset_tokens (user_id);
CREATE INDEX reset_tokens_expiry_idx ON reset_tokens (expires_at);

-- +goose Down
DROP TABLE reset_tokens;
DROP TABLE verification_challenges;
DROP TABLE sessions;
DROP TABLE security_contexts;
DROP TABLE credentials;
DROP TABLE users;
