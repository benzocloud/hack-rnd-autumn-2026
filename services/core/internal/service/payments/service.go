package payments

import (
	"context"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

type store interface {
	CreatePayment(context.Context, CreateCommand, domain.ExpectedPayment) (CreatePaymentResult, error)
	MatchPayment(context.Context, MatchCommand) (MatchPaymentResult, error)
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
