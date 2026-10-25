package ml

import (
	"testing"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func TestAnalysisValidatesFactsAndKeepsCalendarMoneyGuards(t *testing.T) {
	input := csvCalendarFixture(t)
	input.Snapshot.Model = forecasting.ModelContext{Name: "mean_90", Version: forecasting.CSVCalendarAnalysisVersion, PreprocessingVersion: forecasting.CSVCalendarAnalysisPreprocessing}
	request, err := encodeFinancial(input)
	if err != nil {
		t.Fatal(err)
	}
	base := proto.Clone(request).(*pb.PredictRequest)
	base.Model.ModelVersion = forecasting.CSVCalendarModelVersion
	base.Model.PreprocessingVersion = forecasting.CSVCalendarPreprocessing
	base.Model.ConfigurationId = forecasting.CSVCalendarModelVersion
	response := csvCalendarResponse(t, base)
	response.Model = request.Model
	response.ClassifiedOperations = nil
	for _, op := range request.Operations {
		label := proto.Clone(op).(*pb.FinancialOperation)
		label.CategoryRuleId = "empty_purpose"
		response.ClassifiedOperations = append(response.ClassifiedOperations, label)
	}
	response.AnalysisContext = &pb.AnalysisContext{AnalysisVersion: forecasting.AnalysisVersion, WindowFrom: request.CsvHistory.FromDate, WindowThrough: request.Snapshot.CutoffDate}
	if err := validateFinancialResponse(input, request, response); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*pb.PredictResponse){
		"amount":           func(r *pb.PredictResponse) { r.ClassifiedOperations[0].InflowMinor++ },
		"date":             func(r *pb.PredictResponse) { r.ClassifiedOperations[0].Date = r.StartDate },
		"version":          func(r *pb.PredictResponse) { r.ClassifiedOperations[0].Version++ },
		"id":               func(r *pb.PredictResponse) { r.ClassifiedOperations[0].OperationId = uuid.NewString() },
		"duplicate":        func(r *pb.PredictResponse) { r.ClassifiedOperations[1] = r.ClassifiedOperations[0] },
		"party":            func(r *pb.PredictResponse) { r.ClassifiedOperations[0].CounterpartyInn = "forged" },
		"purpose":          func(r *pb.PredictResponse) { r.ClassifiedOperations[0].PaymentPurpose = "forged" },
		"provenance":       func(r *pb.PredictResponse) { r.ClassifiedOperations[0].FactualSourceKind = "manual" },
		"rule":             func(r *pb.PredictResponse) { r.ClassifiedOperations[0].CategoryRuleId = "unknown_rule" },
		"label":            func(r *pb.PredictResponse) { r.ClassifiedOperations[0].CategoryCode = "invented" },
		"category UUID":    func(r *pb.PredictResponse) { r.ClassifiedOperations[0].CategoryId = new(uuid.NewString()) },
		"source":           func(r *pb.PredictResponse) { r.ClassifiedOperations[0].CategorySource = "user" },
		"analysis version": func(r *pb.PredictResponse) { r.AnalysisContext.AnalysisVersion = "unknown" },
		"window":           func(r *pb.PredictResponse) { r.AnalysisContext.WindowFrom = r.CutoffDate },
		"missing analysis": func(r *pb.PredictResponse) { r.AnalysisContext = nil },
		"context": func(r *pb.PredictResponse) {
			r.Model = proto.Clone(r.Model).(*pb.ModelContext)
			r.Model.ConfigurationId = "old"
		},
		"monetary": func(r *pb.PredictResponse) { r.Days[0].InflowMinor++ },
		"evidence": func(r *pb.PredictResponse) { r.HistoryEvidence[0].HistoricalOperationIds[0] = uuid.NewString() },
		"foreign hypothesis": func(r *pb.PredictResponse) {
			r.RecurringHypotheses = []*pb.RecurringHypothesis{{RuleId: "monthly_fit", RuleVersion: forecasting.AnalysisVersion, Status: pb.RecurringHypothesisStatus_RECURRING_HYPOTHESIS_STATUS_HYPOTHESIS, ExpectedAmountMinor: 1, HistoricalOperations: []*pb.OperationVersionReference{{OperationId: uuid.NewString(), Version: 1}}}}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			damaged := proto.Clone(response).(*pb.PredictResponse)
			mutate(damaged)
			if validateFinancialResponse(input, request, damaged) == nil {
				t.Fatal("corruption accepted")
			}
		})
	}
	// Legacy decoder still rejects even fully valid analysis: no silent activation.
	legacy := input
	legacy.Snapshot.Model = baseAnalysisContext(input.Snapshot.Model)
	legacyResponse := proto.Clone(response).(*pb.PredictResponse)
	legacyResponse.Model = base.Model
	if validateFinancialResponse(legacy, base, legacyResponse) == nil {
		t.Fatal("legacy accepted derived output")
	}
}

func TestMonthlyPatternThresholdsDatesAmountsAndAnchor(t *testing.T) {
	facts := map[string]*pb.FinancialOperation{}
	h := &pb.RecurringHypothesis{Direction: pb.RecurringDirection_RECURRING_DIRECTION_OUTFLOW, CategoryCode: "rent", ExpectedAmountMinor: 900, Confirmations: 3, ExpectedPeriods: 3, FirstEvidenceDate: "2026-07-05", LastEvidenceDate: "2026-09-05", MonthlyAnchor: &pb.RecurringHypothesis_DayOfMonth{DayOfMonth: 5}, WeekendShift: pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NONE}
	for _, date := range []string{"2026-07-05", "2026-08-05", "2026-09-05"} {
		id := uuid.NewString()
		facts[id] = &pb.FinancialOperation{Date: date, OutflowMinor: 900}
		h.HistoricalOperations = append(h.HistoricalOperations, &pb.OperationVersionReference{OperationId: id, Version: 1})
	}
	if !validMonthlyPattern(h, facts, "2026-09-29") {
		t.Fatal("valid monthly evidence rejected")
	}
	mutations := map[string]func(*pb.RecurringHypothesis){"amount": func(h *pb.RecurringHypothesis) { h.ExpectedAmountMinor++ }, "confirmations": func(h *pb.RecurringHypothesis) { h.Confirmations = 2 }, "periods": func(h *pb.RecurringHypothesis) { h.ExpectedPeriods = 4 }, "anchor": func(h *pb.RecurringHypothesis) { h.MonthlyAnchor = &pb.RecurringHypothesis_DayOfMonth{DayOfMonth: 20} }, "shift": func(h *pb.RecurringHypothesis) { h.WeekendShift = 0 }, "insufficient incoming": func(h *pb.RecurringHypothesis) { h.Direction = pb.RecurringDirection_RECURRING_DIRECTION_INFLOW }, "date": func(h *pb.RecurringHypothesis) { h.FirstEvidenceDate = "2026-06-01" }}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			v := proto.Clone(h).(*pb.RecurringHypothesis)
			mutate(v)
			if validMonthlyPattern(v, facts, "2026-09-29") {
				t.Fatal("invalid pattern accepted")
			}
		})
	}
}
