package forecasting

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

var (
	ErrHistory     = errors.New("certified complete forecast history unavailable")
	ErrHorizon     = errors.New("financial model horizon unsupported")
	ErrCursor      = errors.New("invalid forecast cursor")
	ErrJobConflict = errors.New("forecast idempotency conflict")
)

type jobStore interface {
	LockDataVersion(context.Context, ledger.Scope) (int64, error)
	ReadCoverage(context.Context, ledger.Scope, int64, ledger.Date, ledger.Date) (domain.JobCoverage, bool, error)
	InsertJob(context.Context, domain.Job) error
	ReadJob(context.Context, ledger.Scope, uuid.UUID) (domain.Job, error)
	ListJobs(context.Context, ledger.Scope, time.Time, uuid.UUID, int) ([]domain.Job, error)
	ClaimJob(context.Context, uuid.UUID) (domain.Job, bool, error)
	FinishJob(context.Context, domain.Job, uuid.UUID) error
}

type jobCommands interface {
	Reserve(context.Context, ledger.Scope, string, string, string, string, []byte) ([]byte, bool, error)
	Complete(context.Context, ledger.Scope, string, string, string, string, []byte) error
}

type jobSnapshots interface {
	ReadCategories(context.Context, ledger.Scope) ([]ledger.CategoryDefinition, error)
	ReadOpeningBalance(context.Context, ledger.Scope, ledger.Date) (domain.OpeningBalance, bool, error)
	ReadFacts(context.Context, ledger.Scope, ledger.Date) ([]domain.Fact, error)
	ReadBalanceHistory(context.Context, ledger.Scope, ledger.Date) ([]domain.OpeningBalance, error)
	ReadCalendar(context.Context, ledger.Scope) (domain.Calendar, error)
	InsertSnapshot(context.Context, domain.FinancialSnapshot) error
	ReadSnapshot(context.Context, ledger.Scope, uuid.UUID) (domain.FinancialSnapshot, error)
}

type ForecastWork func(context.Context, jobStore, jobSnapshots, jobCommands) error

type financialPredictor interface {
	PredictFinancial(context.Context, FinancialModelInput) (FinancialModelResult, error)
}

type jobs struct {
	withinTx func(context.Context, ForecastWork) error
	predict  financialPredictor
}

func NewJobs(work func(context.Context, ForecastWork) error, model financialPredictor) *jobs {
	return &jobs{withinTx: work, predict: model}
}

type CreateForecastCommand struct {
	Scope                   ledger.Scope
	ActorID, IdempotencyKey string
	Cutoff, Today           ledger.Date
	HorizonDays             int
}

type ForecastAccepted struct {
	ID          uuid.UUID
	PollAfterMS uint32
}

type SavedForecastDay struct {
	Date                     ledger.Date
	Inflow, Outflow, Balance int64
}

// SavedForecast uses explicit primitive money fields, never JSON-encodes the
// private internals of Money. Residual evidence stays auditable, not targetable.
type SavedForecast struct {
	Analysis                           *Analysis `json:",omitempty"`
	SchemaVersion                      int
	Days                               []SavedForecastDay
	FirstNegativeDate                  *ledger.Date
	ProbabilityReason                  string
	Components                         []FinancialComponent
	Evidence                           []HistoryEvidence
	Assumptions, Limitations, Warnings []string
}

type ForecastView struct {
	ScenarioExpenses     []ExpenseChange
	ScenarioID           *uuid.UUID
	ID                   uuid.UUID
	Scope                ledger.Scope
	Status               string
	CreatedAt            time.Time
	CompletedAt          *time.Time
	CurrentDataVersion   int64
	DataVersion          int64
	Cutoff               ledger.Date
	Snapshot             domain.FinancialSnapshot
	ModelContextID       uuid.UUID
	Coverage             domain.JobCoverage
	Assessment           *HistoryAssessment
	Model                ModelContext
	Result               *SavedForecast
	ErrorCode, RequestID string
}

type ListForecastCommand struct {
	Scope            ledger.Scope
	ActorID, Cursor  string
	Limit            int
	IncludeScenarios bool
}
type ListForecastResult struct {
	Items      []ForecastView
	NextCursor string
}
