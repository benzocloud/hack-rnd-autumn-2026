package imports

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/imports"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type store interface {
	LockQuota(context.Context) error
	FindKey(context.Context, domain.Metadata) (domain.Record, bool, error)
	Usage(context.Context, ledger.Scope) (accountBytes, globalBytes int64, active int, err error)
	Insert(context.Context, domain.Record) error
	Read(context.Context, ledger.Scope, uuid.UUID) (domain.Record, error)
	Save(context.Context, domain.Record) error
	AppendChunk(context.Context, uuid.UUID, int, []byte) error
	Chunk(context.Context, uuid.UUID, int) ([]byte, error)
	Claim(context.Context, uuid.UUID, time.Time) (domain.Record, bool, error)
	Account(context.Context, ledger.Scope) (int64, domain.Binding, error)
	AddRows(context.Context, domain.Record, []domain.NormalizedRow) error
	AddIssues(context.Context, uuid.UUID, []domain.Issue) error
	Summary(context.Context, domain.Record) (domain.Preview, error)
	List(context.Context, ledger.Scope, int, time.Time, uuid.UUID) ([]domain.Record, error)
	Issues(context.Context, uuid.UUID, int64, int) ([]domain.Issue, []int64, error)
	Cleanup(context.Context, time.Time) error
}

// Work runs synchronously in an app-owned transaction. Network reads and CSV
// parsing happen between callbacks; repositories never escape Work.
type Work func(context.Context, store) error

// commitStore extends durable staging only for the atomic application use case.
type commitStore interface {
	store
	SeedCategories(context.Context, []ledger.CategoryDefinition) error
	CommitConflicts(context.Context, domain.Record) (bool, error)
	CountNew(context.Context, domain.Record) (int64, int64, error)
	BindSource(context.Context, ledger.Scope, string) error
	SetRevision(context.Context, ledger.Scope, int64) error
	StagedRows(context.Context, uuid.UUID, int64, int) ([]domain.NormalizedRow, error)
	InsertFacts(context.Context, []ledger.Transaction, int64, uuid.UUID) error
	InsertBalance(context.Context, ledger.BalanceObservation, int64) error
	VerifyCoverage(context.Context, domain.Record, ledger.Date, ledger.Date, ledger.Date, int64, []int64) (bool, error)
	InsertCoverage(context.Context, ledger.Scope, domain.HistoryCoverage, uuid.UUID, string) error
	CoverageExists(context.Context, ledger.Scope, int64) (bool, error)
}
type commandStore interface {
	Reserve(context.Context, ledger.Scope, string, string, string, string, []byte) ([]byte, bool, error)
	Complete(context.Context, ledger.Scope, string, string, string, string, []byte) error
}

// CommitWork borrows Tx-bound repositories; service owns orchestration/receipt.
type CommitWork func(context.Context, commitStore, commandStore) error
