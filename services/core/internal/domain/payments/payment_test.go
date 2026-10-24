package payments_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

func rub(t *testing.T, minor int64) money.Money {
	t.Helper()
	amount, err := money.New(minor, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	return amount
}

func fixtures(t *testing.T) (payments.ExpectedPayment, ledger.Transaction) {
	t.Helper()
	scope := ledger.Scope{CompanyID: uuid.MustParse("019a0c65-9800-7000-8000-000000000001"), AccountID: uuid.MustParse("019a0c65-9800-7000-8000-000000000002")}
	source := ledger.Source{Kind: ledger.Manual}
	payment, err := payments.NewExpectedPayment(payments.PaymentData{ID: uuid.MustParse("019a0c65-9800-7000-8000-000000000008"), Scope: scope,
		Direction: ledger.Outflow, Amount: rub(t, 100), ScheduledDate: "2026-10-23", Source: source,
		CreatedAt: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := ledger.NewTransaction(ledger.TransactionData{ID: uuid.MustParse("019a0c65-9800-7000-8000-000000000009"), Scope: scope, Date: "2026-10-24",
		Direction: ledger.Outflow, Amount: rub(t, 100), Source: source})
	if err != nil {
		t.Fatal(err)
	}
	return payment, fact
}

func TestPaymentLifecycle(t *testing.T) {
	t.Parallel()
	original, fact := fixtures(t)
	at := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	if overdue, err := original.IsOverdue("2026-10-24"); err != nil || !overdue || original.Status() != payments.Planned {
		t.Fatalf("past date changed status: %v, %v", overdue, err)
	}
	partial, match, err := original.Match(uuid.MustParse("019a0c65-9800-7000-8000-00000000000b"), fact, rub(t, 0), rub(t, 40), at)
	if err != nil || partial.Status() != payments.PartiallyMatched || partial.Remaining().Minor() != 60 ||
		partial.Version() != 2 || match.Amount().Minor() != 40 || match.Scope() != fact.Data().Scope {
		t.Fatalf("partial match: %v, %v, %v", partial, match, err)
	}
	if original.Status() != payments.Planned || original.Remaining().Minor() != 100 || fact.Data().Amount.Minor() != 100 {
		t.Fatal("matching modified an input or created money")
	}
	full, _, err := partial.Match(uuid.MustParse("019a0c65-9800-7000-8000-00000000000c"), fact, rub(t, 40), rub(t, 60), at)
	if err != nil || full.Status() != payments.Matched || full.Remaining().Minor() != 0 || full.Version() != 3 {
		t.Fatalf("full match: %v, %v", full, err)
	}
	if _, _, err := full.Match(uuid.MustParse("019a0c65-9800-7000-8000-00000000000d"), fact, rub(t, 100), rub(t, 1), at); !errors.Is(err, payments.ErrState) {
		t.Fatalf("matched payment accepted another match: %v", err)
	}
	cancelled, err := partial.Cancel()
	if err != nil || cancelled.Status() != payments.Cancelled || cancelled.Remaining().Minor() != 60 || cancelled.Version() != 3 {
		t.Fatalf("cancel: %v, %v", cancelled, err)
	}
	if overdue, err := cancelled.IsOverdue("2026-10-25"); err != nil || overdue {
		t.Fatalf("cancelled obligation is overdue: %v, %v", overdue, err)
	}
	if _, _, err := cancelled.Match(uuid.MustParse("019a0c65-9800-7000-8000-00000000000e"), fact, rub(t, 40), rub(t, 1), at); !errors.Is(err, payments.ErrState) {
		t.Fatalf("cancelled payment matched: %v", err)
	}
}
