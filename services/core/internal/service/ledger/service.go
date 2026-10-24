package ledger

import (
	"context"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type store interface {
	CreateTransaction(context.Context, TransactionCommand, domain.Transaction) (CreateTransactionResult, error)
	RecordBalance(context.Context, BalanceCommand, domain.BalanceObservation) (RecordBalanceResult, error)
}

type service struct{ store store }

func New(storage store) *service { return &service{store: storage} }

func validateActorAndKey(actorID, idempotencyKey string, required bool) error {
	if actorID == "" && idempotencyKey == "" && !required {
		return nil
	}
	if actorID == "" || idempotencyKey == "" {
		return ErrInvalid
	}
	return nil
}
