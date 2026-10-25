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

type transactionStore interface {
	accountStore
	FindTransaction(context.Context, domain.Scope, uuid.UUID, domain.Source) (domain.Transaction, int64, bool, error)
	InsertTransaction(context.Context, domain.Transaction, int64) error
	HasImportedEquivalent(context.Context, domain.TransactionData) (bool, error)
}

func (s *service) CreateTransaction(ctx context.Context, cmd TransactionCommand) (CreateTransactionResult, error) {
	if err := validateCreateTransaction(cmd); err != nil {
		return CreateTransactionResult{}, err
	}
	data := cmd.Record
	if data.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return CreateTransactionResult{}, fmt.Errorf("generate transaction id: %w", err)
		}
		data.ID = id
	}
	record, err := domain.NewTransaction(data)
	if err != nil {
		return CreateTransactionResult{}, err
	}
	var result CreateTransactionResult
	txErr := s.withinTx(ctx, func(ctx context.Context, store store, commands commandStore) error {
		var saved transactionResult
		replay, err := replayCommand(ctx, commands, cmd.Record.Scope, cmd.ActorID, "create_transaction", "", cmd.IdempotencyKey, transactionRequest(cmd), &saved)
		if err != nil {
			return err
		}
		if replay {
			if err := validateTransactionResult(saved); err != nil {
				return err
			}
			result = CreateTransactionResult{TransactionID: saved.ID, TransactionVersion: saved.Version, DataVersion: saved.DataVersion}
			return nil
		}
		result, err = createTransaction(ctx, store, cmd, record)
		if err != nil {
			return err
		}
		return completeCommand(ctx, commands, cmd.Record.Scope, cmd.ActorID, "create_transaction", "", cmd.IdempotencyKey, transactionResult{ID: result.TransactionID, Version: result.TransactionVersion, DataVersion: result.DataVersion})
	})
	if txErr != nil {
		return CreateTransactionResult{}, txErr
	}
	return result, nil
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

func createTransaction(ctx context.Context, store transactionStore, cmd TransactionCommand, record domain.Transaction) (CreateTransactionResult, error) {
	account, err := store.LoadAccount(ctx, record.Data().Scope)
	if err != nil {
		return CreateTransactionResult{}, err
	}
	return createTransactionOnAccount(ctx, store, account, cmd, record)
}

func createTransactionOnAccount(ctx context.Context, store transactionStore, account *domain.BankAccount, cmd TransactionCommand, record domain.Transaction) (CreateTransactionResult, error) {
	if account == nil || account.Scope() != record.Data().Scope {
		return CreateTransactionResult{}, ErrUnavailable
	}
	if record.Data().Source.Kind == domain.Imported {
		stored, revision, found, err := store.FindTransaction(ctx, record.Data().Scope, uuid.Nil, record.Data().Source)
		if err != nil {
			return CreateTransactionResult{}, err
		}
		if found {
			if _, err := record.CheckReplay(stored); err != nil {
				return CreateTransactionResult{}, err
			}
			return CreateTransactionResult{TransactionID: stored.Data().ID, TransactionVersion: stored.Version(), DataVersion: revision}, nil
		}
	}
	if account.DataVersion() != cmd.ExpectedDataVersion {
		return CreateTransactionResult{}, ErrStaleVersion
	}
	if record.Data().Source.Kind == domain.Manual && account.State().Factual == domain.Imported {
		duplicate, err := store.HasImportedEquivalent(ctx, record.Data())
		if err != nil {
			return CreateTransactionResult{}, err
		}
		if duplicate {
			return CreateTransactionResult{}, ErrSuspectedDuplicate
		}
	}
	if err := account.BindFacts(record.Data().Source); err != nil {
		return CreateTransactionResult{}, err
	}
	if err := account.AdvanceDataVersion(); err != nil {
		return CreateTransactionResult{}, err
	}
	if err := store.SaveAccount(ctx, account); err != nil {
		return CreateTransactionResult{}, err
	}
	if err := store.InsertTransaction(ctx, record, account.DataVersion()); err != nil {
		return CreateTransactionResult{}, err
	}
	return CreateTransactionResult{TransactionID: record.Data().ID, TransactionVersion: record.Version(), DataVersion: account.DataVersion()}, nil
}
