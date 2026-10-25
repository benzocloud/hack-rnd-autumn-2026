package payments_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

func TestChangePreservesAllocationsAndCancellation(t *testing.T) {
	p, fact := fixtures(t)
	amount := rub(t, 150)
	changed, err := p.Update(payments.PaymentChange{Amount: &amount})
	if err != nil || changed.Remaining().Minor() != 150 || changed.Version() != 2 || p.Remaining().Minor() != 100 {
		t.Fatal(changed, err)
	}
	date := ledger.Date("2026-10-29")
	changed, err = changed.Update(payments.PaymentChange{ScheduledDate: &date})
	if err != nil || changed.Data().ScheduledDate != date {
		t.Fatal(changed, err)
	}
	partial, m, err := changed.Match(uuid.New(), fact, rub(t, 0), rub(t, 40), time.Now())
	if err != nil || partial.Remaining().Minor() != 110 {
		t.Fatal(partial, err)
	}
	if _, err := partial.Update(payments.PaymentChange{Amount: &amount}); !errors.Is(err, payments.ErrState) {
		t.Fatal(err)
	}
	restored, err := partial.RemoveMatch(m)
	if err != nil || restored.Remaining().Minor() != 150 || restored.Status() != payments.Planned {
		t.Fatal(restored, err)
	}
	cancelled, err := partial.Cancel()
	if err != nil || cancelled.Remaining().Minor() != 110 {
		t.Fatal(cancelled, err)
	}
	if _, err := cancelled.RemoveMatch(m); !errors.Is(err, payments.ErrState) {
		t.Fatal("cancelled payment must retain allocations", err)
	}
	if _, err := cancelled.Update(payments.PaymentChange{ScheduledDate: &date}); !errors.Is(err, payments.ErrState) {
		t.Fatal(err)
	}
	if _, err := p.Update(payments.PaymentChange{}); !errors.Is(err, payments.ErrInvalid) {
		t.Fatal(err)
	}
}
