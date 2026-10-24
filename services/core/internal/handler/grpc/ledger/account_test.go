package ledger

import (
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestAccountProjectionPreservesZeroAndSource(t *testing.T) {
	scope := domain.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	amount, _ := money.New(0, money.RUB)
	namespace := domain.Namespace{System: "dataset_v1", CompanyID: "C001"}
	observation, err := domain.NewBalanceObservation(scope, "2026-10-23", amount, domain.BalanceSource{Kind: domain.StatementEOD, Namespace: namespace, RecordedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	result := ledger.ReadAccountResult{Account: domain.AccountState{Scope: scope, DataVersion: 2, Factual: domain.Imported, Namespace: namespace}, Balance: &observation}
	account := encodeAccount(result.Account, result.Balance)
	if account.Balance == nil || account.Balance.Amount == nil || account.Balance.Amount.AmountMinor != 0 || account.Balance.Source != pb.BalanceSource_BALANCE_SOURCE_STATEMENT_EOD || account.Balance.AsOf != "2026-10-23" {
		t.Fatal(account)
	}
	if account.TransactionSource == nil || account.TransactionSource.Mode != pb.FactualSourceMode_FACTUAL_SOURCE_MODE_IMPORTED || account.TransactionSource.SourceSystem != "dataset_v1" || account.TransactionSource.GetSourceCompanyId() != "C001" {
		t.Fatal(account)
	}
}
