package payments

import (
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type matchResult struct {
	ID             uuid.UUID `json:"id"`
	PaymentVersion int64     `json:"payment_version"`
	DataVersion    int64     `json:"data_version"`
}

func validateMatchResult(result matchResult) error {
	if result.ID == uuid.Nil || result.PaymentVersion < 1 || result.DataVersion < 1 {
		return ErrUnavailable
	}
	return nil
}

type matchBody struct {
	Scope                                ledger.Scope
	ID, PaymentID, TransactionID         uuid.UUID
	Amount                               int64
	Currency                             string
	ExpectedVersion, ExpectedDataVersion int64
	At                                   time.Time
	ExpectedTransactionVersion           int64 `json:",omitempty"`
}

func matchRequest(cmd MatchCommand) matchBody {
	return matchBody{Scope: cmd.Scope, ID: cmd.ID, PaymentID: cmd.PaymentID, TransactionID: cmd.TransactionID,
		Amount: cmd.Amount.Minor(), Currency: cmd.Amount.Currency(), ExpectedVersion: cmd.ExpectedVersion,
		ExpectedDataVersion: cmd.ExpectedDataVersion, At: cmd.At.UTC(), ExpectedTransactionVersion: cmd.ExpectedTransactionVersion}
}
