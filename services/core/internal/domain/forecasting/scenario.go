package forecasting

import (
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

// Scenario is an immutable simulation record; Payload is a versioned receipt.
type Scenario struct {
	Scope                          ledger.Scope
	ID, ForecastID, BaseForecastID uuid.UUID
	Name                           string
	CreatedAt                      time.Time
	Payload                        []byte
}
