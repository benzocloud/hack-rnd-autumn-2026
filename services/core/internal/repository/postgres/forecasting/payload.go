package forecasting

import (
	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

// Version 1 is private storage, not a transport contract. Typed int64 fields avoid
// float conversion; domain Money and Transaction have private fields.
type snapshotPayload struct {
	Opening  balancePayload  `json:"opening"`
	Facts    []factPayload   `json:"facts"`
	Calendar domain.Calendar `json:"calendar"`
}

type balancePayload struct {
	AmountMinor *int64               `json:"amount_minor"`
	Currency    string               `json:"currency"`
	Source      ledger.BalanceSource `json:"source"`
	DataVersion int64                `json:"data_version"`
}

type factPayload struct {
	ID               uuid.UUID        `json:"id"`
	Date             ledger.Date      `json:"date"`
	Direction        ledger.Direction `json:"direction"`
	AmountMinor      *int64           `json:"amount_minor"`
	Currency         string           `json:"currency"`
	Source           ledger.Source    `json:"source"`
	CategoryID       uuid.UUID        `json:"category_id"`
	CounterpartyINN  string           `json:"counterparty_inn"`
	CounterpartyName string           `json:"counterparty_name"`
	Purpose          string           `json:"purpose"`
	Version          int64            `json:"version"`
	DataVersion      int64            `json:"data_version"`
}
