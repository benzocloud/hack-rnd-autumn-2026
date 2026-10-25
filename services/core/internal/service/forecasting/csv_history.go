package forecasting

import (
	"fmt"
	"math/big"
	"slices"
	"time"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

const CSVModelVersion = "mean_90_csv_v1"
const CSVPreprocessing = "csv_observed_operations_v1"
const CSVCalendarModelVersion = "mean_90_csv_calendar_v1"
const CSVCalendarPreprocessing = "csv_observed_operations_calendar_v1"
const CSVQualityVersion = "csv_history_quality_v1"

var (
	ErrHistoryTooShort        = fmt.Errorf("%w: CSV history does not span the 90-day model window", ErrHistory)
	ErrHistoryReconciliation  = fmt.Errorf("%w: CSV EOD observations do not reconcile with imported operations", ErrHistory)
	ErrHistorySource          = fmt.Errorf("%w: CSV model requires one imported factual source", ErrHistory)
	ErrHistoryOperationsLimit = fmt.Errorf("%w: CSV model operation limit exceeded", ErrHistory)
)

// HistoryAssessment establishes model suitability, never complete bank coverage.
// Its evidence is frozen with the job; facts and old import receipts stay intact.
type HistoryAssessment struct {
	RuleVersion        string
	Scope              ledger.Scope
	DataVersion        int64
	From, Through      ledger.Date
	FirstOperationDate ledger.Date
	OperationDays      int
	Balances           []HistoryBalance
	Limitations        []string
}

type HistoryBalance struct {
	Date                     ledger.Date
	AmountMinor, DataVersion int64
	Source                   ledger.BalanceSource
}

func assessCSVHistory(snapshot domain.FinancialSnapshot, observations []domain.OpeningBalance) (*HistoryAssessment, error) {
	d := snapshot.Data()
	cutoff, _ := time.Parse(time.DateOnly, string(d.Cutoff))
	from := ledger.Date(cutoff.AddDate(0, 0, -89).Format(time.DateOnly))
	balances := make([]HistoryBalance, 0, len(observations))
	for _, b := range observations {
		if b.Observation.Scope() != d.Scope || b.DataVersion < 1 || b.DataVersion > d.DataVersion || b.Observation.Date() > d.Cutoff {
			return nil, ErrInvalid
		}
		balances = append(balances, HistoryBalance{Date: b.Observation.Date(), AmountMinor: b.Observation.Amount().Minor(), DataVersion: b.DataVersion, Source: b.Observation.Source()})
	}
	return assessCSVBalances(snapshot, from, balances)
}

func assessCSVBalances(snapshot domain.FinancialSnapshot, from ledger.Date, balances []HistoryBalance) (*HistoryAssessment, error) {
	d := snapshot.Data()
	balances = slices.Clone(balances)
	slices.SortFunc(balances, func(a, b HistoryBalance) int {
		if a.Date < b.Date {
			return -1
		}
		if a.Date > b.Date {
			return 1
		}
		return 0
	})
	var first ledger.Date
	var namespace ledger.Namespace
	seenDates := make(map[ledger.Date]bool)
	count := 0
	netByDate := make(map[ledger.Date]*big.Int)
	for _, fact := range d.Facts {
		op := fact.Transaction.Data()
		if first == "" || op.Date < first {
			first = op.Date
		}
		if op.Source.Kind != ledger.Imported && op.Source.Kind != ledger.Manual {
			return nil, ErrHistorySource
		}
		if op.Source.Kind == ledger.Imported && namespace == (ledger.Namespace{}) {
			namespace = op.Source.Namespace
		}
		if op.Source.Kind == ledger.Imported && namespace != op.Source.Namespace {
			return nil, ErrHistorySource
		}
		if op.Date >= from {
			count++
			seenDates[op.Date] = true
		}
		if netByDate[op.Date] == nil {
			netByDate[op.Date] = new(big.Int)
		}
		if op.Direction == ledger.Inflow {
			netByDate[op.Date].Add(netByDate[op.Date], big.NewInt(op.Amount.Minor()))
		} else {
			netByDate[op.Date].Sub(netByDate[op.Date], big.NewInt(op.Amount.Minor()))
		}
	}
	if first == "" || first > from || count == 0 {
		return nil, ErrHistoryTooShort
	}
	if namespace == (ledger.Namespace{}) {
		return nil, ErrHistorySource
	}
	if count > 20000 {
		return nil, ErrHistoryOperationsLimit
	}
	dates := make([]ledger.Date, 0, len(netByDate))
	for date := range netByDate {
		dates = append(dates, date)
	}
	slices.Sort(dates)
	position := 0
	cumulative, previousNet := new(big.Int), new(big.Int)
	for n, b := range balances {
		if b.Date.Validate() != nil || b.Date > d.Cutoff || b.DataVersion < 1 || b.DataVersion > d.DataVersion {
			return nil, ErrInvalid
		}
		amount, err := money.New(b.AmountMinor, money.RUB)
		if err != nil {
			return nil, ErrInvalid
		}
		if _, err := ledger.NewBalanceObservation(d.Scope, b.Date, amount, b.Source); err != nil {
			return nil, ErrInvalid
		}
		if n > 0 && balances[n-1].Date == b.Date {
			return nil, ErrHistoryReconciliation
		}
		if b.Source.Kind == ledger.StatementEOD && b.Source.Namespace != namespace {
			return nil, ErrHistorySource
		}
		for position < len(dates) && dates[position] <= b.Date {
			cumulative.Add(cumulative, netByDate[dates[position]])
			position++
		}
		if n > 0 && b.Date >= from {
			expected := new(big.Int).Sub(cumulative, previousNet)
			expected.Add(expected, big.NewInt(balances[n-1].AmountMinor))
			if expected.Cmp(big.NewInt(b.AmountMinor)) != 0 {
				return nil, ErrHistoryReconciliation
			}
		}
		previousNet.Set(cumulative)
	}
	if len(balances) == 0 || balances[len(balances)-1].Date != d.Cutoff || balances[len(balances)-1].AmountMinor != d.Opening.Observation.Amount().Minor() || balances[len(balances)-1].DataVersion != d.Opening.DataVersion || balances[len(balances)-1].Source != d.Opening.Observation.Source() {
		return nil, ErrMissingBalance
	}
	// Preserve one preceding EOD for reconciliation, then only the model window.
	start := 0
	for start+1 < len(balances) && balances[start+1].Date < from {
		start++
	}
	return &HistoryAssessment{RuleVersion: CSVQualityVersion, Scope: d.Scope, DataVersion: d.DataVersion, From: from, Through: d.Cutoff, FirstOperationDate: first, OperationDays: len(seenDates), Balances: slices.Clone(balances[start:]), Limitations: csvAssessmentLimitations(snapshot)}, nil
}

func csvLimitations() []string {
	return []string{"csv_history_completeness_unverified", "unobserved_days_unknown", "observed_operations_mean_per_calendar_day", "historical_calendar_unavailable", "historical_availability_unverified", "not_calibrated"}
}

func validCSVAssessment(a *HistoryAssessment, snapshot domain.FinancialSnapshot) bool {
	if a == nil || a.RuleVersion != CSVQualityVersion || a.Scope != snapshot.Data().Scope || a.DataVersion != snapshot.Data().DataVersion || a.Through != snapshot.Data().Cutoff || !slices.Equal(a.Limitations, csvAssessmentLimitations(snapshot)) {
		return false
	}
	cutoff, _ := time.Parse(time.DateOnly, string(a.Through))
	if a.From != ledger.Date(cutoff.AddDate(0, 0, -89).Format(time.DateOnly)) {
		return false
	}
	checked, err := assessCSVBalances(snapshot, a.From, a.Balances)
	return err == nil && checked.FirstOperationDate == a.FirstOperationDate && checked.OperationDays == a.OperationDays && slices.Equal(checked.Balances, a.Balances)
}

func csvCalendarLimitations() []string {
	return []string{"csv_history_completeness_unverified", "unobserved_days_unknown", "observed_operations_mean_per_calendar_day", "historical_availability_unverified", "not_calibrated", "current_calendar_only"}
}
func csvAssessmentLimitations(snapshot domain.FinancialSnapshot) []string {
	if activeCalendar(snapshot.Data().Calendar) {
		return csvCalendarLimitations()
	}
	return csvLimitations()
}
func csvProfile(version string) bool {
	return version == CSVModelVersion || version == CSVCalendarModelVersion
}
