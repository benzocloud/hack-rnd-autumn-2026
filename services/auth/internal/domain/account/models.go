package account

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound    = errors.New("auth record not found")
	ErrUnavailable = errors.New("auth dependency unavailable")
)

type User struct {
	ID                                                               uuid.UUID
	Email, DisplayName, PasswordHash, State, EmailVerificationMethod string
	EmailVerified                                                    bool
	CredentialVersion                                                int64
	VerificationFailures                                             int
	VerificationWindowStartedAt                                      *time.Time
}

type SecurityContext struct {
	ID                   uuid.UUID
	Kind                 string
	TokenHash, CSRFNonce []byte
	CSRFKeyID            string
	CreatedAt, ExpiresAt time.Time
	RevokedAt            *time.Time
	UserID               uuid.UUID
	CredentialVersion    int64
	LastSeenAt           time.Time
}

type Challenge struct {
	ID, UserID            uuid.UUID
	CodeHMAC              []byte
	CodeKeyID, Purpose    string
	FailedAttempts        int
	CreatedAt, ExpiresAt  time.Time
	ConsumedAt, RevokedAt *time.Time
}

type ConsentDocument struct{ Text, Version, SHA256 string }
type ConsentEvidence struct {
	ID, UserID uuid.UUID
	Document   ConsentDocument
	AcceptedAt time.Time
}
type Membership struct {
	CompanyID, UserID uuid.UUID
	Role              string
	CreatedAt         time.Time
}
