package ledger

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

const manualTransactionOperation = "create_manual_transaction_v1"

var manualINN = regexp.MustCompile(`^(?:[0-9]{10}|[0-9]{12})$`)

type ManualTransactionCommand struct {
	Record                  domain.TransactionData
	ExpectedDataVersion     int64
	ActorID, IdempotencyKey string
	ViewDate                domain.Date
}

type CreateManualTransactionResult struct {
	Transaction domain.Transaction
	CreatedAt   time.Time
	DataVersion int64
}

type manualTransactionStore interface {
	CategoryExists(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	TransactionCreatedAt(context.Context, domain.Scope, uuid.UUID) (time.Time, error)
}

func (s *service) CreateManualTransaction(ctx context.Context, cmd ManualTransactionCommand) (CreateManualTransactionResult, error) {
	if err := validateManualTransaction(cmd); err != nil {
		return CreateManualTransactionResult{}, err
	}
	if s.withinTx == nil {
		return CreateManualTransactionResult{}, ErrUnavailable
	}
	id, err := uuid.NewV7()
	if err != nil {
		return CreateManualTransactionResult{}, fmt.Errorf("generate manual transaction id: %w", err)
	}
	data := cmd.Record
	data.ID = id
	record, err := domain.NewTransaction(data)
	if err != nil {
		return CreateManualTransactionResult{}, ErrInvalid
	}
	var result CreateManualTransactionResult
	err = s.withinTx(ctx, func(ctx context.Context, storage store, commands commandStore) error {
		var receipt manualTransactionReceipt
		base := manualTransactionCommand(cmd)
		replay, err := replayCommand(ctx, commands, cmd.Record.Scope, cmd.ActorID, manualTransactionOperation, "", cmd.IdempotencyKey, transactionRequest(base), &receipt)
		if err != nil {
			return err
		}
		if replay {
			result, err = restoreManualTransaction(cmd, receipt)
			return err
		}
		account, err := storage.LoadAccount(ctx, cmd.Record.Scope)
		if err != nil {
			return err
		}
		if account == nil || account.Scope() != cmd.Record.Scope {
			return ErrUnavailable
		}
		if account.DataVersion() != cmd.ExpectedDataVersion {
			return ErrStaleVersion
		}
		if cmd.Record.Category.ID != uuid.Nil {
			exists, err := storage.CategoryExists(ctx, cmd.Record.Scope.CompanyID, cmd.Record.Category.ID)
			if err != nil {
				return err
			}
			if !exists {
				return ErrInvalid
			}
		}
		base.Record.ID = id
		created, err := createTransactionOnAccount(ctx, storage, account, base, record)
		if err != nil {
			return err
		}
		at, err := storage.TransactionCreatedAt(ctx, cmd.Record.Scope, id)
		if err != nil {
			return err
		}
		receipt = manualTransactionReceipt{SchemaVersion: 1, Record: transactionRequest(base), CreatedAt: at, DataVersion: created.DataVersion}
		result, err = restoreManualTransaction(cmd, receipt)
		if err != nil {
			return err
		}
		return completeCommand(ctx, commands, cmd.Record.Scope, cmd.ActorID, manualTransactionOperation, "", cmd.IdempotencyKey, receipt)
	})
	if err != nil {
		return CreateManualTransactionResult{}, err
	}
	return result, nil
}

func manualTransactionCommand(cmd ManualTransactionCommand) TransactionCommand {
	return TransactionCommand{Record: cmd.Record, ExpectedDataVersion: cmd.ExpectedDataVersion, ActorID: cmd.ActorID, IdempotencyKey: cmd.IdempotencyKey}
}

func validateManualTransaction(cmd ManualTransactionCommand) error {
	d := cmd.Record
	if strings.TrimSpace(cmd.ActorID) == "" || !validManualBalanceKey(cmd.IdempotencyKey) || cmd.ExpectedDataVersion < 1 || cmd.ViewDate.Validate() != nil || d.Date > cmd.ViewDate ||
		d.ID != uuid.Nil || d.Source != (domain.Source{Kind: domain.Manual}) || (d.CounterpartyINN != "" && !manualINN.MatchString(d.CounterpartyINN)) {
		return ErrInvalid
	}
	for _, field := range []struct {
		text  string
		limit int
	}{{d.CounterpartyName, 500}, {d.Purpose, 2000}} {
		if !utf8.ValidString(field.text) || utf8.RuneCountInString(field.text) > field.limit || strings.IndexByte(field.text, 0) >= 0 {
			return ErrInvalid
		}
	}
	return nil
}
