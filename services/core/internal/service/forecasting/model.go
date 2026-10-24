package forecasting

import (
	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

type ModelContext struct {
	Name, Version, PreprocessingVersion string
}

type HistorySource string

const (
	Observed             HistorySource = "observed"
	VerifiedNoOperations HistorySource = "verified_no_operations"
)

// ModelDay carries verified coverage, not a zero-filled missing day.
type ModelDay struct {
	Date                            ledger.Date
	Inflow, Outflow, ClosingBalance money.Money
	Source                          HistorySource
}

// ModelInput is an application input, not FinancialSnapshot or a REST schema.
// Its 90-day coverage must be verified by the future orchestrator.
type ModelInput struct {
	SnapshotID     uuid.UUID
	Scope          ledger.Scope
	DataVersion    int64
	Cutoff         ledger.Date
	OpeningBalance money.Money
	History        []ModelDay
	KnownPayments  []payments.ExpectedPayment
	HorizonDays    int
	Model          ModelContext
}

type ModelForecastDay struct {
	Date                              ledger.Date
	Inflow, Outflow, ReferenceBalance money.Money
}

// ModelResult is a history-only mathematical projection, not an authoritative
// persisted forecast or a calibrated probability of insolvency.
type ModelResult struct {
	SnapshotID                        uuid.UUID
	Scope                             ledger.Scope
	DataVersion                       int64
	Cutoff                            ledger.Date
	Model                             ModelContext
	Days                              []ModelForecastDay
	FirstNegativeDate                 *ledger.Date
	HistoryOnlyIncludesRecurringFlows bool
	Warnings                          []string
}
