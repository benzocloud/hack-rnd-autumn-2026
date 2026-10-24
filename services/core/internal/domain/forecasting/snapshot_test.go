package forecasting_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func snapshotData(t *testing.T) forecasting.SnapshotData {
	t.Helper()
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	amount, err := money.New(123, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	balance, err := ledger.NewBalanceObservation(scope, "2026-10-23", amount, ledger.BalanceSource{Kind: ledger.ManualEOD, RecordedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := ledger.NewTransaction(ledger.TransactionData{ID: uuid.New(), Scope: scope, Date: "2026-10-23", Direction: ledger.Inflow, Amount: amount, Source: ledger.Source{Kind: ledger.Manual}})
	if err != nil {
		t.Fatal(err)
	}
	return forecasting.SnapshotData{ID: uuid.New(), Scope: scope, DataVersion: 3, Cutoff: "2026-10-23", CreatedAt: at,
		Opening: forecasting.OpeningBalance{Observation: balance, DataVersion: 3},
		Facts:   []forecasting.Fact{{Transaction: fact, DataVersion: 2}}, Calendar: forecasting.Calendar{Reason: forecasting.HistoricalCalendarUnavailable}}
}

func TestSnapshotOwnsFactsAndExposesLimitations(t *testing.T) {
	data := snapshotData(t)
	snapshot, err := forecasting.NewFinancialSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	data.Facts[0] = forecasting.Fact{}
	got := snapshot.Data()
	got.Facts[0] = forecasting.Fact{}
	if snapshot.Data().Facts[0].Transaction.Data().ID == uuid.Nil {
		t.Fatal("snapshot aliases caller slice")
	}
	limits := snapshot.Limitations()
	if len(limits) != 2 || limits[0] != forecasting.HistoricalAvailabilityUnverified || limits[1] != forecasting.HistoricalCalendarUnavailable {
		t.Fatalf("missing historical limitations: %v", limits)
	}
}

func TestSnapshotRejectsMixedOrFutureInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*forecasting.SnapshotData)
	}{
		{"future fact", func(d *forecasting.SnapshotData) {
			d.Cutoff = "2026-10-22"
			amount := d.Opening.Observation.Amount()
			d.Opening.Observation, _ = ledger.NewBalanceObservation(d.Scope, d.Cutoff, amount, d.Opening.Observation.Source())
		}},
		{"foreign scope", func(d *forecasting.SnapshotData) { d.Scope.CompanyID = uuid.New() }},
		{"newer fact revision", func(d *forecasting.SnapshotData) { d.Facts[0].DataVersion = 4 }},
		{"newer balance revision", func(d *forecasting.SnapshotData) { d.Opening.DataVersion = 4 }},
		{"wrong EOD date", func(d *forecasting.SnapshotData) { d.Cutoff = "2026-10-24" }},
		{"duplicate fact", func(d *forecasting.SnapshotData) { d.Facts = append(d.Facts, d.Facts[0]) }},
		{"silent calendar", func(d *forecasting.SnapshotData) { d.Calendar.Reason = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := snapshotData(t)
			tc.change(&d)
			_, err := forecasting.NewFinancialSnapshot(d)
			if !errors.Is(err, forecasting.ErrInvalid) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
