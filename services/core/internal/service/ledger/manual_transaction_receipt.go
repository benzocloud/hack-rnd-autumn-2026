package ledger

import (
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

type manualTransactionReceipt struct {
	SchemaVersion int             `json:"schema_version"`
	Record        transactionBody `json:"record"`
	CreatedAt     time.Time       `json:"created_at"`
	DataVersion   int64           `json:"data_version"`
}

func restoreManualTransaction(cmd ManualTransactionCommand, receipt manualTransactionReceipt) (CreateManualTransactionResult, error) {
	if receipt.SchemaVersion != 1 || receipt.Record.ID == uuid.Nil || receipt.Record.ID.Version() != 7 || receipt.CreatedAt.IsZero() || receipt.CreatedAt.Year() < 1 || receipt.CreatedAt.Year() > 9999 ||
		receipt.DataVersion <= cmd.ExpectedDataVersion || receipt.DataVersion-cmd.ExpectedDataVersion != 1 {
		return CreateManualTransactionResult{}, ErrUnavailable
	}
	saved := receipt.Record
	saved.ID = uuid.Nil
	if saved != transactionRequest(manualTransactionCommand(cmd)) {
		return CreateManualTransactionResult{}, ErrUnavailable
	}
	d := receipt.Record
	amount, err := money.New(d.Amount, d.Currency)
	if err != nil {
		return CreateManualTransactionResult{}, ErrUnavailable
	}
	record, err := domain.NewTransaction(domain.TransactionData{ID: d.ID, Scope: d.Scope, Date: d.Date, Direction: d.Direction, Amount: amount,
		Source: d.Source, Category: d.Category, CounterpartyINN: d.CounterpartyINN, CounterpartyName: d.CounterpartyName, Purpose: d.Purpose})
	if err != nil {
		return CreateManualTransactionResult{}, ErrUnavailable
	}
	return CreateManualTransactionResult{Transaction: record, CreatedAt: receipt.CreatedAt.UTC(), DataVersion: receipt.DataVersion}, nil
}
