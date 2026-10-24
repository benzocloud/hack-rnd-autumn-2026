package forecasting

import (
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

// Job carries immutable input/result payloads and durable lifecycle metadata.
// The application owns their versioned codecs; storage treats them as opaque JSON.
type Job struct {
	ID          uuid.UUID
	Scope       ledger.Scope
	DataVersion int64
	Status      string
	CreatedAt   time.Time
	CompletedAt *time.Time
	LeaseToken  uuid.UUID
	Input       []byte
	Result      []byte
	ErrorCode   string
	RequestID   string
}

// JobCoverage records the origin of a complete factual interval at one revision.
// Missing coverage must never be interpreted as verified zero activity.
type JobCoverage struct {
	From, Through ledger.Date
	Reference     string
}
