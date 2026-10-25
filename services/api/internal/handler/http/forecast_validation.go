package httpapi

import (
	"math"
	"time"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func validForecast(f *corev1.Forecast, a *corev1.Account, id string) bool {
	if f == nil || f.Id != id || !resourceID(f.Id) || f.CompanyId != a.CompanyId || !timestamp(f.CreatedAt) || !optionalID(f.ScenarioId) || !validForecastInput(f.Input, a.Id) || f.CurrentDataVersion < f.Input.DataVersion || f.IsStale != (f.CurrentDataVersion > f.Input.DataVersion) || !validReadiness(f.Readiness) {
		return false
	}
	switch state := f.State.(type) {
	case *corev1.Forecast_Pending:
		return state != nil && state.Pending != nil && (state.Pending.Status == corev1.ForecastStatus_FORECAST_STATUS_QUEUED || state.Pending.Status == corev1.ForecastStatus_FORECAST_STATUS_RUNNING)
	case *corev1.Forecast_Succeeded:
		return (provenCoverage(f.Input.GetHistoryCoverage()) || (validCSVAssessment(f.Input) && f.Readiness.Status == "limited" && csvReadiness(f))) && f.Readiness.Status != "insufficient" && state != nil && state.Succeeded != nil && timestamp(state.Succeeded.CompletedAt) && !state.Succeeded.CompletedAt.AsTime().Before(f.CreatedAt.AsTime()) && validForecastResult(state.Succeeded.Result, f.Input)
	case *corev1.Forecast_Failed:
		if state == nil || state.Failed == nil || !timestamp(state.Failed.CompletedAt) || state.Failed.CompletedAt.AsTime().Before(f.CreatedAt.AsTime()) {
			return false
		}
		e := state.Failed.Error
		return e != nil && machineCode.MatchString(e.Code) && nonempty(e.RequestId, 128)
	default:
		return false
	}
}
func validForecastInput(i *corev1.ForecastInput, account string) bool {
	if i == nil || !resourceID(i.SnapshotId) || i.AccountId != account || i.DataVersion < 1 || i.Currency != "RUB" || !validMoney(i.OpeningBalance) || !dateSequence(i.CutoffDate, i.StartDate, i.EndDate, i.HorizonDays) || i.BalanceAsOf != i.CutoffDate || !businessDate(i.DataAsOf) || i.DataAsOf > i.CutoffDate || i.RiskDefinition != "below_zero_eod" || !validHistoryCoverage(i.HistoryCoverage, i.DataVersion, i.CutoffDate) {
		return false
	}
	if !validCalendarKnowledge(i) {
		return false
	}
	if i.HistoryAssessment != nil && !validCSVAssessment(i) {
		return false
	}
	if i.ModelContext != nil && i.ModelContext.ModelVersion == "mean_90_csv_v1" && !validCSVAssessment(i) {
		return false
	}
	m := i.ModelContext
	return m != nil && resourceID(m.Id) && nonempty(m.ModelName, 128) && nonempty(m.ModelVersion, 128) && nonempty(m.PreprocessingVersion, 128) && nonempty(m.ConfigurationId, 128) && (m.RandomSeed == nil || unsignedDecimal(*m.RandomSeed)) && (m.TrajectoryCount == nil || *m.TrajectoryCount > 0)
}
func provenCoverage(c *corev1.HistoryCoverage) bool {
	if c == nil {
		return false
	}
	return c.Kind == corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_CERTIFIED || c.Kind == corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_DATASET_FIXTURE_ASSUMPTION
}

func validHistoryCoverage(c *corev1.HistoryCoverage, revision int64, cutoff string) bool {
	if c == nil || c.Kind == corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_UNSPECIFIED {
		return true
	}
	if !businessDate(c.FromDate) || !businessDate(c.ThroughDate) || c.FromDate > c.ThroughDate || c.ThroughDate < cutoff || c.DataVersion != revision || !nonempty(c.Reference, 256) || len(c.Limitations) > 32 {
		return false
	}
	switch c.Kind {
	case corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_UNSPECIFIED:
		return false
	case corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_CERTIFIED:
	case corev1.HistoryCoverageKind_HISTORY_COVERAGE_KIND_DATASET_FIXTURE_ASSUMPTION:
		if !contains("dataset_coverage_assumed_not_bank_certified", c.Limitations...) {
			return false
		}
	default:
		return false
	}
	seen := make(map[string]bool, len(c.Limitations))
	for _, limitation := range c.Limitations {
		if !nonempty(limitation, 128) || seen[limitation] {
			return false
		}
		seen[limitation] = true
	}
	return true
}
func unsignedDecimal(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func validReadiness(r *corev1.ForecastReadiness) bool {
	if r == nil || !contains(r.Status, "ready", "limited", "insufficient") || r.ObservedDays > r.HistoryDays {
		return false
	}
	for _, i := range r.QualityIssues {
		if i == nil || !contains(i.Code, "MISSING_COUNTERPARTY_INN", "EMPTY_PAYMENT_PURPOSE", "SUSPECTED_CONTENT_DUPLICATE", "BALANCE_DISCONTINUITY", "SHORT_HISTORY", "DUPLICATE_EXTERNAL_ID", "CONFLICTING_EXTERNAL_ID", "INVALID_ROW", "UNSUPPORTED_CURRENCY", "MULTIPLE_SOURCE_COMPANIES", "FACTUAL_SOURCE_CONFLICT", "SOURCE_BINDING_CONFLICT") || !contains(issueSeverity(i.Severity), "warning", "blocking") || (i.RowNumber != nil && *i.RowNumber < 2) {
			return false
		}
	}
	return true
}
func validProbability(p *corev1.Probability) bool {
	if p == nil {
		return false
	}
	switch v := p.Estimate.(type) {
	case *corev1.Probability_Value:
		return v != nil && !math.IsNaN(v.Value) && !math.IsInf(v.Value, 0) && v.Value >= 0 && v.Value <= 1
	case *corev1.Probability_UnavailableReason:
		return v != nil && contains(v.UnavailableReason, "not_estimated", "not_calibrated", "insufficient_history", "incomplete_coverage")
	default:
		return false
	}
}
func validSource(s *corev1.RecordSource) bool {
	if s == nil {
		return true
	}
	if !nonempty(s.SourceSystem, 128) || !optionalID(s.ImportId) || !optionalID(s.ConnectionId) || !optionalOpaque(s.SourceCompanyId) || !optionalOpaque(s.ExternalOperationId) || !optionalOpaque(s.ExternalPaymentId) {
		return false
	}
	return s.Kind == corev1.RecordSourceKind_RECORD_SOURCE_KIND_FILE || s.Kind == corev1.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL || s.Kind == corev1.RecordSourceKind_RECORD_SOURCE_KIND_MOCK_EXTERNAL
}
func validForecastResult(r *corev1.ForecastResult, input *corev1.ForecastInput) bool {
	if r == nil || r.Metrics == nil || r.ScenarioCapabilities == nil || len(r.DailyPoints) != int(input.HorizonDays) || len(r.Components) > 10000 || len(r.EffectiveOccurrences) > 10000 {
		return false
	}
	m := r.Metrics
	if !nonnegativeMoney(m.MaxShortfall) || !validMoney(m.MinimumBalance) || !validMoney(m.ClosingBalance) || !validProbability(m.GapProbability) {
		return false
	}
	if g := m.FirstGap; g != nil && (!businessDate(g.Date) || g.Date < input.StartDate || g.Date > input.EndDate || !positiveMoney(g.Shortfall)) {
		return false
	}
	start, _ := time.Parse(time.DateOnly, input.StartDate)
	for index, p := range r.DailyPoints {
		if p == nil || p.Date != start.AddDate(0, 0, index).Format(time.DateOnly) || !nonnegativeMoney(p.Inflow) || !nonnegativeMoney(p.Outflow) || !validMoney(p.Balance) || !validProbability(p.NegativeBalanceProbability) {
			return false
		}
		if (p.BalanceLower == nil) != (p.BalanceUpper == nil) {
			return false
		}
		if p.BalanceLower != nil && (!validMoney(p.BalanceLower) || !validMoney(p.BalanceUpper) || p.BalanceLower.AmountMinor > p.BalanceUpper.AmountMinor) {
			return false
		}
	}
	if !validForecastMetrics(m, r.DailyPoints) {
		return false
	}
	for _, c := range r.Components {
		if c == nil || !nonempty(c.Id, 128) || !nonempty(c.OccurrenceId, 128) || !contains(c.Kind, "known_payment", "ml_sales_receipt", "recurring_expense", "scenario_expense") || !contains(directionName(c.Direction), "inflow", "outflow") || !businessDate(c.Date) || !positiveMoney(c.Amount) || !nonnegativeMoney(c.RemainingAmount) || c.RemainingAmount.AmountMinor > c.Amount.AmountMinor || !optionalID(c.CategoryId) || !optionalID(c.PaymentId) || !optionalOpaque(c.RecurrenceGroupId) || !validSource(c.Source) {
			return false
		}
	}
	for _, o := range r.EffectiveOccurrences {
		if o == nil || !nonempty(o.EffectiveOccurrenceId, 128) || !nonempty(o.ComponentId, 128) || !nonempty(o.OccurrenceId, 128) || !optionalID(o.PaymentId) || !contains(directionName(o.Direction), "inflow", "outflow") || !businessDate(o.Date) || !nonnegativeMoney(o.RemainingAmount) {
			return false
		}
	}
	for _, rec := range r.Recommendations {
		if rec == nil || !nonempty(rec.Id, 128) || (rec.SuggestedAction != nil && !contains(*rec.SuggestedAction, "reschedule_payment", "request_early_receipt", "reduce_discretionary_expense")) {
			return false
		}
		for _, e := range rec.Evidence {
			if e == nil || !contains(e.Type, "payment", "transaction", "forecast_component", "quality_issue") || !nonempty(e.ResourceId, 128) || !validSource(e.Source) {
				return false
			}
		}
	}
	return true
}

// Points are structurally validated before comparing their horizon aggregates.
func validForecastMetrics(m *corev1.ForecastMetrics, points []*corev1.ForecastPoint) bool {
	if len(points) == 0 {
		return false
	}
	minimum := points[0].Balance.AmountMinor
	var first *corev1.ForecastPoint
	for _, point := range points {
		minimum = min(minimum, point.Balance.AmountMinor)
		if first == nil && point.Balance.AmountMinor < 0 {
			first = point
		}
	}
	// -MinInt64 cannot be represented by the positive int64 money contract.
	if minimum == math.MinInt64 || m.MinimumBalance.AmountMinor != minimum || m.ClosingBalance.AmountMinor != points[len(points)-1].Balance.AmountMinor || m.MaxShortfall.AmountMinor != max(int64(0), -minimum) {
		return false
	}
	if first == nil {
		return m.FirstGap == nil
	}
	return m.FirstGap != nil && m.FirstGap.Date == first.Date && m.FirstGap.Shortfall.AmountMinor == -first.Balance.AmountMinor
}

func validCSVAssessment(i *corev1.ForecastInput) bool {
	a, m := i.HistoryAssessment, i.ModelContext
	if a == nil || m == nil || i.HistoryCoverage != nil || a.RuleVersion != "csv_history_quality_v1" || a.DataVersion != i.DataVersion || a.ThroughDate != i.CutoffDate || a.OperationDays < 1 || a.OperationDays > 90 || a.BalanceDays < 1 || a.BalanceDays > 90 || m.ModelName != "mean_90" || m.ModelVersion != "mean_90_csv_v1" || m.PreprocessingVersion != "csv_observed_operations_v1" || m.ConfigurationId != "mean_90_csv_v1" || m.RandomSeed != nil || m.TrajectoryCount != nil {
		return false
	}
	cutoff, err := time.Parse(time.DateOnly, i.CutoffDate)
	if err != nil || a.FromDate != cutoff.AddDate(0, 0, -89).Format(time.DateOnly) {
		return false
	}
	expected := []string{"csv_history_completeness_unverified", "unobserved_days_unknown", "observed_operations_mean_per_calendar_day", "historical_calendar_unavailable", "historical_availability_unverified", "not_calibrated"}
	if len(a.Limitations) != len(expected) {
		return false
	}
	for n, v := range expected {
		if a.Limitations[n] != v {
			return false
		}
	}
	return true
}

func csvReadiness(f *corev1.Forecast) bool {
	if f.Readiness.HistoryDays != 90 || f.Readiness.ObservedDays != f.Input.HistoryAssessment.OperationDays {
		return false
	}
	for _, limitation := range f.Input.HistoryAssessment.Limitations {
		if !contains(limitation, f.Readiness.Limitations...) {
			return false
		}
	}
	return true
}

func validCalendarKnowledge(i *corev1.ForecastInput) bool {
	switch i.CalendarKnowledgeMode {
	case corev1.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_UNSPECIFIED, corev1.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE:
		return i.CalendarAsOfDate == "" && !i.NoPostCutoffMovements
	case corev1.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_CURRENT:
		return (i.CalendarAsOfDate == i.CutoffDate || i.CalendarAsOfDate == i.StartDate && i.NoPostCutoffMovements) && i.ModelContext != nil && i.ModelContext.ModelVersion == "mean_90_components_v1"
	case corev1.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_HISTORICAL_AS_OF_CERTIFIED, corev1.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_SYNTHETIC_SCENARIO:
		return false
	default:
		return false
	}
}
