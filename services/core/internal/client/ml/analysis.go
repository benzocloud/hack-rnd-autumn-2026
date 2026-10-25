package ml

import (
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func encodeAnalysis(input forecasting.FinancialModelInput) (*pb.PredictRequest, error) {
	if !forecasting.ValidCSVContext(input.Snapshot.Model) {
		return nil, forecasting.ErrModelContext
	}
	base := input
	base.Snapshot.Model = baseAnalysisContext(input.Snapshot.Model)
	request, err := encodeFinancial(base)
	if err != nil {
		return nil, err
	}
	request.Model = &pb.ModelContext{ModelName: input.Snapshot.Model.Name, ModelVersion: input.Snapshot.Model.Version, PreprocessingVersion: input.Snapshot.Model.PreprocessingVersion, ConfigurationId: input.Snapshot.Model.Version}
	return request, nil
}
func baseAnalysisContext(m forecasting.ModelContext) forecasting.ModelContext {
	if m.Version == forecasting.CSVCalendarAnalysisVersion {
		return forecasting.ModelContext{Name: "mean_90", Version: forecasting.CSVCalendarModelVersion, PreprocessingVersion: forecasting.CSVCalendarPreprocessing}
	}
	return forecasting.ModelContext{Name: "mean_90", Version: forecasting.CSVModelVersion, PreprocessingVersion: forecasting.CSVPreprocessing}
}

func validateAnalysisResponse(input forecasting.FinancialModelInput, request *pb.PredictRequest, response *pb.PredictResponse) error {
	if response == nil || !proto.Equal(response.Model, request.Model) || response.AnalysisContext == nil || response.AnalysisContext.AnalysisVersion != forecasting.AnalysisVersion || response.AnalysisContext.WindowFrom != request.CsvHistory.FromDate || response.AnalysisContext.WindowThrough != request.Snapshot.CutoffDate || len(response.ClassifiedOperations) != len(request.Operations) || len(response.RecurringHypotheses) > 1000 {
		return forecasting.ErrModelResponse
	}
	for n, fact := range request.Operations {
		label := response.ClassifiedOperations[n]
		if label == nil {
			return forecasting.ErrModelResponse
		}
		validation := proto.CloneOf(label)
		if fact.CategorySource == "user" {
			if !proto.Equal(fact, label) {
				return forecasting.ErrModelResponse
			}
		} else {
			if !forecasting.ValidAnalysisLabel(label.CategoryCode, label.CategorySource, label.CategoryRuleId) || label.CategoryId != nil || label.CategoryCompanyId != nil {
				return forecasting.ErrModelResponse
			}
			validation.CategoryCode = fact.CategoryCode
			validation.CategorySource = fact.CategorySource
			validation.CategoryRuleId = fact.CategoryRuleId
		}
		if !proto.Equal(fact, validation) {
			return forecasting.ErrModelResponse
		}
	}
	if err := forecasting.ValidateAnalysis(input, decodeAnalysis(input, response)); err != nil {
		return err
	}
	// Strip only independently validated derived fields on a copied response;
	// existing v1 financial/evidence/overflow guards remain unchanged.
	baseInput := input
	baseInput.Snapshot.Model = baseAnalysisContext(input.Snapshot.Model)
	baseRequest := proto.CloneOf(request)
	baseRequest.Model = &pb.ModelContext{ModelName: "mean_90", ModelVersion: baseInput.Snapshot.Model.Version, PreprocessingVersion: baseInput.Snapshot.Model.PreprocessingVersion, ConfigurationId: baseInput.Snapshot.Model.Version}
	baseResponse := proto.CloneOf(response)
	baseResponse.Model = baseRequest.Model
	baseResponse.AnalysisContext = nil
	baseResponse.RecurringHypotheses = nil
	baseResponse.ClassifiedOperations = baseRequest.Operations
	return validateFinancialResponse(baseInput, baseRequest, baseResponse)
}

func decodeAnalysis(input forecasting.FinancialModelInput, r *pb.PredictResponse) *forecasting.Analysis {
	if r.AnalysisContext == nil {
		return nil
	}
	a := &forecasting.Analysis{Version: r.AnalysisContext.AnalysisVersion, From: domain.Date(r.AnalysisContext.WindowFrom), Through: domain.Date(r.AnalysisContext.WindowThrough)}
	for _, op := range r.ClassifiedOperations {
		id, _ := uuid.Parse(op.OperationId)
		category, _ := uuid.Parse(op.GetCategoryId())
		source := op.CategorySource
		if source == "user" {
			source = "manual"
		} else {
			direction := domain.Outflow
			if op.InflowMinor > 0 {
				direction = domain.Inflow
			}
			category = forecasting.AnalysisCategory(op.CategoryCode, direction, input.Categories)
		}
		a.Operations = append(a.Operations, forecasting.OperationAnalysis{ID: id, Version: int64(op.Version), CategoryCode: op.CategoryCode, CategorySource: source, RuleID: op.CategoryRuleId, CategoryID: category})
	}
	for _, h := range r.RecurringHypotheses {
		if h == nil {
			a.Hypotheses = append(a.Hypotheses, forecasting.RecurringHypothesis{})
			continue
		}
		direction := domain.Direction("")
		if h.Direction == pb.RecurringDirection_RECURRING_DIRECTION_OUTFLOW {
			direction = domain.Outflow
		}
		if h.Direction == pb.RecurringDirection_RECURRING_DIRECTION_INFLOW {
			direction = domain.Inflow
		}
		shift := map[pb.RecurringWeekendShift]string{pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NONE: "none", pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_PREVIOUS: "previous", pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NEXT: "next"}[h.WeekendShift]
		v := forecasting.RecurringHypothesis{ID: h.PatternId, RuleID: h.RuleId, RuleVersion: h.RuleVersion, CategoryCode: h.CategoryCode, Direction: direction, DayOfMonth: h.GetDayOfMonth(), MonthEnd: h.GetMonthEnd() != nil, WeekendShift: shift, AmountMinor: h.ExpectedAmountMinor, Confirmations: h.Confirmations, ExpectedPeriods: h.ExpectedPeriods, First: domain.Date(h.FirstEvidenceDate), Last: domain.Date(h.LastEvidenceDate), Status: ""}
		if h.Status == pb.RecurringHypothesisStatus_RECURRING_HYPOTHESIS_STATUS_HYPOTHESIS {
			v.Status = "hypothesis"
		}
		for _, ref := range h.HistoricalOperations {
			if ref == nil {
				v.Operations = append(v.Operations, forecasting.OperationReference{})
				continue
			}
			id, _ := uuid.Parse(ref.OperationId)
			v.Operations = append(v.Operations, forecasting.OperationReference{ID: id, Version: int64(ref.Version)})
		}
		a.Hypotheses = append(a.Hypotheses, v)
	}
	return a
}
