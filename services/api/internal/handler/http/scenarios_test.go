package httpapi

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

type scenarioFixture struct {
	corev1.ScenariosServiceClient
	calls   int
	request *corev1.CreateScenarioRequest
}

func (f *scenarioFixture) CreateScenario(_ context.Context, r *corev1.CreateScenarioRequest, _ ...grpc.CallOption) (*corev1.CreateScenarioResponse, error) {
	f.calls++
	f.request = r
	return &corev1.CreateScenarioResponse{Accepted: &corev1.ScenarioAccepted{Id: forecastID, ForecastId: accountID, Status: corev1.ForecastStatus_FORECAST_STATUS_QUEUED, PollAfterMs: 1000}}, nil
}

func TestSalesScenarioRequestMappingAndStrictFields(t *testing.T) {
	f := newFinancialFixture()
	scenarios := &scenarioFixture{}
	h := NewWithCore(f, "https://localhost", slog.New(slog.DiscardHandler), func(context.Context) bool { return true }, CoreClients{Companies: f, Ledger: f, Forecasts: f, Scenarios: scenarios})
	change := `{"id":"` + forecastID + `","type":"reduce_sales","targets":[{"component_id":"sales-target","occurrence_id":"sales-target"}],"period_start":"2026-10-24","period_end":"2026-11-22","reduction_basis_points":2500}`
	body := `{"name":"Sales -25%","base_forecast_id":"` + forecastID + `","changes":[` + change + `]}`
	if w := financialRequest(h, "POST", financialRoot+"/scenarios", body); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	reduction := scenarios.request.GetIndependent().Changes[0].GetReduceSales()
	if reduction == nil || reduction.ReductionBasisPoints != 2500 || len(reduction.Targets) != 1 || reduction.Targets[0].ComponentId != "sales-target" {
		t.Fatal("reduction not mapped")
	}
	for _, oldNew := range [][2]string{{`2500`, `10001`}, {`2500`, `0`}, {`2500`, `2.5`}, {`"occurrence_id":"sales-target"`, `"occurrence_id":"other"`}, {`"period_end":"2026-11-22"`, `"period_end":"2026-10-01"`}, {`"reduction_basis_points":2500`, `"reduction_basis_points":2500,"amount":{}`}} {
		invalid := strings.Replace(body, oldNew[0], oldNew[1], 1)
		if w := financialRequest(h, "POST", financialRoot+"/scenarios", invalid); w.Code != 422 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if scenarios.calls != 1 {
		t.Fatal("invalid reduction reached Core")
	}
	value, ok := scenarioDTO(&corev1.Scenario{Id: forecastID, BaseForecastId: forecastID, ForecastId: accountID, Name: "sales", CreatedAt: f.forecast.CreatedAt, Changes: scenarios.request.GetIndependent().Changes})
	if !ok || len(value["changes"].([]object)) != 1 {
		t.Fatal("saved reduction not serializable")
	}
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
