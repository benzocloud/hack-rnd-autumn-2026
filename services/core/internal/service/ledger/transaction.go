package ledger

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type TransactionCommand struct {
	Record              domain.TransactionData
	ExpectedDataVersion int64
	ActorID             string
	IdempotencyKey      string
}

type CreateTransactionResult struct {
	TransactionID      uuid.UUID
	TransactionVersion int64
	DataVersion        int64
}

func (s *service) CreateTransaction(ctx context.Context, cmd TransactionCommand) (CreateTransactionResult, error) {
	if err := validateCreateTransaction(cmd); err != nil {
		return CreateTransactionResult{}, err
	}
	if cmd.Record.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return CreateTransactionResult{}, fmt.Errorf("generate transaction id: %w", err)
		}
		cmd.Record.ID = id
	}
	record, err := domain.NewTransaction(cmd.Record)
	if err != nil {
		return CreateTransactionResult{}, err
	}
	return s.store.CreateTransaction(ctx, cmd, record)
}

func validateCreateTransaction(cmd TransactionCommand) error {
	if err := validateActorAndKey(cmd.ActorID, cmd.IdempotencyKey, cmd.Record.Source.Kind == domain.Manual); err != nil {
		return err
	}
	if cmd.ExpectedDataVersion < 1 {
		return ErrInvalid
	}
	return nil
}
