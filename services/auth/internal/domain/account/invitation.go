package account

import (
	"time"

	"github.com/google/uuid"
)

type Invitation struct {
	ID                   uuid.UUID
	CompanyID            uuid.UUID
	Email                string
	CompanyNameSnapshot  string
	Role                 string
	Status               string
	Version              int64
	CreatedAt, ExpiresAt time.Time
	TerminalAt           *time.Time
	CreatedBy            uuid.UUID
	CreateIdempotencyKey *string
	CreateRequestSHA256  *string
	AcceptIdempotencyKey *string
	AcceptRequestSHA256  *string
}

type Member struct {
	UserID      uuid.UUID
	Email       string
	DisplayName string
	Role        string
	JoinedAt    time.Time
}
