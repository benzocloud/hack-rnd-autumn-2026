package payments

import (
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type paymentResult struct {
	Projection  *changeReceipt `json:"projection,omitempty"`
	ID          uuid.UUID      `json:"id"`
	Version     int64          `json:"version"`
	DataVersion int64          `json:"data_version"`
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
	ReconciliationGroupID         *uuid.UUID `json:",omitempty"`
}

func paymentRequest(cmd CreateCommand) paymentBody {
	d := cmd.Record
	d.Source.ImportedAt = d.Source.ImportedAt.UTC()
	var group *uuid.UUID
	if d.ReconciliationGroupID != uuid.Nil {
		group = &d.ReconciliationGroupID
	}
	return paymentBody{ID: d.ID, Scope: d.Scope, Date: d.ScheduledDate, Direction: d.Direction, Amount: d.Amount.Minor(),
		Currency: d.Amount.Currency(), Source: d.Source, Category: d.Category, CounterpartyName: d.CounterpartyName,
		Description: d.Description, CreatedAt: d.CreatedAt.UTC(), ExpectedDataVersion: cmd.ExpectedDataVersion, ReconciliationGroupID: group}
}

func paymentProjection(r CreatePaymentResult) *changeReceipt {
	p := newChangeReceipt(ChangePaymentResult{Payment: r.Payment, DataVersion: r.DataVersion})
	return &p
}
