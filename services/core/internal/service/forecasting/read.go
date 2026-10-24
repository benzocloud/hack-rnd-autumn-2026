package forecasting

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func (s *service) GetSnapshot(ctx context.Context, scope ledger.Scope, id uuid.UUID) (domain.FinancialSnapshot, error) {
	if err := validateGetSnapshot(scope, id); err != nil {
		return domain.FinancialSnapshot{}, err
	}
	var result domain.FinancialSnapshot
	err := s.withinTx(ctx, func(ctx context.Context, storage store) error {
		var err error
		result, err = storage.ReadSnapshot(ctx, scope, id)
		return err
	})
	if err != nil {
		return domain.FinancialSnapshot{}, err
	}
	return result, nil
}

func validateGetSnapshot(scope ledger.Scope, id uuid.UUID) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if id == uuid.Nil {
		return ErrInvalid
	}
	return nil
}
