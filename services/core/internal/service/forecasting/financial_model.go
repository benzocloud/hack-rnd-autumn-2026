package forecasting

import (
	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

const ReconstructedCovered HistorySource = "reconstructed_covered"

// Coverage is an upstream assertion with provenance, not inferred from missing
// rows. The future job producer must persist it with its immutable input.
type Coverage struct {
	From, Through ledger.Date
	Reference     string
}

// FinancialModelInput currently represents an explicitly unavailable historical
// calendar. An unavailable calendar never selects the history-only model.
type FinancialModelInput struct {
	Snapshot   ModelInput
	Coverage   Coverage
	Operations []ledger.Transaction
}

type FinancialComponent struct {
	ID, Kind, CategoryCode, RegularityKey string
	Date                                  ledger.Date
	InflowMinor, OutflowMinor             int64
	SourceIDs                             []string
	CategoryID                            uuid.UUID
}

type HistoryEvidence struct {
	ID, CategorySource string
	OperationIDs       []uuid.UUID
}

type FinancialModelResult struct {
	Projection                   ModelResult
	Components                   []FinancialComponent
	Evidence                     []HistoryEvidence
	ProbabilityUnavailableReason string
	Assumptions, Limitations     []string
}
