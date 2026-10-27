package httpapi

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func TestDelayAndResponseRESTMappingAndHybridRejection(t *testing.T) {
	f := newFinancialFixture()
	scenarios := &scenarioFixture{}
	h := NewWithCore(f, "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{Companies: f, Ledger: f, Forecasts: f, Scenarios: scenarios})
	delay := `{"name":"Delayed receipt","base_forecast_id":"` + forecastID + `","changes":[{"id":"` + forecastID + `","type":"delay_receipt","target":{"component_id":"sales-target","occurrence_id":"sales-target"},"amount":{"amount_minor":"50","currency":"RUB"},"new_date":"2026-11-23"}]}`
	if w := financialRequest(h, "POST", financialRoot+"/scenarios", delay); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	mapped := scenarios.request.GetIndependent().Changes[0].GetDelayReceipt()
	if mapped == nil || mapped.Amount.AmountMinor != 50 || mapped.NewDate != "2026-11-23" {
		t.Fatal("delay not mapped")
	}
	response := `{"name":"Supplier deferral","comparison_parent_scenario_id":"` + forecastID + `","response_action":{"type":"reschedule_payment_part","target":{"effective_occurrence_id":"effective-target"},"amount":{"amount_minor":"100","currency":"RUB"},"new_date":"2026-11-23","feasibility_condition":"Supplier agrees"}}`
	if w := financialRequest(h, "POST", financialRoot+"/scenarios", response); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	action := scenarios.request.GetResponse()
	if action == nil || action.ComparisonParentScenarioId != forecastID || action.ResponseAction.EffectiveOccurrenceId != "effective-target" || scenarios.request.GetIndependent() != nil {
		t.Fatal("response not mapped")
	}
	for _, bad := range []string{strings.Replace(response, `"name":"Supplier deferral"`, `"name":"Supplier deferral","base_forecast_id":"`+forecastID+`"`, 1), strings.Replace(response, `"name":"Supplier deferral"`, `"name":"Supplier deferral","changes":[]`, 1), strings.Replace(response, `"Supplier agrees"`, `" "`, 1), strings.Replace(response, `"100"`, `"0"`, 1), strings.Replace(delay, `"sales-target"`, `""`, 1), strings.Replace(delay, `"2026-11-23"`, `"not-a-date"`, 1)} {
		if w := financialRequest(h, "POST", financialRoot+"/scenarios", bad); w.Code != 422 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if scenarios.calls != 2 {
		t.Fatal("invalid requests reached Core")
	}
	parent := forecastID
	dto, ok := scenarioDTO(&corev1.Scenario{Id: forecastID, ForecastId: accountID, BaseForecastId: forecastID, Name: "Response", CreatedAt: f.forecast.CreatedAt, ComparisonParentScenarioId: &parent, ResponseAction: action.ResponseAction, Changes: []*corev1.ScenarioChange{{Change: &corev1.ScenarioChange_DelayReceipt{DelayReceipt: mapped}}}, OutOfHorizonOccurrenceIds: []string{"future-part"}})
	if !ok || dto["comparison_parent_scenario_id"] != parent || dto["response_action"] == nil {
		t.Fatal("response read DTO")
	}
}
