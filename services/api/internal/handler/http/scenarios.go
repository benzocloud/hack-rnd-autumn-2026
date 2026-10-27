package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func (h *handler) scenarioRoute(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/v1/companies/") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/companies/"), "/")
	method := http.MethodGet
	var action http.HandlerFunc
	switch {
	case len(parts) == 2 && parts[1] == "scenarios":
		action = h.listScenarios
		if r.Method == http.MethodPost {
			method = http.MethodPost
			action = h.createScenario
		}
	case len(parts) == 3 && parts[1] == "scenarios":
		action = h.getScenario
	case len(parts) == 4 && parts[1] == "forecasts" && parts[3] == "comparison":
		action = h.compareForecasts
	default:
		return false
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeError(w, r, 405, "INVALID_REQUEST", nil)
		return true
	}
	if !resourceID(parts[0]) || len(parts) > 2 && !resourceID(parts[2]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return true
	}
	if method == http.MethodPost && (!h.allowedOrigin(r) || len(r.Header.Values("X-CSRF-Token")) != 1 || len(r.Header.Get("X-CSRF-Token")) < 16 || len(r.Header.Get("X-CSRF-Token")) > 512) {
		writeError(w, r, 403, "CSRF_INVALID", nil)
		return true
	}
	if h.core.Scenarios == nil || h.core.Ledger == nil || len(parts) == 4 && h.core.Comparisons == nil {
		dependencyError(w, r)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	r.SetPathValue("company_id", parts[0])
	if len(parts) > 2 {
		r.SetPathValue("resource_id", parts[2])
	}
	action(w, r)
	return true
}
func (h *handler) createScenario(w http.ResponseWriter, r *http.Request) {
	fields, err := paymentFields(w, r, 64<<10, []string{"name", "base_forecast_id", "changes", "comparison_parent_scenario_id", "response_action"}, "name")
	if err != nil || r.URL.RawQuery != "" {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	name, ok := paymentText(fields, "name", 200)
	if !ok || name == nil || strings.TrimSpace(*name) == "" {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	_, hasParent := fields["comparison_parent_scenario_id"]
	_, hasResponse := fields["response_action"]
	if hasParent || hasResponse {
		if !hasParent || !hasResponse || fields["base_forecast_id"] != nil || fields["changes"] != nil {
			writeError(w, r, 422, "VALIDATION_ERROR", nil)
			return
		}
		var parent string
		action, valid := responseActionBody(fields["response_action"])
		if json.Unmarshal(fields["comparison_parent_scenario_id"], &parent) != nil || !resourceID(parent) || !valid {
			writeError(w, r, 422, "VALIDATION_ERROR", nil)
			return
		}
		h.submitScenario(w, r, &corev1.CreateScenarioRequest{Name: *name, Scenario: &corev1.CreateScenarioRequest_Response{Response: &corev1.ResponseScenario{ComparisonParentScenarioId: parent, ResponseAction: action}}})
		return
	}

	var base string
	if json.Unmarshal(fields["base_forecast_id"], &base) != nil || !resourceID(base) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	var raw []map[string]json.RawMessage
	if json.Unmarshal(fields["changes"], &raw) != nil || len(raw) < 1 || len(raw) > 20 {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	changes := []*corev1.ScenarioChange{}
	for _, f := range raw {
		var discriminator string
		if json.Unmarshal(f["type"], &discriminator) == nil {
			var change *corev1.ScenarioChange
			var valid bool
			switch discriminator {
			case "reduce_sales":
				change, valid = salesChangeBody(f)
			case "delay_receipt":
				change, valid = delayChangeBody(f)
			}
			if discriminator == "reduce_sales" || discriminator == "delay_receipt" {
				if !valid {
					writeError(w, r, 422, "VALIDATION_ERROR", nil)
					return
				}
				changes = append(changes, change)
				continue
			}
		}

		for key := range f {
			if !contains(key, "id", "type", "date", "amount", "description", "category_id") {
				writeError(w, r, 422, "VALIDATION_ERROR", nil)
				return
			}
		}
		var id, kind, date, description string
		if json.Unmarshal(f["type"], &kind) != nil || kind != "one_off_expense" {
			writeError(w, r, 422, "SCENARIO_NOT_SUPPORTED", nil)
			return
		}
		amount, valid := paymentMoney(f["amount"])
		if !valid || json.Unmarshal(f["id"], &id) != nil || !resourceID(id) || json.Unmarshal(f["date"], &date) != nil || !businessDate(date) || json.Unmarshal(f["description"], &description) != nil || !nonempty(description, 1000) {
			writeError(w, r, 422, "VALIDATION_ERROR", nil)
			return
		}
		category, valid := paymentOptionalID(f, "category_id")
		if !valid || category != nil {
			writeError(w, r, 422, "VALIDATION_ERROR", nil)
			return
		}
		changes = append(changes, &corev1.ScenarioChange{Change: &corev1.ScenarioChange_OneOffExpense{OneOffExpense: &corev1.OneOffExpenseChange{Id: id, Date: date, Amount: amount, Description: description}}})
	}
	h.submitScenario(w, r, &corev1.CreateScenarioRequest{Name: *name, Scenario: &corev1.CreateScenarioRequest_Independent{Independent: &corev1.IndependentScenario{BaseForecastId: base, Changes: changes}}})
}
func (h *handler) submitScenario(w http.ResponseWriter, r *http.Request, request *corev1.CreateScenarioRequest) {
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !validIdempotencyKey(keys[0]) {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	request.Context = writeContext(r)
	request.Scope = scopeFor(account)
	request.IdempotencyKey = keys[0]
	response, err := h.core.Scenarios.CreateScenario(r.Context(), request)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	v := response.GetAccepted()
	if v == nil || !resourceID(v.Id) || !resourceID(v.ForecastId) || v.Status != corev1.ForecastStatus_FORECAST_STATUS_QUEUED || v.PollAfterMs < 1000 {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 202, object{"id": v.Id, "forecast_id": v.ForecastId, "status": "queued", "poll_after_ms": v.PollAfterMs, "status_url": "/api/v1/companies/" + account.CompanyId + "/forecasts/" + v.ForecastId})
}
func scenarioDTO(s *corev1.Scenario) (object, bool) {
	if s == nil || !resourceID(s.Id) || !resourceID(s.BaseForecastId) || !resourceID(s.ForecastId) || !nonempty(s.Name, 200) || !timestamp(s.CreatedAt) || !optionalID(s.ComparisonParentScenarioId) || (s.ComparisonParentScenarioId == nil) != (s.ResponseAction == nil) || len(s.Changes) < 1 || len(s.Changes) > 20 {
		return nil, false
	}
	var response any
	if action := s.ResponseAction; action != nil {
		if !nonempty(action.EffectiveOccurrenceId, 128) || !positiveMoney(action.Amount) || !businessDate(action.NewDate) || !nonempty(action.FeasibilityCondition, 1000) {
			return nil, false
		}
		response = object{"type": "reschedule_payment_part", "target": object{"effective_occurrence_id": action.EffectiveOccurrenceId}, "amount": moneyDTO(action.Amount), "new_date": action.NewDate, "feasibility_condition": action.FeasibilityCondition}
	}
	for _, id := range s.OutOfHorizonOccurrenceIds {
		if !nonempty(id, 128) {
			return nil, false
		}
	}

	changes := []object{}
	for _, c := range s.Changes {
		if delay := c.GetDelayReceipt(); delay != nil {
			if !resourceID(delay.Id) || delay.Target == nil || !nonempty(delay.Target.ComponentId, 128) || !nonempty(delay.Target.OccurrenceId, 128) || !positiveMoney(delay.Amount) || !businessDate(delay.NewDate) {
				return nil, false
			}
			changes = append(changes, object{"id": delay.Id, "type": "delay_receipt", "target": object{"component_id": delay.Target.ComponentId, "occurrence_id": delay.Target.OccurrenceId}, "amount": moneyDTO(delay.Amount), "new_date": delay.NewDate})
			continue
		}

		if reduction := c.GetReduceSales(); reduction != nil {
			if !resourceID(reduction.Id) || !businessDate(reduction.PeriodStart) || !businessDate(reduction.PeriodEnd) || reduction.PeriodStart > reduction.PeriodEnd || reduction.ReductionBasisPoints < 1 || reduction.ReductionBasisPoints > 10000 || len(reduction.Targets) < 1 || len(reduction.Targets) > 1000 {
				return nil, false
			}
			targets := []object{}
			for _, t := range reduction.Targets {
				if t == nil || !nonempty(t.ComponentId, 128) || t.ComponentId != t.OccurrenceId {
					return nil, false
				}
				targets = append(targets, object{"component_id": t.ComponentId, "occurrence_id": t.OccurrenceId})
			}
			changes = append(changes, object{"id": reduction.Id, "type": "reduce_sales", "targets": targets, "period_start": reduction.PeriodStart, "period_end": reduction.PeriodEnd, "reduction_basis_points": reduction.ReductionBasisPoints})
			continue
		}

		v := c.GetOneOffExpense()
		if v == nil || !resourceID(v.Id) || !businessDate(v.Date) || !positiveMoney(v.Amount) || !nonempty(v.Description, 1000) || v.CategoryId != nil {
			return nil, false
		}
		changes = append(changes, object{"id": v.Id, "type": "one_off_expense", "date": v.Date, "amount": moneyDTO(v.Amount), "description": v.Description, "category_id": nil})
	}
	return object{"id": s.Id, "name": s.Name, "base_forecast_id": s.BaseForecastId, "forecast_id": s.ForecastId, "comparison_parent_scenario_id": textPointer(s.ComparisonParentScenarioId), "changes": changes, "response_action": response, "out_of_horizon_occurrence_ids": stringsDTO(s.OutOfHorizonOccurrenceIds), "created_at": utc(s.CreatedAt)}, true
}
func (h *handler) getScenario(w http.ResponseWriter, r *http.Request) {
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Scenarios.GetScenario(r.Context(), &corev1.GetScenarioRequest{Context: browser(r), Scope: scopeFor(account), ScenarioId: r.PathValue("resource_id")})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	value, ok := scenarioDTO(response.GetScenario())
	if !ok || response.GetScenario().Id != r.PathValue("resource_id") {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, value)
}
func (h *handler) listScenarios(w http.ResponseWriter, r *http.Request) {
	q, err := parseQuery(r)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	var base *string
	if values, ok := q["base_forecast_id"]; ok {
		if len(values) != 1 || !resourceID(values[0]) {
			writeError(w, r, 400, "INVALID_REQUEST", nil)
			return
		}
		base = &values[0]
		q.Del("base_forecast_id")
	}
	copyRequest := r.Clone(r.Context())
	copyURL := *r.URL
	copyURL.RawQuery = q.Encode()
	copyRequest.URL = &copyURL
	page, _, err := readPage(copyRequest, false)
	if err != nil {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Scenarios.ListScenarios(r.Context(), &corev1.ListScenariosRequest{Context: browser(r), Scope: scopeFor(account), Page: page, BaseForecastId: base})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	if response == nil || response.Pagination == nil || !validCursor(response.Pagination.NextCursor) || len(response.Items) > 100 {
		dependencyError(w, r)
		return
	}
	items := []object{}
	for _, v := range response.Items {
		item, ok := scenarioDTO(v)
		if !ok || base != nil && v.BaseForecastId != *base {
			dependencyError(w, r)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, 200, object{"items": items, "next_cursor": optionalText(response.Pagination.NextCursor)})
}
func (h *handler) compareForecasts(w http.ResponseWriter, r *http.Request) {
	q, err := parseQuery(r)
	if err != nil || len(q) != 1 || len(q["reference_forecast_id"]) != 1 || !resourceID(q.Get("reference_forecast_id")) {
		writeError(w, r, 400, "INVALID_REQUEST", nil)
		return
	}
	account, err := h.accountScope(r)
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	response, err := h.core.Comparisons.CompareForecasts(r.Context(), &corev1.CompareForecastsRequest{Context: browser(r), Scope: scopeFor(account), CandidateForecastId: r.PathValue("resource_id"), ReferenceForecastId: q.Get("reference_forecast_id")})
	if err != nil {
		coreRPCError(w, r, err)
		return
	}
	c := response.GetComparison()
	if c == nil || c.CandidateForecastId != r.PathValue("resource_id") || c.ReferenceForecastId != q.Get("reference_forecast_id") || !resourceID(c.SnapshotId) || !resourceID(c.ModelContextId) || !validComparisonMetrics(c.CandidateMetrics) || !validComparisonMetrics(c.ReferenceMetrics) || !validMoney(c.DeltaMaxShortfall) || !validMoney(c.DeltaMinimumBalance) || !validMoney(c.DeltaClosingBalance) {
		dependencyError(w, r)
		return
	}
	writeJSON(w, 200, object{"candidate_forecast_id": c.CandidateForecastId, "reference_forecast_id": c.ReferenceForecastId, "snapshot_id": c.SnapshotId, "model_context_id": c.ModelContextId, "is_stale": c.IsStale, "candidate_metrics": metricsDTO(c.CandidateMetrics), "reference_metrics": metricsDTO(c.ReferenceMetrics), "delta_max_shortfall": moneyDTO(c.DeltaMaxShortfall), "delta_minimum_balance": moneyDTO(c.DeltaMinimumBalance), "delta_closing_balance": moneyDTO(c.DeltaClosingBalance), "delta_gap_probability": c.DeltaGapProbability, "delta_first_gap_days": c.DeltaFirstGapDays})
}
func validComparisonMetrics(m *corev1.ForecastMetrics) bool {
	return m != nil && nonnegativeMoney(m.MaxShortfall) && validMoney(m.MinimumBalance) && validMoney(m.ClosingBalance) && validProbability(m.GapProbability) && (m.FirstGap == nil || businessDate(m.FirstGap.Date) && positiveMoney(m.FirstGap.Shortfall))
}

func salesChangeBody(f map[string]json.RawMessage) (*corev1.ScenarioChange, bool) {
	for key := range f {
		if !contains(key, "id", "type", "targets", "period_start", "period_end", "reduction_basis_points") {
			return nil, false
		}
	}
	v := &corev1.ReduceSalesChange{}
	if json.Unmarshal(f["id"], &v.Id) != nil || !resourceID(v.Id) || json.Unmarshal(f["period_start"], &v.PeriodStart) != nil || json.Unmarshal(f["period_end"], &v.PeriodEnd) != nil || !businessDate(v.PeriodStart) || !businessDate(v.PeriodEnd) || v.PeriodStart > v.PeriodEnd || json.Unmarshal(f["reduction_basis_points"], &v.ReductionBasisPoints) != nil || v.ReductionBasisPoints < 1 || v.ReductionBasisPoints > 10000 {
		return nil, false
	}
	var targets []map[string]json.RawMessage
	if json.Unmarshal(f["targets"], &targets) != nil || len(targets) < 1 || len(targets) > 1000 {
		return nil, false
	}
	seen := map[string]bool{}
	for _, t := range targets {
		var component, occurrence string
		if len(t) != 2 || json.Unmarshal(t["component_id"], &component) != nil || json.Unmarshal(t["occurrence_id"], &occurrence) != nil || !nonempty(component, 128) || component != occurrence || seen[component] {
			return nil, false
		}
		seen[component] = true
		v.Targets = append(v.Targets, &corev1.OccurrenceTarget{ComponentId: component, OccurrenceId: occurrence})
	}
	return &corev1.ScenarioChange{Change: &corev1.ScenarioChange_ReduceSales{ReduceSales: v}}, true
}
