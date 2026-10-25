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
	if fields["response_action"] != nil || fields["comparison_parent_scenario_id"] != nil {
		writeError(w, r, 422, "SCENARIO_NOT_SUPPORTED", nil)
		return
	}
	name, ok := paymentText(fields, "name", 200)
	if !ok || name == nil || strings.TrimSpace(*name) == "" {
		writeError(w, r, 422, "VALIDATION_ERROR", nil)
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
	response, err := h.core.Scenarios.CreateScenario(r.Context(), &corev1.CreateScenarioRequest{Context: writeContext(r), Scope: scopeFor(account), IdempotencyKey: keys[0], Name: *name, Scenario: &corev1.CreateScenarioRequest_Independent{Independent: &corev1.IndependentScenario{BaseForecastId: base, Changes: changes}}})
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
	if s == nil || !resourceID(s.Id) || !resourceID(s.BaseForecastId) || !resourceID(s.ForecastId) || !nonempty(s.Name, 200) || !timestamp(s.CreatedAt) || s.ComparisonParentScenarioId != nil || s.ResponseAction != nil || len(s.Changes) < 1 || len(s.Changes) > 20 {
		return nil, false
	}
	changes := []object{}
	for _, c := range s.Changes {
		v := c.GetOneOffExpense()
		if v == nil || !resourceID(v.Id) || !businessDate(v.Date) || !positiveMoney(v.Amount) || !nonempty(v.Description, 1000) || v.CategoryId != nil {
			return nil, false
		}
		changes = append(changes, object{"id": v.Id, "type": "one_off_expense", "date": v.Date, "amount": moneyDTO(v.Amount), "description": v.Description, "category_id": nil})
	}
	return object{"id": s.Id, "name": s.Name, "base_forecast_id": s.BaseForecastId, "forecast_id": s.ForecastId, "comparison_parent_scenario_id": nil, "changes": changes, "response_action": nil, "out_of_horizon_occurrence_ids": stringsDTO(s.OutOfHorizonOccurrenceIds), "created_at": utc(s.CreatedAt)}, true
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
