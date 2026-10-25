package forecasting

import (
	"slices"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

const AnalysisVersion = "text_categories_monthly_v1"
const CSVAnalysisVersion = "mean_90_csv_analysis_v2"
const CSVAnalysisPreprocessing = "csv_observed_operations_analysis_v2"
const CSVCalendarAnalysisVersion = "mean_90_csv_calendar_analysis_v2"
const CSVCalendarAnalysisPreprocessing = "csv_observed_operations_calendar_analysis_v2"

func AnalysisProfile(version string) bool {
	return version == CSVAnalysisVersion || version == CSVCalendarAnalysisVersion
}
func CSVCalendarProfile(version string) bool {
	return version == CSVCalendarModelVersion || version == CSVCalendarAnalysisVersion
}
func ValidCSVContext(m ModelContext) bool {
	for _, pair := range [][2]string{{CSVModelVersion, CSVPreprocessing}, {CSVCalendarModelVersion, CSVCalendarPreprocessing}, {CSVAnalysisVersion, CSVAnalysisPreprocessing}, {CSVCalendarAnalysisVersion, CSVCalendarAnalysisPreprocessing}} {
		if m == (ModelContext{Name: "mean_90", Version: pair[0], PreprocessingVersion: pair[1]}) {
			return true
		}
	}
	return false
}

type Analysis struct {
	Version       string
	From, Through ledger.Date
	Operations    []OperationAnalysis
	Hypotheses    []RecurringHypothesis
}
type OperationAnalysis struct {
	ID                                   uuid.UUID
	Version                              int64
	CategoryCode, CategorySource, RuleID string
	CategoryID                           uuid.UUID
}
type OperationReference struct {
	ID      uuid.UUID
	Version int64
}
type RecurringHypothesis struct {
	ID, RuleID, RuleVersion, CategoryCode string
	Direction                             ledger.Direction
	Operations                            []OperationReference
	DayOfMonth                            uint32
	MonthEnd                              bool
	WeekendShift                          string
	AmountMinor                           int64
	Confirmations, ExpectedPeriods        uint32
	First, Last                           ledger.Date
	Status                                string
}

// Only exact catalogue codes with a defined mapping obtain a tenant UUID.
// Unmapped model labels stay semantic codes: never guess by display name.
func AnalysisCategory(code string, direction ledger.Direction, categories []ledger.CategoryDefinition) uuid.UUID {
	catalogue := ""
	switch code {
	case "tax":
		catalogue = "tax"
	case "operating_expense":
		catalogue = "operating"
	case "customer_payment", "agency_revenue", "acquiring_revenue", "cash_revenue", "marketplace_revenue":
		catalogue = "sales"
	case "owner_capital", "owner_financing", "owner_repayment", "loan_receipt", "loan_payment":
		catalogue = "financing"
	case "own_transfer":
		catalogue = "own_transfer"
	}
	for _, c := range categories {
		if catalogue != "" && c.CatalogueCode == catalogue && c.Allows(direction) {
			return c.ID
		}
	}
	return uuid.Nil
}

func validateSavedAnalysis(frozen jobInput, snapshot domain.FinancialSnapshot, analysis *Analysis) error {
	if !AnalysisProfile(frozen.Model.Version) {
		if analysis != nil {
			return ErrModelResponse
		}
		return nil
	}
	if analysis == nil || frozen.Assessment == nil || analysis.Version != AnalysisVersion || analysis.From != frozen.Assessment.From || analysis.Through != frozen.Cutoff || len(analysis.Operations) > 20000 || len(analysis.Hypotheses) > 1000 || len(frozen.Categories) > 1000 {
		return ErrModelResponse
	}
	categories := map[uuid.UUID]bool{}
	codes := map[string]bool{}
	for _, c := range frozen.Categories {
		if c.ID == uuid.Nil || c.CompanyID != frozen.Scope.CompanyID || categories[c.ID] || codes[c.CatalogueCode] {
			return ErrModelResponse
		}
		categories[c.ID] = true
		codes[c.CatalogueCode] = true
	}
	facts := map[uuid.UUID]ledger.Transaction{}
	for _, f := range snapshot.Data().Facts {
		d := f.Transaction.Data()
		if d.Date >= analysis.From && d.Date <= analysis.Through {
			facts[d.ID] = f.Transaction
		}
	}
	if len(facts) != len(analysis.Operations) {
		return ErrModelResponse
	}
	seen := map[uuid.UUID]bool{}
	for _, op := range analysis.Operations {
		fact, ok := facts[op.ID]
		d := fact.Data()
		if !ok || seen[op.ID] || op.Version != fact.Version() {
			return ErrModelResponse
		}
		seen[op.ID] = true
		if d.Category.ID != uuid.Nil {
			if op.CategorySource != "manual" || op.CategoryID != d.Category.ID || op.CategoryCode != "unknown" || op.RuleID != "" {
				return ErrModelResponse
			}
		} else {
			if !ValidAnalysisLabel(op.CategoryCode, op.CategorySource, op.RuleID) || op.CategoryID != AnalysisCategory(op.CategoryCode, d.Direction, frozen.Categories) {
				return ErrModelResponse
			}
		}
	}
	refs := map[uuid.UUID]bool{}
	ids := map[string]bool{}
	total := 0
	for _, h := range analysis.Hypotheses {
		minimum := uint32(4)
		if h.Direction == ledger.Outflow && slices.Contains([]string{"salary", "salary_advance", "salary_final", "tax", "rent", "equipment_rent", "bank_fee", "utilities", "subscription", "loan_payment", "overdraft_interest"}, h.CategoryCode) {
			minimum = 3
		}
		if h.ID == "" || ids[h.ID] || h.RuleID != "monthly_fit" || h.RuleVersion != AnalysisVersion || h.Status != "hypothesis" || (h.Direction != ledger.Inflow && h.Direction != ledger.Outflow) || h.AmountMinor <= 0 || h.Confirmations < minimum || h.ExpectedPeriods < h.Confirmations || h.ExpectedPeriods > 5 || h.Confirmations*4 < h.ExpectedPeriods*3 || (h.MonthEnd && h.DayOfMonth != 0) || (!h.MonthEnd && (h.DayOfMonth < 1 || h.DayOfMonth > 31)) || !slices.Contains([]string{"none", "previous", "next"}, h.WeekendShift) || len(h.Operations) < int(h.Confirmations) || h.First < analysis.From || h.Last > analysis.Through || h.First > h.Last {
			return ErrModelResponse
		}
		ids[h.ID] = true
		total += len(h.Operations)
		if total > 20000 {
			return ErrModelResponse
		}
		first, last := ledger.Date("9999-12-31"), ledger.Date("0001-01-01")
		for _, ref := range h.Operations {
			fact, ok := facts[ref.ID]
			d := fact.Data()
			if !ok || refs[ref.ID] || ref.Version != fact.Version() || d.Direction != h.Direction {
				return ErrModelResponse
			}
			refs[ref.ID] = true
			first = min(first, d.Date)
			last = max(last, d.Date)
		}
		if first != h.First || last != h.Last {
			return ErrModelResponse
		}
		lastDate, err := time.Parse(time.DateOnly, string(h.Last))
		through, _ := time.Parse(time.DateOnly, string(analysis.Through))
		if err != nil || through.Sub(lastDate) > 45*24*time.Hour {
			return ErrModelResponse
		}
	}
	return nil
}

// Each semantic label is tied to its existing pure text rule; unknown is explicit.
func ValidAnalysisLabel(code, source, rule string) bool {
	if code == "unknown" {
		return source == "unknown" && (rule == "empty_purpose" || rule == "no_matching_rule")
	}
	rules := map[string][]string{
		"self_funding_source_unspecified": {"self_funding_unknown"}, "explicit_capital_contribution": {"owner_capital"}, "owner_overdraft_support": {"owner_financing"}, "owner_funding": {"owner_financing", "owner_repayment"}, "owner_loan_return": {"owner_repayment", "loan_receipt"}, "owner_named_counterparty_loan": {"owner_financing", "owner_repayment"}, "personal_transfer_or_dividend": {"owner_withdrawal"}, "closure_transfer_unverified_account": {"own_transfer_candidate"}, "own_funds_transfer": {"own_transfer", "owner_withdrawal"}, "overdraft_interest": {"overdraft_interest"}, "credit_or_lease": {"loan_receipt", "loan_payment"}, "contract_retention_release": {"retention_receipt"}, "payment_return": {"refund_in", "refund_out"}, "explicit_insurance_compensation": {"insurance_receipt"}, "explicit_grant_or_subsidy": {"grant_receipt"}, "booking_agency_settlement": {"agency_revenue"}, "explicit_enforcement_order": {"enforcement_payment"}, "explicit_tax_text": {"tax"}, "payroll_text": {"salary", "salary_advance", "salary_final"}, "rental_text": {"rent", "equipment_rent"}, "bank_fee_text": {"bank_fee"}, "utility_text": {"utilities"}, "subscription_text": {"subscription"}, "acquiring_text": {"acquiring_revenue"}, "cash_revenue_text": {"cash_revenue"}, "marketplace_text": {"marketplace_revenue"}, "generic_customer_payment": {"customer_payment"}, "generic_operating_expense": {"operating_expense"},
	}
	return source == "rules" && slices.Contains(rules[rule], code)
}
