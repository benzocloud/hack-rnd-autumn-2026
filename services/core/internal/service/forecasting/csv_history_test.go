package forecasting

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func csvHistoryFixture(t *testing.T) (domain.FinancialSnapshot, []domain.OpeningBalance) {
	t.Helper()
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	namespace := ledger.Namespace{System: "dataset_v1", CompanyID: "C001"}
	var facts []domain.Fact
	for n, raw := range []struct {
		date      ledger.Date
		amount    int64
		direction ledger.Direction
	}{{"2026-07-02", 9000, ledger.Inflow}, {"2026-09-29", 4500, ledger.Outflow}} {
		amount, _ := money.New(raw.amount, money.RUB)
		fact, err := ledger.NewTransaction(ledger.TransactionData{ID: uuid.New(), Scope: scope, Date: raw.date, Direction: raw.direction, Amount: amount, Source: ledger.Source{Kind: ledger.Imported, Namespace: namespace, ExternalID: []string{"first", "last"}[n], ImportedAt: now}})
		if err != nil {
			t.Fatal(err)
		}
		facts = append(facts, domain.Fact{Transaction: fact, DataVersion: 2})
	}
	var balances []domain.OpeningBalance
	for _, raw := range []struct {
		date   ledger.Date
		amount int64
	}{{"2026-07-02", 100000}, {"2026-09-29", 95500}} {
		amount, _ := money.New(raw.amount, money.RUB)
		b, err := ledger.NewBalanceObservation(scope, raw.date, amount, ledger.BalanceSource{Kind: ledger.StatementEOD, Namespace: namespace, RecordedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		balances = append(balances, domain.OpeningBalance{Observation: b, DataVersion: 2})
	}
	snapshot, err := domain.NewFinancialSnapshot(domain.SnapshotData{ID: uuid.New(), Scope: scope, DataVersion: 2, Cutoff: "2026-09-29", CreatedAt: now, Opening: balances[1], Facts: facts, Calendar: domain.Calendar{Reason: domain.HistoricalCalendarUnavailable}})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, balances
}

func TestCSVAssessmentDoesNotCertifyMissingDays(t *testing.T) {
	snapshot, balances := csvHistoryFixture(t)
	a, err := assessCSVHistory(snapshot, balances)
	if err != nil || a.OperationDays != 2 || len(a.Balances) != 2 || !validCSVAssessment(a, snapshot) {
		t.Fatal(a, err)
	}
	a.Balances[0].AmountMinor++
	if validCSVAssessment(a, snapshot) {
		t.Fatal("tampered EOD accepted")
	}
	if balances[0].Observation.Amount().Minor() != 100000 {
		t.Fatal("assessment aliases source evidence")
	}
}

func TestCSVAssessmentAcceptsManualSupplement(t *testing.T) {
	snapshot, balances := csvHistoryFixture(t)
	d := snapshot.Data()
	manual := d.Facts[1].Transaction.Data()
	manual.Source = ledger.Source{Kind: ledger.Manual}
	record, err := ledger.NewTransaction(manual)
	if err != nil {
		t.Fatal(err)
	}
	d.Facts[1].Transaction = record
	mixed, err := domain.NewFinancialSnapshot(d)
	if err != nil {
		t.Fatal(err)
	}
	a, err := assessCSVHistory(mixed, balances)
	if err != nil || !validCSVAssessment(a, mixed) || a.OperationDays != 2 {
		t.Fatal(a, err)
	}
}

func TestCSVAssessmentRejectsShortStaleAndCrossScopeInputs(t *testing.T) {
	snapshot, balances := csvHistoryFixture(t)
	d := snapshot.Data()
	short := d
	short.Facts = short.Facts[1:]
	shortSnapshot, err := domain.NewFinancialSnapshot(short)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assessCSVHistory(shortSnapshot, balances); !errors.Is(err, ErrHistoryTooShort) {
		t.Fatal(err)
	}
	a, err := assessCSVHistory(snapshot, balances)
	if err != nil {
		t.Fatal(err)
	}
	a.DataVersion++
	if validCSVAssessment(a, snapshot) {
		t.Fatal("stale assessment accepted")
	}
	a.DataVersion--
	a.Scope.AccountID = uuid.New()
	if validCSVAssessment(a, snapshot) {
		t.Fatal("foreign assessment accepted")
	}
	balances[0].DataVersion = 3
	if _, err := assessCSVHistory(snapshot, balances); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestCSVAssessmentRejectsReconciliationGapAndMissingCutoff(t *testing.T) {
	snapshot, balances := csvHistoryFixture(t)
	a, err := assessCSVHistory(snapshot, balances)
	if err != nil {
		t.Fatal(err)
	}
	a.Balances[0].AmountMinor++
	if _, err := assessCSVBalances(snapshot, a.From, a.Balances); !errors.Is(err, ErrHistoryReconciliation) {
		t.Fatal(err)
	}
	if _, err := assessCSVHistory(snapshot, balances[:1]); !errors.Is(err, ErrMissingBalance) {
		t.Fatal(err)
	}
}
