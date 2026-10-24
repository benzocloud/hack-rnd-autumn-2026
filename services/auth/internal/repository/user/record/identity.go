package record

import (
	"database/sql"

	"github.com/google/uuid"
)

type Identity struct {
	ID                          uuid.UUID    `db:"id"`
	Email                       string       `db:"email"`
	DisplayName                 string       `db:"display_name"`
	PasswordHash                string       `db:"password_hash"`
	State                       string       `db:"state"`
	EmailVerified               bool         `db:"email_verified"`
	CredentialVersion           int64        `db:"credential_version"`
	EmailVerificationMethod     string       `db:"email_verification_method"`
	VerificationFailures        int          `db:"verification_failures"`
	VerificationWindowStartedAt sql.NullTime `db:"verification_window_started_at"`
}
