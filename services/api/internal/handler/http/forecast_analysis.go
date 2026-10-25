package httpapi

import (
	"strconv"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func analysisModel(m *corev1.ForecastModelContext) bool {
	return m != nil && (m.ModelVersion == "mean_90_csv_analysis_v2" || m.ModelVersion == "mean_90_csv_calendar_analysis_v2")
}
func forecastAnalysisDTO(a *corev1.ForecastAnalysis) any {
	if a == nil {
		return nil
	}
	operations := make([]object, 0, len(a.ClassifiedOperations))
	for _, o := range a.ClassifiedOperations {
		operations = append(operations, object{"transaction_id": o.TransactionId, "version": strconv.FormatInt(o.Version, 10), "category_code": o.CategoryCode, "category_source": o.CategorySource, "category_rule_id": o.CategoryRuleId, "category_id": textPointer(o.CategoryId)})
	}
	hypotheses := make([]object, 0, len(a.RecurringHypotheses))
	for _, h := range a.RecurringHypotheses {
		refs := make([]object, 0, len(h.HistoricalOperations))
		for _, r := range h.HistoricalOperations {
			refs = append(refs, object{"transaction_id": r.TransactionId, "version": strconv.FormatInt(r.Version, 10)})
		}
		shift := map[corev1.RecurringWeekendShift]string{corev1.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NONE: "none", corev1.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_PREVIOUS: "previous", corev1.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NEXT: "next"}[h.WeekendShift]
		row := object{"pattern_id": h.PatternId, "rule_id": h.RuleId, "rule_version": h.RuleVersion, "category_code": h.CategoryCode, "direction": directionName(h.Direction), "historical_operations": refs, "weekend_shift": shift, "expected_amount_minor": strconv.FormatInt(h.ExpectedAmountMinor, 10), "confirmations": h.Confirmations, "expected_periods": h.ExpectedPeriods, "first_evidence_date": h.FirstEvidenceDate, "last_evidence_date": h.LastEvidenceDate, "status": "hypothesis"}
		if h.GetMonthEnd() != nil {
			row["month_end"] = true
		} else {
			row["day_of_month"] = h.GetDayOfMonth()
		}
		hypotheses = append(hypotheses, row)
	}
	return object{"analysis_version": a.AnalysisVersion, "window_from": a.WindowFrom, "window_through": a.WindowThrough, "classified_operations": operations, "recurring_hypotheses": hypotheses}
}

func validForecastAnalysis(a *corev1.ForecastAnalysis, i *corev1.ForecastInput) bool {
	if !analysisModel(i.ModelContext) {
		return a == nil
	}
	if a == nil || !validCSVAssessment(i) || a.AnalysisVersion != "text_categories_monthly_v1" || a.WindowFrom != i.HistoryAssessment.FromDate || a.WindowThrough != i.CutoffDate || len(a.ClassifiedOperations) == 0 || len(a.ClassifiedOperations) > 20000 || len(a.RecurringHypotheses) > 1000 {
		return false
	}
	operations := map[string]*corev1.ClassifiedOperationAnalysis{}
	for _, op := range a.ClassifiedOperations {
		if op == nil || !resourceID(op.TransactionId) || op.Version < 1 || operations[op.TransactionId] != nil || !nonempty(op.CategoryCode, 128) || !optionalID(op.CategoryId) || !contains(op.CategorySource, "manual", "rules", "unknown") || (op.CategorySource == "manual" && (op.CategoryId == nil || op.CategoryRuleId != "")) || (op.CategorySource != "manual" && !nonempty(op.CategoryRuleId, 128)) || (op.CategorySource == "unknown" && (op.CategoryCode != "unknown" || op.CategoryId != nil)) {
			return false
		}
		operations[op.TransactionId] = op
	}
	patterns := map[string]bool{}
	refs := map[string]bool{}
	total := 0
	for _, h := range a.RecurringHypotheses {
		if h == nil || !nonempty(h.PatternId, 256) || patterns[h.PatternId] || h.RuleId != "monthly_fit" || h.RuleVersion != a.AnalysisVersion || h.Status != corev1.RecurringHypothesisStatus_RECURRING_HYPOTHESIS_STATUS_HYPOTHESIS || !contains(directionName(h.Direction), "inflow", "outflow") || h.ExpectedAmountMinor <= 0 || h.Confirmations < 3 || h.ExpectedPeriods < h.Confirmations || h.ExpectedPeriods > 5 || h.Confirmations*4 < h.ExpectedPeriods*3 || len(h.HistoricalOperations) < int(h.Confirmations) || !businessDate(h.FirstEvidenceDate) || !businessDate(h.LastEvidenceDate) || h.FirstEvidenceDate < a.WindowFrom || h.LastEvidenceDate > a.WindowThrough || h.FirstEvidenceDate > h.LastEvidenceDate || !nonempty(h.CategoryCode, 128) {
			return false
		}
		patterns[h.PatternId] = true
		switch v := h.MonthlyAnchor.(type) {
		case *corev1.RecurringHypothesis_DayOfMonth:
			if v == nil || v.DayOfMonth < 1 || v.DayOfMonth > 31 {
				return false
			}
		case *corev1.RecurringHypothesis_MonthEnd:
			if v == nil || v.MonthEnd == nil {
				return false
			}
		default:
			return false
		}
		if h.WeekendShift < corev1.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NONE || h.WeekendShift > corev1.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NEXT {
			return false
		}
		for _, ref := range h.HistoricalOperations {
			if ref == nil || refs[ref.TransactionId] {
				return false
			}
			op := operations[ref.TransactionId]
			if op == nil || ref.Version != op.Version || op.CategorySource != "rules" || op.CategoryCode != h.CategoryCode {
				return false
			}
			refs[ref.TransactionId] = true
			total++
		}
		if total > 20000 {
			return false
		}
	}
	return true
}
