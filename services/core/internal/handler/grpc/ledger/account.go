package ledger

import (
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
)

func encodeAccount(state domain.AccountState, balance *domain.BalanceObservation) *pb.Account {
	account := &pb.Account{Id: state.Scope.AccountID.String(), CompanyId: state.Scope.CompanyID.String(), Name: "Основной счёт", Currency: money.RUB, DataVersion: state.DataVersion}
	if balance != nil {
		observation := balance
		source := pb.BalanceSource_BALANCE_SOURCE_MANUAL
		if observation.Source().Kind == domain.StatementEOD {
			source = pb.BalanceSource_BALANCE_SOURCE_STATEMENT_EOD
		}
		account.Balance = &pb.DatedBalance{Amount: &pb.Money{AmountMinor: observation.Amount().Minor(), Currency: observation.Amount().Currency()}, AsOf: string(observation.Date()), Source: source}
	}
	switch state.Factual {
	case domain.Manual:
		account.TransactionSource = &pb.FactualSourceBinding{Mode: pb.FactualSourceMode_FACTUAL_SOURCE_MODE_MANUAL, SourceSystem: "manual_v1"}
	case domain.Imported:
		companyID := state.Namespace.CompanyID
		account.TransactionSource = &pb.FactualSourceBinding{Mode: pb.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED, SourceSystem: state.Namespace.System, SourceCompanyId: &companyID}
	}
	return account
}
