package forecasting

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func monthlyAnalysisFixture(t *testing.T) (FinancialModelInput, *Analysis, domain.FinancialSnapshot, jobInput) {
	t.Helper()
	scope := ledger.Scope{CompanyID: uuid.New(), AccountID: uuid.New()}
	snapshotID := uuid.New()
	now := time.Now().UTC()
	model := ModelContext{Name: "mean_90", Version: CSVAnalysisVersion, PreprocessingVersion: CSVAnalysisPreprocessing}
	input := FinancialModelInput{Snapshot: ModelInput{SnapshotID: snapshotID, Scope: scope, Model: model, Cutoff: "2026-09-29"}, Assessment: &HistoryAssessment{From: "2026-07-02", Through: "2026-09-29"}}
	analysis := &Analysis{Version: AnalysisVersion, From: input.Assessment.From, Through: input.Snapshot.Cutoff}
	h := RecurringHypothesis{RuleID: "monthly_fit", RuleVersion: AnalysisVersion, CategoryCode: "rent", Direction: ledger.Outflow, DayOfMonth: 5, WeekendShift: "none", AmountMinor: 900, Confirmations: 3, ExpectedPeriods: 3, First: "2026-07-05", Last: "2026-09-05", Status: "hypothesis"}
	facts := []domain.Fact{}
	for _, date := range []ledger.Date{"2026-07-05", "2026-08-05", "2026-09-05"} {
		amount, _ := money.New(900, money.RUB)
		op, err := ledger.NewTransaction(ledger.TransactionData{ID: uuid.New(), Scope: scope, Date: date, Direction: ledger.Outflow, Amount: amount, CounterpartyINN: "0012345678", CounterpartyName: "Арендодатель", Purpose: "Аренда помещения", Source: ledger.Source{Kind: ledger.Manual}})
		if err != nil {
			t.Fatal(err)
		}
		input.Operations = append(input.Operations, op)
		facts = append(facts, domain.Fact{Transaction: op, DataVersion: 1})
		analysis.Operations = append(analysis.Operations, OperationAnalysis{ID: op.Data().ID, Version: op.Version(), CategoryCode: "rent", CategorySource: "rules", RuleID: "rental_text"})
		h.Operations = append(h.Operations, OperationReference{ID: op.Data().ID, Version: op.Version()})
	}
	slices.SortFunc(h.Operations, func(a, b OperationReference) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	ids := []string{}
	for _, ref := range h.Operations {
		ids = append(ids, ref.ID.String())
	}
	h.ID = fmt.Sprintf("monthly_%x", sha256.Sum256([]byte(strings.Join(append([]string{snapshotID.String(), "monthly_fit", AnalysisVersion}, ids...), "\n"))))
	analysis.Hypotheses = []RecurringHypothesis{h}
	openingAmount, _ := money.New(100000, money.RUB)
	opening, err := ledger.NewBalanceObservation(scope, input.Snapshot.Cutoff, openingAmount, ledger.BalanceSource{Kind: ledger.ManualEOD, RecordedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := domain.NewFinancialSnapshot(domain.SnapshotData{ID: snapshotID, CreatedAt: now, Scope: scope, DataVersion: 1, Cutoff: input.Snapshot.Cutoff, Opening: domain.OpeningBalance{Observation: opening, DataVersion: 1}, Facts: facts, Calendar: domain.Calendar{Reason: domain.HistoricalCalendarUnavailable}})
	if err != nil {
		t.Fatal(err)
	}
	frozen := jobInput{Model: model, SnapshotID: snapshotID, Scope: scope, Cutoff: input.Snapshot.Cutoff, Assessment: input.Assessment}
	return input, analysis, snapshot, frozen
}

func TestAnalysisDurableReadRejectsCorruptedEvidence(t *testing.T) {
	input, a, snapshot, frozen := monthlyAnalysisFixture(t)
	if err := ValidateAnalysis(input, a); err != nil {
		t.Fatal(err)
	}
	if err := validateSavedAnalysis(frozen, snapshot, a); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*Analysis){
		"category":          func(a *Analysis) { a.Hypotheses[0].CategoryCode = "tax" },
		"rule":              func(a *Analysis) { a.Hypotheses[0].RuleVersion = "unknown" },
		"amount":            func(a *Analysis) { a.Hypotheses[0].AmountMinor++ },
		"anchor":            func(a *Analysis) { a.Hypotheses[0].DayOfMonth = 20 },
		"shift":             func(a *Analysis) { a.Hypotheses[0].WeekendShift = "unknown" },
		"confirmations":     func(a *Analysis) { a.Hypotheses[0].Confirmations = 2 },
		"periods":           func(a *Analysis) { a.Hypotheses[0].ExpectedPeriods = 4 },
		"status":            func(a *Analysis) { a.Hypotheses[0].Status = "payment" },
		"pattern id":        func(a *Analysis) { a.Hypotheses[0].ID = "forged" },
		"foreign ref":       func(a *Analysis) { a.Hypotheses[0].Operations[0].ID = uuid.New() },
		"stale ref":         func(a *Analysis) { a.Hypotheses[0].Operations[0].Version++ },
		"duplicate ref":     func(a *Analysis) { a.Hypotheses[0].Operations[1] = a.Hypotheses[0].Operations[0] },
		"duplicate pattern": func(a *Analysis) { a.Hypotheses = append(a.Hypotheses, a.Hypotheses[0]) },
		"excluded category": func(a *Analysis) {
			a.Hypotheses[0].CategoryCode = "own_transfer"
			for n := range a.Operations {
				a.Operations[n].CategoryCode = "own_transfer"
				a.Operations[n].RuleID = "own_funds_transfer"
			}
		},
		"manual source":           func(a *Analysis) { a.Operations[0].CategorySource = "manual" },
		"derived rule":            func(a *Analysis) { a.Operations[0].RuleID = "unknown" },
		"foreign tenant category": func(a *Analysis) { a.Operations[0].CategoryID = uuid.New() },
		"unknown version":         func(a *Analysis) { a.Version = "unknown" },
		"wrong window":            func(a *Analysis) { a.From = "2026-06-01" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(a)
			if err != nil {
				t.Fatal(err)
			}
			var damaged Analysis
			if err := json.Unmarshal(encoded, &damaged); err != nil {
				t.Fatal(err)
			}
			mutate(&damaged)
			if ValidateAnalysis(input, &damaged) == nil || validateSavedAnalysis(frozen, snapshot, &damaged) == nil {
				t.Fatal("corrupted runtime/persisted analysis accepted")
			}
		})
	}
	// Facts from another party remain valid Ledger facts but cannot confirm this pattern.
	data := input.Operations[0].Data()
	data.CounterpartyINN = "0098765432"
	other, err := ledger.RestoreTransaction(data, input.Operations[0].Version())
	if err != nil {
		t.Fatal(err)
	}
	input.Operations[0] = other
	if ValidateAnalysis(input, a) == nil {
		t.Fatal("mixed parties accepted")
	}
}
