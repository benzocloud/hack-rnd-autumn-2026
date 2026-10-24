package ledger

import (
	"context"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type ReadAccountResult struct {
	Account domain.AccountState
	Balance *domain.BalanceObservation
}

type readStore interface {
	accountStore
	FindLatestBalance(context.Context, domain.Scope, domain.Date) (domain.BalanceObservation, int64, bool, error)
}

// ReadAccount keeps account/version/bindings and the latest accepted EOD in one
// transaction. LoadAccount locks the account, as do all financial writers.
// A missing observation remains nil; no balance is inferred from transactions.
func (s *service) ReadAccount(ctx context.Context, scope domain.Scope, asOf domain.Date) (ReadAccountResult, error) {
	if err := validateReadAccount(scope, asOf); err != nil {
		return ReadAccountResult{}, err
	}
	if s.withinTx == nil {
		return ReadAccountResult{}, ErrUnavailable
	}
	var result ReadAccountResult
	err := s.withinTx(ctx, func(ctx context.Context, storage store, _ commandStore) error {
		account, err := storage.LoadAccount(ctx, scope)
		if err != nil {
			return err
		}
		if account == nil || account.Scope() != scope {
			return ErrUnavailable
		}
		state := account.State()
		observation, revision, found, err := storage.FindLatestBalance(ctx, scope, asOf)
		if err != nil {
			return err
		}
		result.Account = state
		if found {
			if observation.Scope() != scope || observation.Date().Validate() != nil || observation.Date() > asOf || observation.Amount().Validate() != nil || revision < 1 || revision > state.DataVersion {
				return ErrUnavailable
			}
			result.Balance = &observation
		}
		return nil
	})
	if err != nil {
		return ReadAccountResult{}, err
	}
	return result, nil
}

func validateReadAccount(scope domain.Scope, asOf domain.Date) error {
	if scope.Validate() != nil || asOf.Validate() != nil {
		return ErrInvalid
	}
	return nil
}
