package forecasting

import (
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	service "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func TestObservedDaysCountDistinctCoveredFacts(t *testing.T) {
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	amount, err := money.New(100, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	balance, err := ledger.NewBalanceObservation(scope, "2026-10-23", amount, ledger.BalanceSource{Kind: ledger.ManualEOD, RecordedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	fact := func(date string) domain.Fact {
		t.Helper()
		transaction, err := ledger.NewTransaction(ledger.TransactionData{ID: uuid.New(), Scope: scope, Date: ledger.Date(date), Direction: ledger.Inflow, Amount: amount, Source: ledger.Source{Kind: ledger.Manual}})
		if err != nil {
			t.Fatal(err)
		}
		return domain.Fact{Transaction: transaction, DataVersion: 2}
	}
	data := domain.SnapshotData{ID: uuid.New(), Scope: scope, DataVersion: 3, Cutoff: "2026-10-23", CreatedAt: at,
		Opening:  domain.OpeningBalance{Observation: balance, DataVersion: 3},
		Facts:    []domain.Fact{fact("2026-10-23"), fact("2026-10-23"), fact("2026-10-22"), fact("2026-07-01")},
		Calendar: domain.Calendar{Reason: domain.HistoricalCalendarUnavailable}}
	snapshot, err := domain.NewFinancialSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	view := service.ForecastView{ID: uuid.New(), Scope: scope, CreatedAt: at, CurrentDataVersion: 3, DataVersion: 3, Cutoff: "2026-10-23", Snapshot: snapshot, ModelContextID: uuid.New(),
		Coverage: domain.JobCoverage{Kind: "certified", From: "2026-07-01", Through: "2026-10-23", Reference: "certified", DataVersion: 3}}
	if got := encodeForecast(view).Readiness.ObservedDays; got != 2 {
		t.Fatal(got)
	}
	view.Coverage.Kind = "legacy"
	if got := encodeForecast(view).Readiness.ObservedDays; got != 0 {
		t.Fatal(got)
	}
}
