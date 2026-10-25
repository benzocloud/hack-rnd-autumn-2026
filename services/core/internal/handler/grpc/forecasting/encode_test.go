package forecasting

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"

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

func TestKnownComponentsExposeOnlyFrozenRemaining(t *testing.T) {
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	amount, _ := money.New(100, money.RUB)
	remaining, _ := money.New(40, money.RUB)
	id := uuid.New()
	payment, err := payments.RestoreExpectedPayment(payments.PaymentData{ID: id, Scope: scope, Direction: ledger.Outflow, Amount: amount, ScheduledDate: "2026-10-30", Source: ledger.Source{Kind: ledger.Manual}, CreatedAt: time.Now()}, remaining, payments.PartiallyMatched, 2)
	if err != nil {
		t.Fatal(err)
	}
	out := &pb.ForecastResult{}
	saved := &service.SavedForecast{Components: []service.FinancialComponent{{ID: "known:" + id.String(), Kind: "known_remaining_payment", Date: "2026-10-30", OutflowMinor: 40, SourceIDs: []string{id.String()}}}}
	encodeKnownComponents(out, saved, domain.Calendar{Payments: []payments.ExpectedPayment{payment}})
	if len(out.Components) != 1 || len(out.EffectiveOccurrences) != 1 || len(out.Recommendations) != 1 {
		t.Fatal(out)
	}
	c := out.Components[0]
	if c.Amount.AmountMinor != 100 || c.RemainingAmount.AmountMinor != 40 || c.GetPaymentId() != id.String() || c.ScenarioEligible || c.Source.SourceSystem != "manual_v1" {
		t.Fatal(c)
	}
	if out.EffectiveOccurrences[0].RemainingAmount.AmountMinor != 40 || !out.EffectiveOccurrences[0].InHorizon || out.EffectiveOccurrences[0].ResponseActionEligible {
		t.Fatal(out.EffectiveOccurrences)
	}
}
