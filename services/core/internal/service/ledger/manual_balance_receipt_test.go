package ledger

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func receiptFixture(t *testing.T) (ManualBalanceCommand, manualBalanceReceipt) {
	t.Helper()
	scope := domain.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	amount, _ := money.New(0, money.RUB)
	account, err := domain.NewBankAccount(scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := account.AdvanceDataVersion(); err != nil {
		t.Fatal(err)
	}
	balance, err := domain.NewBalanceObservation(scope, "2026-10-23", amount, domain.BalanceSource{Kind: domain.ManualEOD, RecordedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	cmd := ManualBalanceCommand{Scope: scope, Date: "2026-10-23", ViewDate: "2026-10-24", Amount: amount, ExpectedDataVersion: 1}
	return cmd, saveManualBalance(cmd.ViewDate, account.State(), balance, 2)
}

func TestManualBalanceReceiptZeroRoundtripAndCorruption(t *testing.T) {
	cmd, receipt := receiptFixture(t)
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var stored manualBalanceReceipt
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	result, err := restoreManualBalance(cmd, stored)
	if err != nil || result.Balance == nil || result.Balance.Amount().Minor() != 0 || result.Account.DataVersion != 2 {
		t.Fatal(result, err)
	}
	for name, edit := range map[string]func(*manualBalanceReceipt){
		"schema":                    func(r *manualBalanceReceipt) { r.SchemaVersion = 2 },
		"scope":                     func(r *manualBalanceReceipt) { r.Scope.CompanyID = uuid.New() },
		"missing source":            func(r *manualBalanceReceipt) { r.Factual = nil },
		"missing balance":           func(r *manualBalanceReceipt) { r.Balance = nil },
		"missing zero amount":       func(r *manualBalanceReceipt) { r.Balance.AmountMinor = nil },
		"wrong currency":            func(r *manualBalanceReceipt) { r.Balance.Currency = "USD" },
		"missing version":           func(r *manualBalanceReceipt) { r.DataVersion = 0 },
		"unrelated account version": func(r *manualBalanceReceipt) { r.DataVersion = 4 },
		"future balance":            func(r *manualBalanceReceipt) { r.Balance.Date = "2026-10-25" },
		"balance before command":    func(r *manualBalanceReceipt) { r.Balance.Date = "2026-10-22" },
		"balance revision":          func(r *manualBalanceReceipt) { r.Balance.DataVersion = 3 },
		"zero source time":          func(r *manualBalanceReceipt) { r.Balance.RecordedAt = time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			cmd, r := receiptFixture(t)
			edit(&r)
			if _, err := restoreManualBalance(cmd, r); !errors.Is(err, ErrUnavailable) {
				t.Fatal("corrupt receipt accepted", err)
			}
		})
	}
}
