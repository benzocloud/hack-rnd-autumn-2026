package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func csvCalendarFixture() *financialFixture {
	f := newFinancialFixture()
	succeeded(f)
	i := f.forecast.Input
	cutoff, _ := time.Parse(time.DateOnly, i.CutoffDate)
	i.HistoryCoverage = nil
	i.CalendarKnowledgeMode = pb.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_CURRENT
	i.CalendarAsOfDate = i.StartDate
	i.NoPostCutoffMovements = true
	i.ModelContext = &pb.ForecastModelContext{Id: forecastID, ModelName: "mean_90", ModelVersion: "mean_90_csv_calendar_v1", PreprocessingVersion: "csv_observed_operations_calendar_v1", ConfigurationId: "mean_90_csv_calendar_v1"}
	limits := []string{"csv_history_completeness_unverified", "unobserved_days_unknown", "observed_operations_mean_per_calendar_day", "historical_availability_unverified", "not_calibrated", "current_calendar_only"}
	i.HistoryAssessment = &pb.HistoryAssessment{RuleVersion: "csv_history_quality_v1", FromDate: cutoff.AddDate(0, 0, -89).Format(time.DateOnly), ThroughDate: i.CutoffDate, DataVersion: i.DataVersion, OperationDays: 2, BalanceDays: 2, Limitations: limits}
	f.forecast.Readiness = &pb.ForecastReadiness{Status: "limited", HistoryDays: 90, ObservedDays: 2, Limitations: append([]string{}, limits...)}
	result := f.forecast.GetSucceeded().Result
	result.Metrics.GapProbability = notCalibratedProbability()
	for _, point := range result.DailyPoints {
		point.NegativeBalanceProbability = notCalibratedProbability()
	}
	return f
}
func notCalibratedProbability() *pb.Probability {
	return &pb.Probability{Estimate: &pb.Probability_UnavailableReason{UnavailableReason: "not_calibrated"}}
}
func TestCSVCalendarForecastProjectionPreservesTypedContextAndNullUncertainty(t *testing.T) {
	t.Parallel()
	f := csvCalendarFixture()
	result := f.forecast.GetSucceeded().Result
	i := f.forecast.Input
	result.DailyPoints[0].Outflow = rub(100)
	for _, point := range result.DailyPoints {
		point.Balance = rub(-100)
	}
	result.Metrics.MinimumBalance = rub(-100)
	result.Metrics.ClosingBalance = rub(-100)
	result.Metrics.MaxShortfall = rub(100)
	result.Metrics.FirstGap = &pb.CashGap{Date: i.StartDate, Shortfall: rub(100)}
	result.Components = []*pb.ForecastComponent{{Id: "known-component", OccurrenceId: "known-occurrence", Kind: "known_payment", PaymentId: proto.String(paymentTestID), Direction: pb.Direction_DIRECTION_OUTFLOW, Date: i.StartDate, Amount: rub(100), RemainingAmount: rub(100), Source: &pb.RecordSource{Kind: pb.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL, SourceSystem: "manual_v1"}}}
	result.EffectiveOccurrences = []*pb.EffectiveOccurrence{{EffectiveOccurrenceId: "effective-known", ComponentId: "known-component", OccurrenceId: "known-occurrence", PaymentId: proto.String(paymentTestID), Direction: pb.Direction_DIRECTION_OUTFLOW, Date: i.StartDate, RemainingAmount: rub(100), InHorizon: true, ResponseActionEligible: true}}
	w := financialRequest(f.handler(), "GET", financialRoot+"/forecasts/"+forecastID, "")
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	var body struct {
		Input struct {
			ModelContext          map[string]any `json:"model_context"`
			CalendarKnowledgeMode string         `json:"calendar_knowledge_mode"`
			CalendarAsOfDate      string         `json:"calendar_as_of_date"`
			NoPostCutoffMovements bool           `json:"no_post_cutoff_movements"`
			HistoryCoverage       any            `json:"history_coverage"`
			HistoryAssessment     map[string]any `json:"history_assessment"`
		} `json:"input"`
		Result struct {
			Metrics struct {
				GapProbability map[string]any `json:"gap_probability"`
			} `json:"metrics"`
			DailyPoints []map[string]any `json:"daily_points"`
			Components  []map[string]any `json:"components"`
			Effective   []map[string]any `json:"effective_occurrences"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Input.ModelContext["model_version"] != "mean_90_csv_calendar_v1" || body.Input.CalendarKnowledgeMode != "current" || body.Input.CalendarAsOfDate != i.StartDate || !body.Input.NoPostCutoffMovements || body.Input.HistoryCoverage != nil || body.Input.HistoryAssessment == nil {
		t.Fatalf("lost explicit CSV/CURRENT context: %s", w.Body)
	}
	if len(body.Result.DailyPoints) != 30 || len(body.Result.Components) != 1 || body.Result.Components[0]["payment_id"] != paymentTestID || len(body.Result.Effective) != 1 || body.Result.Effective[0]["remaining_amount"].(map[string]any)["amount_minor"] != "100" {
		t.Fatalf("lost financial explanation: %s", w.Body)
	}
	if body.Result.Metrics.GapProbability["value"] != nil || body.Result.Metrics.GapProbability["unavailable_reason"] != "not_calibrated" {
		t.Fatal("invented calibrated probability")
	}
	for _, point := range body.Result.DailyPoints {
		if point["balance_lower"] != nil || point["balance_upper"] != nil || point["negative_balance_probability"].(map[string]any)["value"] != nil {
			t.Fatal("invented band/probability")
		}
	}
	if strings.Contains(w.Body.String(), "historical_calendar_unavailable") {
		t.Fatal("CURRENT calendar mislabeled unavailable")
	}
}
func TestCSVCalendarForecastRejectsUnknownOrIncompatibleContext(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*financialFixture)
	}{
		{"unknown profile", func(f *financialFixture) { f.forecast.Input.ModelContext.ModelVersion = "mean_90_csv_calendar_v9" }},
		{"old preprocessing", func(f *financialFixture) {
			f.forecast.Input.ModelContext.PreprocessingVersion = "csv_observed_operations_v1"
		}},
		{"old configuration", func(f *financialFixture) { f.forecast.Input.ModelContext.ConfigurationId = "mean_90_csv_v1" }},
		{"unknown mode", func(f *financialFixture) { f.forecast.Input.CalendarKnowledgeMode = 99 }},
		{"unavailable calendar", func(f *financialFixture) {
			i := f.forecast.Input
			i.CalendarKnowledgeMode = pb.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE
			i.CalendarAsOfDate = ""
			i.NoPostCutoffMovements = false
		}},
		{"historical scenario", func(f *financialFixture) {
			f.forecast.Input.CalendarKnowledgeMode = pb.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_SYNTHETIC_SCENARIO
		}},
		{"future factual bridge missing", func(f *financialFixture) { f.forecast.Input.NoPostCutoffMovements = false }},
		{"stale asof", func(f *financialFixture) { f.forecast.Input.CalendarAsOfDate = "2026-10-22" }},
		{"missing assessment", func(f *financialFixture) { f.forecast.Input.HistoryAssessment = nil }},
		{"fake certified coverage", func(f *financialFixture) {
			f.forecast.Input.HistoryCoverage = &pb.HistoryCoverage{Kind: pb.HistoryCoverageKind_HISTORY_COVERAGE_KIND_CERTIFIED}
		}},
		{"missing current limitation", func(f *financialFixture) {
			f.forecast.Input.HistoryAssessment.Limitations = f.forecast.Input.HistoryAssessment.Limitations[:5]
		}},
		{"missing readiness limitation", func(f *financialFixture) { f.forecast.Readiness.Limitations = f.forecast.Readiness.Limitations[:5] }},
		{"fully ready", func(f *financialFixture) { f.forecast.Readiness.Status = "ready" }},
		{"random seed", func(f *financialFixture) { f.forecast.Input.ModelContext.RandomSeed = proto.String("1") }},
		{"trajectory count", func(f *financialFixture) { f.forecast.Input.ModelContext.TrajectoryCount = proto.Uint32(100) }},
		{"fabricated probability", func(f *financialFixture) {
			f.forecast.GetSucceeded().Result.Metrics.GapProbability = &pb.Probability{Estimate: &pb.Probability_Value{Value: 0}}
		}},
		{"fabricated daily probability", func(f *financialFixture) {
			f.forecast.GetSucceeded().Result.DailyPoints[0].NegativeBalanceProbability = &pb.Probability{Estimate: &pb.Probability_Value{Value: 0}}
		}},
		{"fabricated bands", func(f *financialFixture) {
			p := f.forecast.GetSucceeded().Result.DailyPoints[0]
			p.BalanceLower = rub(0)
			p.BalanceUpper = rub(0)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := csvCalendarFixture()
			tc.change(f)
			w := financialRequest(f.handler(), "GET", financialRoot+"/forecasts/"+forecastID, "")
			if w.Code != 503 {
				t.Fatalf("unsupported context accepted status=%d body=%s", w.Code, w.Body)
			}
		})
	}
}
func TestCSVCalendarAcceptsSameDayAsOfWithoutInventingBridge(t *testing.T) {
	t.Parallel()
	f := csvCalendarFixture()
	f.forecast.Input.CalendarAsOfDate = f.forecast.Input.CutoffDate
	f.forecast.Input.NoPostCutoffMovements = false
	if !validForecast(f.forecast, f.account, forecastID) {
		t.Fatal("valid same-day CURRENT CSV calendar rejected")
	}
}
