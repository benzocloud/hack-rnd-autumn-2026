package record

import (
	"time"

	"github.com/google/uuid"
)

type Membership struct {
	CompanyID uuid.UUID `db:"company_id"`
	UserID    uuid.UUID `db:"user_id"`
	Role      string    `db:"role"`
	CreatedAt time.Time `db:"created_at"`
}
