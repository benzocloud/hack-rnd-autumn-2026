package imports

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

const (
	MaxBytes = 100 << 20
	MaxChunk = 1 << 20
	MaxRows  = 300000
	Lease    = 5 * time.Minute
)

var (
	ErrInvalid       = errors.New("invalid import input")
	ErrCursor        = errors.New("invalid import cursor")
	ErrNotFound      = errors.New("import not found")
	ErrConflict      = errors.New("import idempotency conflict")
	ErrState         = errors.New("import state conflict")
	ErrQuota         = errors.New("import quota exceeded")
	ErrTooLarge      = errors.New("import payload too large")
	ErrUnavailable   = errors.New("import unavailable")
	ErrPreviewStale  = errors.New("import preview stale")
	ErrWarnings      = errors.New("import warnings not acknowledged")
	ErrSource        = errors.New("import factual source conflict")
	ErrSourceBinding = errors.New("import source company conflict")
)

type Metadata struct {
	Scope                                                    ledger.Scope
	ActorID, Key, Adapter, SourceCompanyID, Currency, Digest string
	ByteSize                                                 int64
}

type Record struct {
	ID                                                                  uuid.UUID
	Metadata                                                            Metadata
	Status                                                              string
	Version                                                             int64
	CreatedAt, UpdatedAt, AcceptedAt, LeaseUntil, ExpiresAt, TerminalAt time.Time
	Token                                                               uuid.UUID
	Received                                                            int64
	ChunkCount                                                          int
	RawDeleted                                                          bool
	Preview                                                             *Preview
	ErrorCode                                                           string
	AppliedDataVersion                                                  int64
	Coverage                                                            *HistoryCoverage
}

type HistoryCoverage struct {
	Kind, FromDate, ThroughDate, Reference string
	DataVersion                            int64
	Limitations                            []string
}

type Accepted struct{ ID uuid.UUID }

type Issue struct {
	Code, Severity, Field string
	Row                   int64
	ExternalIDs           []string
}

type Balance struct {
	Date  string
	Minor int64
}
type Binding struct{ Mode, System, CompanyID string }
type Preview struct {
	PeriodStart, PeriodEnd                                              string
	Rows, Excluded, NewTransactions, AlreadyPresent, Blockers, Warnings int64
	BaseVersion                                                         int64
	Source                                                              Binding
	WouldBind                                                           *Binding
	Coverage                                                            *HistoryCoverage
	Balances                                                            []Balance
	Issues                                                              []Issue
}

// NormalizedRow is durable staging, never a command to mutate Ledger.
type NormalizedRow struct {
	Row                                             int64
	ExternalID, Date, Direction, INN, Name, Purpose string
	Amount, Balance                                 int64
}

func SameEconomics(a, b NormalizedRow) bool {
	a.Row, b.Row = 0, 0
	a.Balance, b.Balance = 0, 0
	return a == b
}
