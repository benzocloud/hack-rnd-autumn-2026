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

func (g ReconciliationGroup) Validate() error {
	if g.ID == uuid.Nil || g.Scope.Validate() != nil || g.Version < 1 || (g.Status != "active" && g.Status != "revoked") || g.Cutoff.Validate() != nil || g.HorizonStart.Validate() != nil || g.HorizonEnd.Validate() != nil || g.ConfirmedBy == "" || g.ConfirmedAt.IsZero() || g.Additional != (len(g.Operations) == 0) || len(g.Operations) > 20000 {
		return ErrInvalid
	}
	cutoff, _ := time.Parse(time.DateOnly, string(g.Cutoff))
	if string(g.HorizonStart) != cutoff.AddDate(0, 0, 1).Format(time.DateOnly) || string(g.HorizonEnd) != cutoff.AddDate(0, 0, 30).Format(time.DateOnly) {
		return ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, ref := range g.Operations {
		if ref.ID == uuid.Nil || ref.Version < 1 || seen[ref.ID] {
			return ErrInvalid
		}
		seen[ref.ID] = true
	}
	return nil
}
