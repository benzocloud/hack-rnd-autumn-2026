package ledger

import (
	"time"

	"github.com/google/uuid"
)

// StoredTransaction retains persisted provenance alongside the financial fact.
// Nil CreatedAt means unknown legacy metadata, never the current read time.
type StoredTransaction struct {
	Transaction Transaction
	CreatedAt   *time.Time
	ImportID    uuid.UUID
}
