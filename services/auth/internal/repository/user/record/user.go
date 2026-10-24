package record

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID                uuid.UUID `db:"id"`
	Email             string    `db:"email"`
	DisplayName       string    `db:"display_name"`
	EmailVerified     bool      `db:"email_verified"`
	CredentialVersion int64     `db:"credential_version"`
	State             string    `db:"state"`
	CreatedAt         time.Time `db:"created_at"`
}
