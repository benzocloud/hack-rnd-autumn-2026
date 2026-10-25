package forecasting

import (
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type OperationReference struct {
	ID      uuid.UUID
	Version int64
}
type ReconciliationGroup struct {
	ID                               uuid.UUID
	Scope                            ledger.Scope
	Version                          int64
	Status                           string
	Cutoff, HorizonStart, HorizonEnd ledger.Date
	ConfirmedBy                      string
	ConfirmedAt                      time.Time
	Additional                       bool
	Operations                       []OperationReference
}
