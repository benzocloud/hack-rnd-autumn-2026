package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

type CreateCommand struct {
	Record              domain.PaymentData
	ExpectedDataVersion int64
	ActorID             string
	IdempotencyKey      string
}

type CreatePaymentResult struct {
	PaymentID      uuid.UUID
	PaymentVersion int64
	DataVersion    int64
}

func (s *service) CreatePayment(ctx context.Context, cmd CreateCommand) (CreatePaymentResult, error) {
	if err := validateCreatePayment(cmd); err != nil {
		return CreatePaymentResult{}, err
	}
	if cmd.Record.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return CreatePaymentResult{}, fmt.Errorf("generate payment id: %w", err)
		}
		cmd.Record.ID = id
	}
	if cmd.Record.CreatedAt.IsZero() {
		cmd.Record.CreatedAt = time.Now().UTC()
	}
	record, err := domain.NewExpectedPayment(cmd.Record)
	if err != nil {
		return CreatePaymentResult{}, err
	}
	return s.store.CreatePayment(ctx, cmd, record)
}

func validateCreatePayment(cmd CreateCommand) error {
	if err := validateActorAndKey(cmd.ActorID, cmd.IdempotencyKey, cmd.Record.Source.Kind == ledger.Manual); err != nil {
		return err
	}
	if cmd.ExpectedDataVersion < 1 {
		return ErrInvalid
	}
	return nil
}
