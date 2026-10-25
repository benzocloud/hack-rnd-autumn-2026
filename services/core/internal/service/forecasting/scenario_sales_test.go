package forecasting

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func salesView(t *testing.T) ForecastView {
	t.Helper()
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	categories, err := ledger.DefaultCategories(scope.CompanyID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]uuid.UUID{}
	for _, c := range categories {
		ids[c.CatalogueCode] = c.ID
	}
	result := &SavedForecast{SchemaVersion: 1, Analysis: &Analysis{Version: AnalysisVersion}}
	facts := []domain.Fact{}
	for _, tc := range []struct {
		code, source, manual string
		amount               int64
		direction            ledger.Direction
	}{
		{"customer_payment", "rules", "", 9000, ledger.Inflow}, {"loan_receipt", "rules", "", 90000, ledger.Inflow}, {"own_transfer", "rules", "", 90000, ledger.Inflow}, {"refund_in", "rules", "", 90000, ledger.Inflow}, {"unknown", "unknown", "", 90000, ledger.Inflow}, {"unknown", "manual", "sales", 9000, ledger.Inflow}, {"unknown", "manual", "financing", 90000, ledger.Inflow}, {"customer_payment", "rules", "", 9000, ledger.Outflow},
	} {
		amount, _ := money.New(tc.amount, money.RUB)
		data := ledger.TransactionData{ID: uuid.New(), Scope: scope, Date: "2026-09-28", Direction: tc.direction, Amount: amount, Source: ledger.Source{Kind: ledger.Manual}}
		if tc.manual != "" {
			data.Category = ledger.Category{CompanyID: scope.CompanyID, ID: ids[tc.manual]}
		}
		transaction, err := ledger.NewTransaction(data)
		if err != nil {
			t.Fatal(err)
		}
		facts = append(facts, domain.Fact{Transaction: transaction, DataVersion: 1})
		result.Analysis.Operations = append(result.Analysis.Operations, OperationAnalysis{ID: data.ID, Version: 1, CategoryCode: tc.code, CategorySource: tc.source, CategoryID: ids[tc.manual]})
	}
	now := time.Now().UTC()
	amount, _ := money.New(100000, money.RUB)
	observation, err := ledger.NewBalanceObservation(scope, "2026-09-29", amount, ledger.BalanceSource{Kind: ledger.ManualEOD, RecordedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := domain.NewFinancialSnapshot(domain.SnapshotData{ID: uuid.New(), Scope: scope, DataVersion: 1, Cutoff: "2026-09-29", CreatedAt: now, Opening: domain.OpeningBalance{Observation: observation, DataVersion: 1}, Facts: facts, Calendar: domain.Calendar{Reason: domain.HistoricalCalendarUnavailable}})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	for n := range 30 {
		result.Days = append(result.Days, SavedForecastDay{Date: ledger.Date(start.AddDate(0, 0, n).Format(time.DateOnly)), Inflow: 10000, Outflow: 10000, Balance: 100000})
	}
	return ForecastView{ID: uuid.New(), Scope: scope, Status: "succeeded", Cutoff: "2026-09-29", Snapshot: snapshot, Assessment: &HistoryAssessment{From: "2026-07-02", Through: "2026-09-29"}, Model: ModelContext{Name: "mean_90", Version: CSVAnalysisVersion}, Categories: categories, Result: result}
}
func TestScenarioSalesExcludeOtherFlowsAndHonorManualCategory(t *testing.T) {
	view := salesView(t)
	targets := ScenarioSales(view)
	if len(targets) != 30 || targets[0].AmountMinor != 200 || targets[29].RemainingMinor != 200 {
		t.Fatal("non-sales included or manual category lost", targets)
	}
	if ScenarioSales(view)[0].ID != targets[0].ID {
		t.Fatal("unstable target identity")
	}
	view.Result.Analysis = nil
	if len(ScenarioSales(view)) != 0 {
		t.Fatal("unclassified inflows enabled")
	}
}
func TestScenarioSalesUsesFrozenCalendarResidualOnly(t *testing.T) {
	view := salesView(t)
	view.Model.Version = CSVCalendarAnalysisVersion
	date := view.Result.Days[0].Date
	view.Result.Components = []FinancialComponent{{ID: "sales", Kind: "mean_90_uncovered", CategoryCode: "customer_payment", Date: date, InflowMinor: 123}, {ID: "known-sale", Kind: "known_remaining_payment", CategoryCode: "customer_payment", Date: date, InflowMinor: 5000}, {ID: "loan", Kind: "mean_90_uncovered", CategoryCode: "loan_receipt", Date: date, InflowMinor: 2000}}
	targets := ScenarioSales(view)
	if len(targets) != 1 || targets[0].AmountMinor != 123 {
		t.Fatal("calendar payment or loan treated as sales", targets)
	}
}
func TestSalesReductionPeriodTargetsRoundingAndNoCompounding(t *testing.T) {
	view := salesView(t)
	basis := ScenarioSales(view)
	targets := []SalesTarget{}
	for _, c := range basis {
		targets = append(targets, SalesTarget{ComponentID: c.ID, OccurrenceID: c.ID})
	}
	change := SalesReductionChange{ID: uuid.New(), Targets: targets, PeriodStart: basis[2].Date, PeriodEnd: basis[4].Date, ReductionBasisPoints: 2500}
	result, err := ApplyScenario(*view.Result, nil)
	if err != nil {
		t.Fatal(err)
	}
	reduced, remaining, err := applySalesReductions(result, basis, []SalesReductionChange{change})
	if err != nil {
		t.Fatal(err)
	}
	for n, d := range reduced.Days {
		loss := int64(0)
		if n >= 2 && n <= 4 {
			loss = 50
		}
		cumulative := int64(max(0, min(n-1, 3))) * 50
		if d.Inflow != 10000-loss || d.Outflow != 10000 || d.Balance != 100000-cumulative {
			t.Fatal("incorrect interval reduction", n, d)
		}
	}
	if basis[2].RemainingMinor != 200 || remaining[2].RemainingMinor != 150 || view.Result.Days[4].Balance != 100000 {
		t.Fatal("base or sales basis mutated")
	}
	for _, mode := range []string{"foreign target", "duplicate target", "overlap", "percent", "date"} {
		t.Run(mode, func(t *testing.T) {
			bad := change
			bad.Targets = append([]SalesTarget(nil), change.Targets...)
			changes := []SalesReductionChange{bad}
			switch mode {
			case "foreign target":
				bad.Targets[0] = SalesTarget{ComponentID: "known-payment", OccurrenceID: "known-payment"}
			case "duplicate target":
				bad.Targets = append(bad.Targets, bad.Targets[0])
			case "overlap":
				changes = append(changes, bad)
			case "percent":
				bad.ReductionBasisPoints = 10001
			case "date":
				bad.PeriodStart = "2026-09-01"
			}
			changes[0] = bad
			copyResult, _ := ApplyScenario(*view.Result, nil)
			if _, _, err := applySalesReductions(copyResult, basis, changes); err == nil {
				t.Fatal("unsafe reduction accepted")
			}
		})
	}
	one := basis[0]
	one.AmountMinor = 1
	one.RemainingMinor = 1
	roundChange := SalesReductionChange{ID: uuid.New(), Targets: []SalesTarget{{ComponentID: one.ID, OccurrenceID: one.ID}}, PeriodStart: one.Date, PeriodEnd: one.Date, ReductionBasisPoints: 5000}
	copyResult, _ := ApplyScenario(*view.Result, nil)
	rounded, _, err := applySalesReductions(copyResult, []SalesComponent{one}, []SalesReductionChange{roundChange})
	if err != nil || rounded.Days[0].Inflow != 9999 {
		t.Fatal("half-up kopeck rounding", err)
	}
	copyResult, _ = ApplyScenario(*view.Result, nil)
	copyResult.Days[1].Balance = math.MinInt64 + 1
	roundChange.ReductionBasisPoints = 10000
	one.RemainingMinor = 2
	if _, _, err := applySalesReductions(copyResult, []SalesComponent{one}, []SalesReductionChange{roundChange}); err == nil {
		t.Fatal("balance overflow accepted")
	}
}
