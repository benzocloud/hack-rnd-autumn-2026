package ledger_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
)

func TestReadAccountWithoutInventedBalance(t *testing.T) {
	scope := transactionCommand(t).Record.Scope
	storage := &captureStore{}
	result, err := ledger.New(storage.withinTx).ReadAccount(t.Context(), scope, "2026-10-24")
	if err != nil || result.Account.Scope != scope || result.Account.DataVersion != 1 || result.Balance != nil || storage.saves != 0 || storage.inserts != 0 {
		t.Fatal(result, err)
	}
	amount, _ := money.New(-500, money.RUB)
	observation, err := domain.NewBalanceObservation(scope, "2026-10-23", amount, domain.BalanceSource{Kind: domain.ManualEOD, RecordedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	storage.balance, storage.balanceFound, storage.revision = observation, true, 1
	result, err = ledger.New(storage.withinTx).ReadAccount(t.Context(), scope, "2026-10-24")
	if err != nil || result.Balance == nil || result.Balance.Amount().Minor() != -500 || result.Balance.Date() != "2026-10-23" || result.Account.Factual != "" {
		t.Fatal(result, err)
	}
}

func TestReadAccountValidatesAndWithholdsFailedResult(t *testing.T) {
	scope := transactionCommand(t).Record.Scope
	for _, test := range []struct {
		name  string
		scope domain.Scope
		date  domain.Date
	}{
		{"missing company", domain.Scope{AccountID: scope.AccountID}, "2026-10-24"},
		{"invalid date", scope, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage := &captureStore{}
			_, err := ledger.New(storage.withinTx).ReadAccount(t.Context(), test.scope, test.date)
			if !errors.Is(err, ledger.ErrInvalid) || storage.calls != 0 {
				t.Fatal(err, storage.calls)
			}
		})
	}
	for _, mode := range []string{"transaction failure", "cross-tenant account", "future balance", "corrupt balance version"} {
		t.Run(mode, func(t *testing.T) {
			storage := &captureStore{}
			if mode == "transaction failure" {
				storage.txErr = ledger.ErrUnavailable
			}
			if mode == "cross-tenant account" {
				storage.account, _ = domain.NewBankAccount(domain.Scope{CompanyID: uuid.New(), AccountID: scope.AccountID})
			}
			if mode == "future balance" || mode == "corrupt balance version" {
				amount, _ := money.New(0, money.RUB)
				date := domain.Date("2026-10-24")
				if mode == "future balance" {
					date = "2026-10-25"
				}
				storage.balance, _ = domain.NewBalanceObservation(scope, date, amount, domain.BalanceSource{Kind: domain.ManualEOD, RecordedAt: time.Now()})
				storage.balanceFound, storage.revision = true, 2
			}
			result, err := ledger.New(storage.withinTx).ReadAccount(t.Context(), scope, "2026-10-24")
			if !errors.Is(err, ledger.ErrUnavailable) || !reflect.DeepEqual(result, ledger.ReadAccountResult{}) {
				t.Fatal(result, err)
			}
		})
	}
}
