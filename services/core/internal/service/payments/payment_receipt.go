package payments

import (
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type paymentResult struct {
	ID          uuid.UUID `json:"id"`
	Version     int64     `json:"version"`
	DataVersion int64     `json:"data_version"`
}

func validatePaymentResult(result paymentResult) error {
	if result.ID == uuid.Nil || result.Version < 1 || result.DataVersion < 1 {
		return ErrUnavailable
	}
	return nil
}

type paymentBody struct {
	ID                            uuid.UUID
	Scope                         ledger.Scope
	Date                          ledger.Date
	Direction                     ledger.Direction
	Amount                        int64
	Currency                      string
	Source                        ledger.Source
	Category                      ledger.Category
	CounterpartyName, Description string
	CreatedAt                     time.Time
	ExpectedDataVersion           int64
}

func paymentRequest(cmd CreateCommand) paymentBody {
	d := cmd.Record
	d.Source.ImportedAt = d.Source.ImportedAt.UTC()
	return paymentBody{ID: d.ID, Scope: d.Scope, Date: d.ScheduledDate, Direction: d.Direction, Amount: d.Amount.Minor(),
		Currency: d.Amount.Currency(), Source: d.Source, Category: d.Category, CounterpartyName: d.CounterpartyName,
		Description: d.Description, CreatedAt: d.CreatedAt.UTC(), ExpectedDataVersion: cmd.ExpectedDataVersion}
}
