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
