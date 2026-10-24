package ledger

import (
	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type transactionResult struct {
	ID          uuid.UUID `json:"id"`
	Version     int64     `json:"version"`
	DataVersion int64     `json:"data_version"`
}

func validateTransactionResult(result transactionResult) error {
	if result.ID == uuid.Nil || result.Version < 1 || result.DataVersion < 1 {
		return ErrUnavailable
	}
	return nil
}

type transactionBody struct {
	ID                                         uuid.UUID
	Scope                                      domain.Scope
	Date                                       domain.Date
	Direction                                  domain.Direction
	Amount                                     int64
	Currency                                   string
	Source                                     domain.Source
	Category                                   domain.Category
	CounterpartyINN, CounterpartyName, Purpose string
	ExpectedDataVersion                        int64
}

func transactionRequest(cmd TransactionCommand) transactionBody {
	d := cmd.Record
	d.Source.ImportedAt = d.Source.ImportedAt.UTC()
	return transactionBody{ID: d.ID, Scope: d.Scope, Date: d.Date, Direction: d.Direction,
		Amount: d.Amount.Minor(), Currency: d.Amount.Currency(), Source: d.Source, Category: d.Category,
		CounterpartyINN: d.CounterpartyINN, CounterpartyName: d.CounterpartyName, Purpose: d.Purpose, ExpectedDataVersion: cmd.ExpectedDataVersion}
}
