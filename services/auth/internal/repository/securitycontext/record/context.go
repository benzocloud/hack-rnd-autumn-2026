package record

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Context struct {
	ID                uuid.UUID    `db:"id"`
	Kind              string       `db:"kind"`
	TokenHash         []byte       `db:"token_hash"`
	CSRFNonce         []byte       `db:"csrf_nonce"`
	CSRFKeyID         string       `db:"csrf_key_id"`
	CreatedAt         time.Time    `db:"created_at"`
	ExpiresAt         time.Time    `db:"expires_at"`
	RevokedAt         sql.NullTime `db:"revoked_at"`
	UserID            uuid.UUID    `db:"user_id"`
	CredentialVersion int64        `db:"credential_version"`
	LastSeenAt        time.Time    `db:"last_seen_at"`
}
