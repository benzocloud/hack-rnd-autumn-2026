package ledger

import (
	"maps"
	"time"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

type manualBalanceBody struct {
	Scope               domain.Scope `json:"scope"`
	Date                domain.Date  `json:"date"`
	AmountMinor         int64        `json:"amount_minor"`
	Currency            string       `json:"currency"`
	Note                string       `json:"note"`
	ExpectedDataVersion int64        `json:"expected_data_version"`
}

func manualBalanceRequest(cmd ManualBalanceCommand) manualBalanceBody {
	return manualBalanceBody{Scope: cmd.Scope, Date: cmd.Date, AmountMinor: cmd.Amount.Minor(), Currency: cmd.Amount.Currency(), Note: cmd.Note, ExpectedDataVersion: cmd.ExpectedDataVersion}
}

// Dedicated persisted v1 projection. SQL stores opaque bytes, not service DTOs.
// Account name/currency are the immutable MVP defaults in the v1 wire mapper.
type manualBalanceReceipt struct {
	SchemaVersion   int                       `json:"schema_version"`
	ViewDate        domain.Date               `json:"view_date"`
	Scope           domain.Scope              `json:"scope"`
	DataVersion     int64                     `json:"data_version"`
	Factual         *domain.SourceKind        `json:"factual_source"`
	System          string                    `json:"source_system"`
	SourceCompanyID string                    `json:"source_company_id"`
	Bindings        map[string]string         `json:"bindings"`
	Balance         *manualBalanceObservation `json:"balance"`
}

type manualBalanceObservation struct {
	Date            domain.Date        `json:"date"`
	AmountMinor     *int64             `json:"amount_minor"`
	Currency        string             `json:"currency"`
	Kind            domain.BalanceKind `json:"source_kind"`
	System          string             `json:"source_system"`
	SourceCompanyID string             `json:"source_company_id"`
	RecordedAt      time.Time          `json:"recorded_at"`
	DataVersion     int64              `json:"created_data_version"`
}

func saveManualBalance(viewDate domain.Date, state domain.AccountState, balance domain.BalanceObservation, revision int64) manualBalanceReceipt {
	source := balance.Source()
	amount := balance.Amount().Minor()
	return manualBalanceReceipt{SchemaVersion: 1, ViewDate: viewDate, Scope: state.Scope, DataVersion: state.DataVersion, Factual: &state.Factual, System: state.Namespace.System,
		SourceCompanyID: state.Namespace.CompanyID, Bindings: maps.Clone(state.Bindings), Balance: &manualBalanceObservation{Date: balance.Date(), AmountMinor: &amount,
			Currency: balance.Amount().Currency(), Kind: source.Kind, System: source.Namespace.System, SourceCompanyID: source.Namespace.CompanyID, RecordedAt: source.RecordedAt, DataVersion: revision}}
}

func restoreManualBalance(cmd ManualBalanceCommand, receipt manualBalanceReceipt) (RecordManualBalanceResult, error) {
	if receipt.SchemaVersion != 1 || receipt.ViewDate.Validate() != nil || receipt.Scope != cmd.Scope || receipt.Factual == nil || receipt.Balance == nil || receipt.Balance.AmountMinor == nil {
		return RecordManualBalanceResult{}, ErrUnavailable
	}
	account, err := domain.RestoreBankAccount(domain.AccountState{Scope: receipt.Scope, DataVersion: receipt.DataVersion, Factual: *receipt.Factual,
		Namespace: domain.Namespace{System: receipt.System, CompanyID: receipt.SourceCompanyID}, Bindings: receipt.Bindings})
	if err != nil {
		return RecordManualBalanceResult{}, ErrUnavailable
	}
	if receipt.DataVersion < cmd.ExpectedDataVersion || receipt.DataVersion-cmd.ExpectedDataVersion > 1 {
		return RecordManualBalanceResult{}, ErrUnavailable
	}
	saved := receipt.Balance
	if saved.DataVersion < 1 || saved.DataVersion > receipt.DataVersion || saved.Date < cmd.Date || saved.Date > receipt.ViewDate {
		return RecordManualBalanceResult{}, ErrUnavailable
	}
	amount, err := money.New(*saved.AmountMinor, saved.Currency)
	if err != nil {
		return RecordManualBalanceResult{}, ErrUnavailable
	}
	balance, err := domain.NewBalanceObservation(receipt.Scope, saved.Date, amount, domain.BalanceSource{Kind: saved.Kind,
		Namespace: domain.Namespace{System: saved.System, CompanyID: saved.SourceCompanyID}, RecordedAt: saved.RecordedAt})
	if err != nil {
		return RecordManualBalanceResult{}, ErrUnavailable
	}
	if saved.Date == cmd.Date && (balance.Amount() != cmd.Amount || saved.Kind != domain.ManualEOD) {
		return RecordManualBalanceResult{}, ErrUnavailable
	}
	return RecordManualBalanceResult{Account: account.State(), Balance: &balance}, nil
}
