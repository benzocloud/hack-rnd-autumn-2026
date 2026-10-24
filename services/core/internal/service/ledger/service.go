package ledger

import (
	"context"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type accountStore interface {
	LoadAccount(context.Context, domain.Scope) (*domain.BankAccount, error)
	SaveAccount(context.Context, *domain.BankAccount) error
}

type service struct {
	withinTx func(context.Context, Work) error
}

func New(withinTx func(context.Context, Work) error) *service { return &service{withinTx: withinTx} }

func validateActorAndKey(actorID, idempotencyKey string, required bool) error {
	if actorID == "" && idempotencyKey == "" && !required {
		return nil
	}
	if actorID == "" || idempotencyKey == "" {
		return ErrInvalid
	}
	return nil
}
