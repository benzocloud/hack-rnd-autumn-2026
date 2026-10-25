package record

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Command struct {
	ID             uuid.UUID      `db:"id"`
	CompanyID      uuid.UUID      `db:"company_id"`
	InitiatorID    uuid.UUID      `db:"initiator_id"`
	Key            string         `db:"idempotency_key"`
	Name           string         `db:"name"`
	INN            sql.NullString `db:"inn"`
	Currency       string         `db:"currency"`
	Timezone       string         `db:"timezone"`
	RequestSHA256  string         `db:"request_sha256"`
	State          string         `db:"state"`
	CreatedAt      time.Time      `db:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at"`
	NextAttemptAt  time.Time      `db:"next_attempt_at"`
	Attempts       int64          `db:"attempts"`
	LeaseToken     uuid.NullUUID  `db:"lease_token"`
	LeaseExpiresAt *time.Time     `db:"lease_expires_at"`
	AccountID      uuid.NullUUID  `db:"account_id"`
	CommittedAt    *time.Time     `db:"committed_at"`
}
