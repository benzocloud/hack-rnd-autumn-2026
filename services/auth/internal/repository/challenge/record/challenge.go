package record

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Challenge struct {
	ID             uuid.UUID    `db:"id"`
	UserID         uuid.UUID    `db:"user_id"`
	Purpose        string       `db:"purpose"`
	CodeHMAC       []byte       `db:"code_hmac"`
	CodeKeyID      string       `db:"code_key_id"`
	FailedAttempts int          `db:"failed_attempts"`
	CreatedAt      time.Time    `db:"created_at"`
	ExpiresAt      time.Time    `db:"expires_at"`
	ConsumedAt     sql.NullTime `db:"consumed_at"`
	RevokedAt      sql.NullTime `db:"revoked_at"`
}
