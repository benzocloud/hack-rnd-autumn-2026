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

func TestAllocationGuards(t *testing.T) {
	t.Parallel()
	payment, fact := fixtures(t)
	at := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		allocated int64
		amount    int64
		want      error
	}{{90, 11, payments.ErrAllocation}, {0, 101, payments.ErrAllocation}, {101, 1, payments.ErrAllocation},
		{-1, 1, money.ErrAmount}, {0, 0, money.ErrAmount}, {0, -1, money.ErrAmount}} {
		if _, _, err := payment.Match(uuid.MustParse("019a0c65-9800-7000-8000-00000000000a"), fact, rub(t, tc.allocated), rub(t, tc.amount), at); !errors.Is(err, tc.want) {
			t.Errorf("allocated=%d amount=%d: %v", tc.allocated, tc.amount, err)
		}
	}
	partial, _, err := payment.Match(uuid.MustParse("019a0c65-9800-7000-8000-00000000000f"), fact, rub(t, 0), rub(t, 70), at)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := partial.Match(uuid.MustParse("019a0c65-9800-7000-8000-000000000010"), fact, rub(t, 0), rub(t, 31), at); !errors.Is(err, payments.ErrAllocation) {
		t.Fatalf("expected-payment side overallocated: %v", err)
	}
	for _, tc := range []struct {
		edit func(*ledger.TransactionData)
		want error
	}{
		{func(d *ledger.TransactionData) {
			d.Scope.CompanyID = uuid.MustParse("019a0c65-9800-7000-8000-000000000003")
		}, ledger.ErrScope},
		{func(d *ledger.TransactionData) {
			d.Scope.AccountID = uuid.MustParse("019a0c65-9800-7000-8000-000000000003")
		}, ledger.ErrScope},
		{func(d *ledger.TransactionData) { d.Direction = ledger.Inflow }, payments.ErrDirection},
	} {
		data := fact.Data()
		tc.edit(&data)
		other, err := ledger.NewTransaction(data)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := payment.Match(uuid.MustParse("019a0c65-9800-7000-8000-00000000000a"), other, rub(t, 0), rub(t, 1), at); !errors.Is(err, tc.want) {
			t.Fatalf("scope/direction guard: %v", err)
		}
	}
}
