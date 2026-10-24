package httpapi

import (
	"strconv"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func forecastDTO(f *corev1.Forecast) object {
	result := object{"id": f.Id, "company_id": f.CompanyId, "created_at": utc(f.CreatedAt), "completed_at": nil, "is_stale": f.IsStale, "current_data_version": strconv.FormatInt(f.CurrentDataVersion, 10), "input": inputDTO(f.Input), "readiness": readinessDTO(f.Readiness), "scenario_id": textPointer(f.ScenarioId), "result": nil, "error": nil}
	switch state := f.State.(type) {
	case *corev1.Forecast_Pending:
		result["status"] = statusName(state.Pending.Status)
	case *corev1.Forecast_Succeeded:
		result["status"], result["completed_at"], result["result"] = "succeeded", utc(state.Succeeded.CompletedAt), forecastResultDTO(state.Succeeded.Result)
	case *corev1.Forecast_Failed:
		e := state.Failed.Error
		fields := make([]fieldError, 0, len(e.FieldErrors))
		for _, f := range e.FieldErrors {
			if f != nil && contains(f.Field, "cutoff_date", "horizon_days", "account_id") {
				fields = append(fields, fieldError{f.Field, e.Code, fieldMessage(f.Field)})
			}
		}
		result["status"], result["completed_at"], result["error"] = "failed", utc(state.Failed.CompletedAt), errorDetail{e.Code, errorMessage(e.Code), fields, e.RequestId}
	}
	return result
}
func inputDTO(i *corev1.ForecastInput) object {
	m := i.ModelContext
	return object{"snapshot_id": i.SnapshotId, "data_version": strconv.FormatInt(i.DataVersion, 10), "cutoff_date": i.CutoffDate, "data_as_of": i.DataAsOf, "start_date": i.StartDate, "end_date": i.EndDate, "horizon_days": i.HorizonDays, "currency": i.Currency, "account_id": i.AccountId, "opening_balance": moneyDTO(i.OpeningBalance), "balance_as_of": i.BalanceAsOf, "risk_definition": i.RiskDefinition,
		"model_context": object{"id": m.Id, "model_name": m.ModelName, "model_version": m.ModelVersion, "preprocessing_version": m.PreprocessingVersion, "configuration_id": m.ConfigurationId, "random_seed": textPointer(m.RandomSeed), "trajectory_count": m.TrajectoryCount}}
}
func readinessDTO(r *corev1.ForecastReadiness) object {
	issues := make([]object, 0, len(r.QualityIssues))
	for _, i := range r.QualityIssues {
		issues = append(issues, object{"code": i.Code, "severity": issueSeverity(i.Severity), "message": i.Message, "row_number": i.RowNumber, "field": textPointer(i.Field), "related_external_operation_ids": stringsDTO(i.RelatedExternalOperationIds)})
	}
	return object{"status": r.Status, "history_days": r.HistoryDays, "observed_days": r.ObservedDays, "quality_issues": issues, "limitations": stringsDTO(r.Limitations)}
}
func probabilityDTO(p *corev1.Probability) object {
	if v, ok := p.Estimate.(*corev1.Probability_Value); ok {
		return object{"value": v.Value, "unavailable_reason": nil}
	}
	return object{"value": nil, "unavailable_reason": p.GetUnavailableReason()}
}
func sourceDTO(s *corev1.RecordSource) any {
	if s == nil {
		return nil
	}
	kind := "manual"
	switch s.Kind {
	case corev1.RecordSourceKind_RECORD_SOURCE_KIND_UNSPECIFIED:
		return nil
	case corev1.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL:
		kind = "manual"
	case corev1.RecordSourceKind_RECORD_SOURCE_KIND_FILE:
		kind = "file"
	case corev1.RecordSourceKind_RECORD_SOURCE_KIND_MOCK_EXTERNAL:
		kind = "mock_external"
	}
	return object{"kind": kind, "source_system": s.SourceSystem, "source_company_id": textPointer(s.SourceCompanyId), "external_operation_id": textPointer(s.ExternalOperationId), "external_payment_id": textPointer(s.ExternalPaymentId), "import_id": textPointer(s.ImportId), "connection_id": textPointer(s.ConnectionId)}
}
func forecastResultDTO(r *corev1.ForecastResult) object {
	m := r.Metrics
	var gap any
	if m.FirstGap != nil {
		gap = object{"date": m.FirstGap.Date, "shortfall": moneyDTO(m.FirstGap.Shortfall)}
	}
	metrics := object{"first_gap": gap, "max_shortfall": moneyDTO(m.MaxShortfall), "minimum_balance": moneyDTO(m.MinimumBalance), "closing_balance": moneyDTO(m.ClosingBalance), "gap_probability": probabilityDTO(m.GapProbability)}
	points := make([]object, 0, len(r.DailyPoints))
	for _, p := range r.DailyPoints {
		points = append(points, object{"date": p.Date, "inflow": moneyDTO(p.Inflow), "outflow": moneyDTO(p.Outflow), "balance": moneyDTO(p.Balance), "balance_lower": moneyDTO(p.BalanceLower), "balance_upper": moneyDTO(p.BalanceUpper), "negative_balance_probability": probabilityDTO(p.NegativeBalanceProbability)})
	}
	components := make([]object, 0, len(r.Components))
	for _, c := range r.Components {
		components = append(components, object{"id": c.Id, "occurrence_id": c.OccurrenceId, "kind": c.Kind, "direction": directionName(c.Direction), "date": c.Date, "amount": moneyDTO(c.Amount), "remaining_amount": moneyDTO(c.RemainingAmount), "category_id": textPointer(c.CategoryId), "payment_id": textPointer(c.PaymentId), "recurrence_group_id": textPointer(c.RecurrenceGroupId), "source": sourceDTO(c.Source), "scenario_eligible": c.ScenarioEligible})
	}
	effective := make([]object, 0, len(r.EffectiveOccurrences))
	for _, o := range r.EffectiveOccurrences {
		effective = append(effective, object{"effective_occurrence_id": o.EffectiveOccurrenceId, "component_id": o.ComponentId, "occurrence_id": o.OccurrenceId, "payment_id": textPointer(o.PaymentId), "direction": directionName(o.Direction), "date": o.Date, "remaining_amount": moneyDTO(o.RemainingAmount), "in_horizon": o.InHorizon, "response_action_eligible": o.ResponseActionEligible})
	}
	recommendations := make([]object, 0, len(r.Recommendations))
	for _, rec := range r.Recommendations {
		evidence := make([]object, 0, len(rec.Evidence))
		for _, e := range rec.Evidence {
			evidence = append(evidence, object{"type": e.Type, "resource_id": e.ResourceId, "source": sourceDTO(e.Source), "description": e.Description})
		}
		recommendations = append(recommendations, object{"id": rec.Id, "title": rec.Title, "explanation": rec.Explanation, "feasibility_condition": rec.FeasibilityCondition, "evidence": evidence, "suggested_action": textPointer(rec.SuggestedAction)})
	}
	c := r.ScenarioCapabilities
	return object{"metrics": metrics, "daily_points": points, "components": components, "scenario_capabilities": object{"delay_receipt": c.DelayReceipt, "reduce_sales": c.ReduceSales, "one_off_expense": c.OneOffExpense, "increase_recurring_expenses": c.IncreaseRecurringExpenses, "unavailable_reason": textPointer(c.UnavailableReason)}, "recommendations": recommendations, "assumptions": stringsDTO(r.Assumptions), "effective_occurrences": effective}
}
