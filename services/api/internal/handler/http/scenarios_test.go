package httpapi

import (
	"context"
	"log/slog"
	"testing"

	"google.golang.org/grpc"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

type scenarioFixture struct {
	corev1.ScenariosServiceClient
	calls int
}

func (f *scenarioFixture) CreateScenario(_ context.Context, r *corev1.CreateScenarioRequest, _ ...grpc.CallOption) (*corev1.CreateScenarioResponse, error) {
	f.calls++
	return &corev1.CreateScenarioResponse{Accepted: &corev1.ScenarioAccepted{Id: forecastID, ForecastId: accountID, Status: corev1.ForecastStatus_FORECAST_STATUS_QUEUED, PollAfterMs: 1000}}, nil
}
func TestScenarioRouteValidatesBeforeDispatch(t *testing.T) {
	f := newFinancialFixture()
	scenarios := &scenarioFixture{}
	h := NewWithCore(f, "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{Companies: f, Ledger: f, Forecasts: f, Scenarios: scenarios})
	body := `{"name":"Purchase","base_forecast_id":"` + forecastID + `","changes":[{"id":"` + forecastID + `","type":"one_off_expense","date":"2026-10-25","amount":{"amount_minor":"10000","currency":"RUB"},"description":"Equipment"}]}`
	if w := financialRequest(h, "POST", financialRoot+"/scenarios", body); w.Code != 202 || scenarios.calls != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, body := range []string{`{"name":"x","comparison_parent_scenario_id":"` + forecastID + `","response_action":{}}`, `{"name":"x","base_forecast_id":"` + forecastID + `","changes":[{"type":"reduce_sales"}]}`, `{"name":"x","name":"y"}`, `{"name":"x","base_forecast_id":"` + forecastID + `","changes":[]}`} {
		if w := financialRequest(h, "POST", financialRoot+"/scenarios", body); w.Code != 422 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if scenarios.calls != 1 {
		t.Fatal("invalid requests reached Core")
	}
}
