package forecasting

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type CaptureCommand struct {
	Scope  ledger.Scope
	Cutoff ledger.Date
}

// CaptureSnapshot is internal-only until a trusted Auth/transport adapter exists.
// It captures business-date history; it never claims point-in-time knowledge or
// copies today's payment remaining amounts into a historical calendar.
func (s *service) CaptureSnapshot(ctx context.Context, cmd CaptureCommand) (domain.FinancialSnapshot, error) {
	if err := validateCapture(cmd); err != nil {
		return domain.FinancialSnapshot{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.FinancialSnapshot{}, fmt.Errorf("generate snapshot id: %w", err)
	}
	var result domain.FinancialSnapshot
	err = s.withinTx(ctx, func(ctx context.Context, storage store) error {
		revision, err := storage.ReadDataVersion(ctx, cmd.Scope)
		if err != nil {
			return err
		}
		opening, found, err := storage.ReadOpeningBalance(ctx, cmd.Scope, cmd.Cutoff)
		if err != nil {
			return err
		}
		if !found {
			return ErrMissingBalance
		}
		facts, err := storage.ReadFacts(ctx, cmd.Scope, cmd.Cutoff)
		if err != nil {
			return err
		}
		calendar, err := storage.ReadCalendar(ctx, cmd.Scope)
		if err != nil {
			return err
		}
		result, err = domain.NewFinancialSnapshot(domain.SnapshotData{ID: id, Scope: cmd.Scope,
			DataVersion: revision, Cutoff: cmd.Cutoff, CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
			Opening: opening, Facts: facts, Calendar: calendar})
		if err != nil {
			return err
		}
		return storage.InsertSnapshot(ctx, result)
	})
	if err != nil {
		return domain.FinancialSnapshot{}, err
	}
	return result, nil
}

func validateCapture(cmd CaptureCommand) error {
	if err := cmd.Scope.Validate(); err != nil {
		return err
	}
	return cmd.Cutoff.Validate()
}
