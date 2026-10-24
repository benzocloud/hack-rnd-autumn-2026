package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

type MatchCommand struct {
	Scope                                ledger.Scope
	ID, PaymentID, TransactionID         uuid.UUID
	Amount                               money.Money
	ExpectedVersion, ExpectedDataVersion int64
	ActorID                              string
	IdempotencyKey                       string
	At                                   time.Time
}

type MatchPaymentResult struct {
	MatchID        uuid.UUID
	PaymentVersion int64
	DataVersion    int64
}

func (s *service) MatchPayment(ctx context.Context, cmd MatchCommand) (MatchPaymentResult, error) {
	if err := validateMatchPayment(cmd); err != nil {
		return MatchPaymentResult{}, err
	}
	if cmd.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return MatchPaymentResult{}, fmt.Errorf("generate match id: %w", err)
		}
		cmd.ID = id
	}
	if cmd.At.IsZero() {
		cmd.At = time.Now().UTC()
	}
	return s.store.MatchPayment(ctx, cmd)
}

func validateMatchPayment(cmd MatchCommand) error {
	if err := validateActorAndKey(cmd.ActorID, cmd.IdempotencyKey, true); err != nil {
		return err
	}
	if err := cmd.Scope.Validate(); err != nil {
		return err
	}
	if err := cmd.Amount.Positive(); err != nil {
		return err
	}
	if cmd.ExpectedVersion < 1 || cmd.ExpectedDataVersion < 1 || cmd.PaymentID == uuid.Nil || cmd.TransactionID == uuid.Nil {
		return ErrInvalid
	}
	return nil
}
