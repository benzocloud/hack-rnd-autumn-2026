package httpapi

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func analysisForecastFixture() *financialFixture {
	f := csvCalendarFixture()
	i := f.forecast.Input
	i.ModelContext.ModelVersion = "mean_90_csv_calendar_analysis_v2"
	i.ModelContext.PreprocessingVersion = "csv_observed_operations_calendar_analysis_v2"
	i.ModelContext.ConfigurationId = i.ModelContext.ModelVersion
	a := &pb.ForecastAnalysis{AnalysisVersion: "text_categories_monthly_v1", WindowFrom: i.HistoryAssessment.FromDate, WindowThrough: i.CutoffDate}
	ids := []string{"019a0c65-9800-7000-8000-000000000101", "019a0c65-9800-7000-8000-000000000102", "019a0c65-9800-7000-8000-000000000103"}
	h := &pb.RecurringHypothesis{PatternId: "monthly_opaque_hash", RuleId: "monthly_fit", RuleVersion: a.AnalysisVersion, CategoryCode: "rent", Direction: pb.Direction_DIRECTION_OUTFLOW, MonthlyAnchor: &pb.RecurringHypothesis_DayOfMonth{DayOfMonth: 5}, WeekendShift: pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NONE, ExpectedAmountMinor: 900, Confirmations: 3, ExpectedPeriods: 3, FirstEvidenceDate: a.WindowFrom, LastEvidenceDate: a.WindowThrough, Status: pb.RecurringHypothesisStatus_RECURRING_HYPOTHESIS_STATUS_HYPOTHESIS}
	for _, id := range ids {
		a.ClassifiedOperations = append(a.ClassifiedOperations, &pb.ClassifiedOperationAnalysis{TransactionId: id, Version: 1, CategoryCode: "rent", CategorySource: "rules", CategoryRuleId: "rental_text"})
		h.HistoricalOperations = append(h.HistoricalOperations, &pb.HistoricalOperationRef{TransactionId: id, Version: 1})
	}
	a.RecurringHypotheses = []*pb.RecurringHypothesis{h}
	f.forecast.GetSucceeded().Result.Analysis = a
	return f
}

func TestForecastAnalysisProjectionIsExplicitAndBackwardCompatible(t *testing.T) {
	f := analysisForecastFixture()
	for _, calendar := range []bool{true, false} {
		if !calendar {
			i := f.forecast.Input
			i.CalendarKnowledgeMode = pb.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE
			i.CalendarAsOfDate = ""
			i.NoPostCutoffMovements = false
			i.ModelContext.ModelVersion = "mean_90_csv_analysis_v2"
			i.ModelContext.PreprocessingVersion = "csv_observed_operations_analysis_v2"
			i.ModelContext.ConfigurationId = i.ModelContext.ModelVersion
			i.HistoryAssessment.Limitations = []string{"csv_history_completeness_unverified", "unobserved_days_unknown", "observed_operations_mean_per_calendar_day", "historical_calendar_unavailable", "historical_availability_unverified", "not_calibrated"}
			f.forecast.Readiness.Limitations = append([]string{}, i.HistoryAssessment.Limitations...)
		}
		w := financialRequest(f.handler(), "GET", financialRoot+"/forecasts/"+forecastID, "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		var body struct {
			Result struct {
				Analysis struct {
					Version    string           `json:"analysis_version"`
					Operations []map[string]any `json:"classified_operations"`
					Hypotheses []map[string]any `json:"recurring_hypotheses"`
				} `json:"analysis"`
			} `json:"result"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Result.Analysis.Version != "text_categories_monthly_v1" || len(body.Result.Analysis.Operations) != 3 || len(body.Result.Analysis.Hypotheses) != 1 || body.Result.Analysis.Operations[0]["version"] != "1" || body.Result.Analysis.Hypotheses[0]["expected_amount_minor"] != "900" {
			t.Fatal(err, w.Body)
		}
	}
	f = analysisForecastFixture()
	a := f.forecast.GetSucceeded().Result.Analysis
	a.RecurringHypotheses[0].MonthlyAnchor = &pb.RecurringHypothesis_MonthEnd{MonthEnd: &emptypb.Empty{}}
	a.RecurringHypotheses[0].WeekendShift = pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NEXT
	if !validForecastResult(f.forecast.GetSucceeded().Result, f.forecast.Input) {
		t.Fatal("month end rejected")
	}
	if forecastAnalysisDTO(a).(object)["recurring_hypotheses"].([]object)[0]["month_end"] != true {
		t.Fatal("month end lost")
	}
	a.RecurringHypotheses = nil
	a.ClassifiedOperations[0].CategorySource = "manual"
	a.ClassifiedOperations[0].CategoryRuleId = ""
	a.ClassifiedOperations[0].CategoryId = proto.String("019a0c65-9800-7000-8000-000000000104")
	a.ClassifiedOperations[1].CategorySource = "unknown"
	a.ClassifiedOperations[1].CategoryCode = "unknown"
	a.ClassifiedOperations[1].CategoryRuleId = "no_matching_rule"
	if !validForecastResult(f.forecast.GetSucceeded().Result, f.forecast.Input) {
		t.Fatal("manual/unknown labels rejected")
	}
	if forecastAnalysisDTO(nil) != nil {
		t.Fatal("legacy result became analyzed")
	}
	old := csvCalendarFixture()
	if !validForecastResult(old.forecast.GetSucceeded().Result, old.forecast.Input) {
		t.Fatal("legacy calendar broken")
	}
	old.forecast.GetSucceeded().Result.Analysis = a
	if validForecastResult(old.forecast.GetSucceeded().Result, old.forecast.Input) {
		t.Fatal("legacy silently gained analysis")
	}
}

func TestForecastAnalysisRejectsCorruptCoreProjection(t *testing.T) {
	cases := map[string]func(*pb.ForecastAnalysis){
		"missing": func(a *pb.ForecastAnalysis) { a.AnalysisVersion = "" }, "version": func(a *pb.ForecastAnalysis) { a.AnalysisVersion = "unknown" }, "window": func(a *pb.ForecastAnalysis) { a.WindowFrom = a.WindowThrough }, "empty labels": func(a *pb.ForecastAnalysis) { a.ClassifiedOperations = nil }, "nil label": func(a *pb.ForecastAnalysis) { a.ClassifiedOperations[0] = nil }, "bad id": func(a *pb.ForecastAnalysis) { a.ClassifiedOperations[0].TransactionId = "foreign" }, "stale version": func(a *pb.ForecastAnalysis) { a.ClassifiedOperations[0].Version = 0 }, "duplicate labels": func(a *pb.ForecastAnalysis) { a.ClassifiedOperations[1] = a.ClassifiedOperations[0] }, "source": func(a *pb.ForecastAnalysis) { a.ClassifiedOperations[0].CategorySource = "forged" }, "manual id": func(a *pb.ForecastAnalysis) {
			a.ClassifiedOperations[0].CategorySource = "manual"
			a.ClassifiedOperations[0].CategoryRuleId = ""
		}, "manual rule": func(a *pb.ForecastAnalysis) {
			a.ClassifiedOperations[0].CategorySource = "manual"
			a.ClassifiedOperations[0].CategoryId = proto.String(forecastID)
		}, "unknown": func(a *pb.ForecastAnalysis) { a.ClassifiedOperations[0].CategorySource = "unknown" }, "empty rule": func(a *pb.ForecastAnalysis) { a.ClassifiedOperations[0].CategoryRuleId = "" }, "nil hypothesis": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0] = nil }, "rule": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0].RuleId = "unknown" }, "hypothesis status": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0].Status = 0 }, "direction": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0].Direction = 0 }, "amount": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0].ExpectedAmountMinor = 0 }, "confirmations": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0].Confirmations = 2 }, "periods": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0].ExpectedPeriods = 5 }, "first date": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0].FirstEvidenceDate = "bad" }, "anchor missing": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0].MonthlyAnchor = nil }, "anchor bad": func(a *pb.ForecastAnalysis) {
			a.RecurringHypotheses[0].MonthlyAnchor = &pb.RecurringHypothesis_DayOfMonth{DayOfMonth: 32}
		}, "month end bad": func(a *pb.ForecastAnalysis) {
			a.RecurringHypotheses[0].MonthlyAnchor = &pb.RecurringHypothesis_MonthEnd{}
		}, "shift": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0].WeekendShift = 0 }, "duplicate pattern": func(a *pb.ForecastAnalysis) {
			a.RecurringHypotheses = append(a.RecurringHypotheses, a.RecurringHypotheses[0])
		}, "nil ref": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0].HistoricalOperations[0] = nil }, "foreign evidence": func(a *pb.ForecastAnalysis) {
			a.RecurringHypotheses[0].HistoricalOperations[0].TransactionId = forecastID
		}, "stale evidence": func(a *pb.ForecastAnalysis) { a.RecurringHypotheses[0].HistoricalOperations[0].Version++ }, "duplicate evidence": func(a *pb.ForecastAnalysis) {
			a.RecurringHypotheses[0].HistoricalOperations[1] = a.RecurringHypotheses[0].HistoricalOperations[0]
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := analysisForecastFixture()
			mutate(f.forecast.GetSucceeded().Result.Analysis)
			w := financialRequest(f.handler(), "GET", financialRoot+"/forecasts/"+forecastID, "")
			if w.Code != 503 {
				t.Fatal("corrupt Core response accepted", w.Code, w.Body)
			}
		})
	}
	f := analysisForecastFixture()
	f.forecast.GetSucceeded().Result.Analysis = nil
	if validForecastResult(f.forecast.GetSucceeded().Result, f.forecast.Input) {
		t.Fatal("missing required analysis accepted")
	}
}
