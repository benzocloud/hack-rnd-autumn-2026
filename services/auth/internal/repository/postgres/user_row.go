package postgres

import (
	"time"

	"github.com/google/uuid"
)

// UserRecord is a cross-package storage projection, not a public or use-case DTO.
type UserRecord struct {
	ID                uuid.UUID
	Email             string
	DisplayName       string
	EmailVerified     bool
	CredentialVersion int64
	State             string
	CreatedAt         time.Time
}

type userRow struct {
	ID                uuid.UUID `db:"id"`
	Email             string    `db:"email"`
	DisplayName       string    `db:"display_name"`
	EmailVerified     bool      `db:"email_verified"`
	CredentialVersion int64     `db:"credential_version"`
	State             string    `db:"state"`
	CreatedAt         time.Time `db:"created_at"`
}

func (row userRow) record() UserRecord {
	return UserRecord(row)
}
