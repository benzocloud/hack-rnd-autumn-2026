package ledger

import (
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type balanceResult struct {
	Date        domain.Date `json:"date"`
	DataVersion int64       `json:"data_version"`
}

func validateBalanceResult(result balanceResult) error {
	if result.DataVersion < 1 || result.Date.Validate() != nil {
		return ErrUnavailable
	}
	return nil
}

type balanceBody struct {
	Scope               domain.Scope
	Date                domain.Date
	Amount              int64
	Currency            string
	Source              domain.BalanceSource
	Note                string
	ExpectedDataVersion int64
}

func balanceRequest(cmd BalanceCommand) balanceBody {
	source := cmd.Source
	source.RecordedAt = source.RecordedAt.UTC()
	return balanceBody{Scope: cmd.Scope, Date: cmd.Date, Amount: cmd.Amount.Minor(), Currency: cmd.Amount.Currency(),
		Source: source, Note: cmd.Note, ExpectedDataVersion: cmd.ExpectedDataVersion}
}
