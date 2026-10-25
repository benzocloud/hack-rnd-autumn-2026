package ml

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func csvFixture(t *testing.T) forecasting.FinancialModelInput {
	t.Helper()
	i := inputFixture()
	from := i.History[0].Date
	i.History = nil
	i.KnownPayments = nil
	i.OpeningBalance = rub(95500)
	i.Model = forecasting.ModelContext{Name: "mean_90", Version: forecasting.CSVModelVersion, PreprocessingVersion: forecasting.CSVPreprocessing}
	namespace := ledger.Namespace{System: "dataset_v1", CompanyID: "C001"}
	now := time.Now().UTC()
	a := &forecasting.HistoryAssessment{RuleVersion: forecasting.CSVQualityVersion, Scope: i.Scope, DataVersion: i.DataVersion, From: from, Through: i.Cutoff, FirstOperationDate: from, OperationDays: 2, Limitations: []string{"csv_history_completeness_unverified", "unobserved_days_unknown", "observed_operations_mean_per_calendar_day", "historical_calendar_unavailable", "historical_availability_unverified", "not_calibrated"}}
	result := forecasting.FinancialModelInput{Snapshot: i, Assessment: a}
	for n, v := range []struct {
		date            ledger.Date
		amount, balance int64
		direction       ledger.Direction
	}{{from, 9000, 100000, ledger.Inflow}, {i.Cutoff, 4500, 95500, ledger.Outflow}} {
		op, err := ledger.NewTransaction(ledger.TransactionData{ID: uuid.New(), Scope: i.Scope, Date: v.date, Direction: v.direction, Amount: rub(v.amount), Source: ledger.Source{Kind: ledger.Imported, Namespace: namespace, ExternalID: []string{"first", "last"}[n], ImportedAt: now}})
		if err != nil {
			t.Fatal(err)
		}
		result.Operations = append(result.Operations, op)
		a.Balances = append(a.Balances, forecasting.HistoryBalance{Date: v.date, AmountMinor: v.balance, DataVersion: i.DataVersion, Source: ledger.BalanceSource{Kind: ledger.StatementEOD, Namespace: namespace, RecordedAt: now}})
	}
	return result
}

func TestCSVAdapterPreservesUnknownHistory(t *testing.T) {
	i := csvFixture(t)
	r, err := encodeFinancial(i)
	if err != nil {
		t.Fatal(err)
	}
	unknown := r.Snapshot.History[1]
	if unknown.Source != pb.DaySource_DAY_SOURCE_UNAVAILABLE || unknown.InflowMinor != nil || unknown.OutflowMinor != nil || unknown.ClosingBalanceMinor != nil || len(r.Snapshot.History) != 90 || r.Model.ModelVersion != forecasting.CSVModelVersion || r.CsvHistory == nil {
		t.Fatal("unknown day became a fact", r)
	}
	i.Coverage.Kind = "certified"
	if _, err := encodeFinancial(i); err == nil {
		t.Fatal("CSV accepted a bank completeness claim")
	}
}
