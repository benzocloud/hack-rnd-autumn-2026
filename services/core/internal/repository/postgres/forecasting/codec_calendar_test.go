package forecasting

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

func TestCalendarCodecFreezesRemainingVersionAndIndependentGroups(t *testing.T) {
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	at := time.Now().UTC().Truncate(time.Microsecond)
	amount, _ := money.New(1000, money.RUB)
	remaining, _ := money.New(600, money.RUB)
	group := domain.ReconciliationGroup{ID: uuid.New(), Scope: scope, Version: 1, Status: "active", Cutoff: "2026-10-24", HorizonStart: "2026-10-25", HorizonEnd: "2026-11-23", ConfirmedBy: "actor", ConfirmedAt: at, Additional: true}
	payment, err := payments.RestoreExpectedPayment(payments.PaymentData{ID: uuid.New(), ReconciliationGroupID: group.ID, Scope: scope, Direction: ledger.Outflow, Amount: amount, ScheduledDate: "2026-10-27", Source: ledger.Source{Kind: ledger.Manual}, CreatedAt: at}, remaining, payments.PartiallyMatched, 2)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := ledger.NewBalanceObservation(scope, "2026-10-24", amount, ledger.BalanceSource{Kind: ledger.ManualEOD, RecordedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	data := domain.SnapshotData{ID: uuid.New(), Scope: scope, DataVersion: 3, Cutoff: "2026-10-24", CreatedAt: at, Opening: domain.OpeningBalance{Observation: observation, DataVersion: 1}, Facts: []domain.Fact{}, Calendar: domain.Calendar{Captured: true, KnowledgeMode: "current", AsOf: "2026-10-25", NoPostCutoffMovements: true, Payments: []payments.ExpectedPayment{payment}, Groups: []domain.ReconciliationGroup{group}}}
	snapshot, err := domain.NewFinancialSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := encodeSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := decodeSnapshot(data, 2, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.Data(), restored.Data()) {
		t.Fatal("calendar lost exact frozen fields")
	}
	changed := restored.Data()
	changed.Calendar.Groups[0].ConfirmedBy = "different"
	changed.Calendar.Payments = nil
	if restored.Data().Calendar.Groups[0].ConfirmedBy != "actor" || len(restored.Data().Calendar.Payments) != 1 {
		t.Fatal("mutable snapshot projection")
	}
	if _, err := decodeSnapshot(data, 1, payload); err == nil {
		t.Fatal("new calendar accepted as legacy codec")
	}
	cancelled, err := payment.Cancel()
	if err != nil {
		t.Fatal(err)
	}
	data.Calendar.Payments[0] = cancelled
	after, err := domain.NewFinancialSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Data().Calendar.Groups) != 1 || restored.Data().Calendar.Payments[0].Status() != payments.PartiallyMatched {
		t.Fatal("cancel mutated frozen state or independent mapping")
	}
}
