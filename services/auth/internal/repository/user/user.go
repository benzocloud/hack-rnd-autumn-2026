package user

import (
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/user/record"
)

// User is a storage projection, not a public API or use-case DTO.
type User struct {
	ID                uuid.UUID
	Email             string
	DisplayName       string
	EmailVerified     bool
	CredentialVersion int64
	State             string
	CreatedAt         time.Time
}

func fromRecord(row record.User) User {
	return User(row)
}
