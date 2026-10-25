package record

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Invitation struct {
	ID                   uuid.UUID      `db:"id"`
	CompanyID            uuid.UUID      `db:"company_id"`
	Email                string         `db:"email"`
	CompanyNameSnapshot  string         `db:"company_name_snapshot"`
	Role                 string         `db:"role"`
	Status               string         `db:"status"`
	Version              int64          `db:"version"`
	CreatedAt            time.Time      `db:"created_at"`
	ExpiresAt            time.Time      `db:"expires_at"`
	TerminalAt           sql.NullTime   `db:"terminal_at"`
	CreatedBy            uuid.UUID      `db:"created_by"`
	CreateIdempotencyKey sql.NullString `db:"create_idempotency_key"`
	CreateRequestSHA256  sql.NullString `db:"create_request_sha256"`
	AcceptIdempotencyKey sql.NullString `db:"accept_idempotency_key"`
	AcceptRequestSHA256  sql.NullString `db:"accept_request_sha256"`
}
