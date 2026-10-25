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

func TestMonthlyFitKeepsAmountThresholdsAndExactLargeMoney(t *testing.T) {
	for _, tc := range []struct {
		name    string
		amounts []int64
		valid   bool
	}{
		{"stable", []int64{100, 100, 100, 100}, true},
		{"mad boundary", []int64{70, 70, 130, 130}, true},
		{"mad exceeds boundary", []int64{69, 69, 131, 131}, false},
		{"review forged pattern", []int64{100, 100000, 100, 100000}, false},
		{"large exact", []int64{9223372036854775807, 9223372036854775807, 9223372036854775807}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if validAmountMAD(tc.amounts) != tc.valid {
				t.Fatal("MAD threshold changed")
			}
		})
	}
	input, a, _, _ := monthlyAnalysisFixture(t)
	facts := map[uuid.UUID]ledger.Transaction{}
	labels := map[uuid.UUID]OperationAnalysis{}
	for n, f := range input.Operations {
		facts[f.Data().ID] = f
		labels[f.Data().ID] = a.Operations[n]
	}
	if !validMonthlyFloor(a.Hypotheses[0], facts, labels) {
		t.Fatal("valid floor rejected")
	}
	// An omitted same-day fact cannot be silently excluded from aggregate evidence.
	original := input.Operations[0].Data()
	original.ID = uuid.New()
	extra, err := ledger.NewTransaction(original)
	if err != nil {
		t.Fatal(err)
	}
	facts[original.ID] = extra
	labels[original.ID] = OperationAnalysis{ID: original.ID, Version: extra.Version(), CategoryCode: "rent", CategorySource: "rules", RuleID: "rental_text"}
	if validMonthlyFloor(a.Hypotheses[0], facts, labels) {
		t.Fatal("incomplete same-day evidence accepted")
	}
	delete(facts, original.ID)
	delete(labels, original.ID)
	// A large event elsewhere in the same party/month raises the helper's floor.
	original.ID = uuid.New()
	original.Date = "2026-07-20"
	original.Amount, _ = money.New(100000, money.RUB)
	extra, err = ledger.NewTransaction(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range []ledger.Date{"2026-07-20", "2026-08-20", "2026-09-20"} {
		d := extra.Data()
		d.ID = uuid.New()
		d.Date = date
		record, err := ledger.NewTransaction(d)
		if err != nil {
			t.Fatal(err)
		}
		facts[d.ID] = record
		labels[d.ID] = OperationAnalysis{ID: d.ID, Version: record.Version(), CategoryCode: "rent", CategorySource: "rules", RuleID: "rental_text"}
	}
	if validMonthlyFloor(a.Hypotheses[0], facts, labels) {
		t.Fatal("below-floor events confirmed")
	}
}

func TestAnalysisRejectsForgedMonthlyAmountVariationOnRuntimeAndRead(t *testing.T) {
	for _, unstable := range []bool{false, true} {
		t.Run(fmt.Sprintf("unstable=%v", unstable), func(t *testing.T) {
			input, _, _, frozen := monthlyAnalysisFixture(t)
			input.Snapshot.Cutoff = "2026-10-01"
			input.Assessment.From = "2026-07-04"
			input.Assessment.Through = input.Snapshot.Cutoff
			input.Operations = nil
			a := &Analysis{Version: AnalysisVersion, From: input.Assessment.From, Through: input.Snapshot.Cutoff}
			h := RecurringHypothesis{RuleID: "monthly_fit", RuleVersion: AnalysisVersion, CategoryCode: "customer_payment", Direction: ledger.Inflow, DayOfMonth: 1, WeekendShift: "none", AmountMinor: 100, Confirmations: 4, ExpectedPeriods: 4, First: "2026-07-05", Last: "2026-10-01", Status: "hypothesis"}
			facts := []domain.Fact{}
			for n, date := range []ledger.Date{"2026-07-05", "2026-08-01", "2026-09-01", "2026-10-01"} {
				raw := int64(100)
				if unstable && n%2 == 1 {
					raw = 100000
				}
				amount, _ := money.New(raw, money.RUB)
				op, err := ledger.NewTransaction(ledger.TransactionData{ID: uuid.New(), Scope: input.Snapshot.Scope, Date: date, Direction: ledger.Inflow, Amount: amount, CounterpartyINN: "0012345678", Purpose: "Оплата", Source: ledger.Source{Kind: ledger.Manual}})
				if err != nil {
					t.Fatal(err)
				}
				input.Operations = append(input.Operations, op)
				facts = append(facts, domain.Fact{Transaction: op, DataVersion: 1})
				a.Operations = append(a.Operations, OperationAnalysis{ID: op.Data().ID, Version: 1, CategoryCode: "customer_payment", CategorySource: "rules", RuleID: "generic_customer_payment"})
				h.Operations = append(h.Operations, OperationReference{ID: op.Data().ID, Version: 1})
			}
			if unstable {
				h.AmountMinor = 100000
			}
			slices.SortFunc(h.Operations, func(a, b OperationReference) int { return strings.Compare(a.ID.String(), b.ID.String()) })
			ids := []string{}
			for _, ref := range h.Operations {
				ids = append(ids, ref.ID.String())
			}
			h.ID = fmt.Sprintf("monthly_%x", sha256.Sum256([]byte(strings.Join(append([]string{input.Snapshot.SnapshotID.String(), "monthly_fit", AnalysisVersion}, ids...), "\n"))))
			a.Hypotheses = []RecurringHypothesis{h}
			value, _ := money.New(100000, money.RUB)
			opening, err := ledger.NewBalanceObservation(input.Snapshot.Scope, input.Snapshot.Cutoff, value, ledger.BalanceSource{Kind: ledger.ManualEOD, RecordedAt: time.Now().UTC()})
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := domain.NewFinancialSnapshot(domain.SnapshotData{ID: input.Snapshot.SnapshotID, Scope: input.Snapshot.Scope, DataVersion: 1, Cutoff: input.Snapshot.Cutoff, CreatedAt: time.Now().UTC(), Opening: domain.OpeningBalance{Observation: opening, DataVersion: 1}, Facts: facts, Calendar: domain.Calendar{Reason: domain.HistoricalCalendarUnavailable}})
			if err != nil {
				t.Fatal(err)
			}
			frozen.Cutoff = input.Snapshot.Cutoff
			frozen.Assessment = input.Assessment
			if (ValidateAnalysis(input, a) != nil) != unstable || (validateSavedAnalysis(frozen, snapshot, a) != nil) != unstable {
				t.Fatal("runtime/read monthly fit threshold mismatch")
			}
		})
	}
}
