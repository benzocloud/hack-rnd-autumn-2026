package ml

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
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
	operations := map[string]*pb.FinancialOperation{}
	derived := map[string]*pb.FinancialOperation{}
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
		operations[fact.OperationId] = fact
		derived[fact.OperationId] = label
	}
	refs := map[string]bool{}
	patterns := map[string]bool{}
	total := 0
	for _, h := range response.RecurringHypotheses {
		if h == nil || h.RuleId != "monthly_fit" || h.RuleVersion != forecasting.AnalysisVersion || h.Status != pb.RecurringHypothesisStatus_RECURRING_HYPOTHESIS_STATUS_HYPOTHESIS || patterns[h.PatternId] || h.ExpectedAmountMinor <= 0 || len(h.HistoricalOperations) == 0 {
			return forecasting.ErrModelResponse
		}
		patterns[h.PatternId] = true
		ids := []string{}
		var party string
		for _, ref := range h.HistoricalOperations {
			if ref == nil {
				return forecasting.ErrModelResponse
			}
			fact := operations[ref.OperationId]
			label := derived[ref.OperationId]
			if fact == nil || label == nil || refs[ref.OperationId] || fact.Version != ref.Version || label.CategorySource != "rules" || label.CategoryCode != h.CategoryCode || (fact.InflowMinor > 0) != (h.Direction == pb.RecurringDirection_RECURRING_DIRECTION_INFLOW) || (h.Direction != pb.RecurringDirection_RECURRING_DIRECTION_INFLOW && h.Direction != pb.RecurringDirection_RECURRING_DIRECTION_OUTFLOW) {
				return forecasting.ErrModelResponse
			}
			key := fact.CounterpartyInn
			// Match the pure helper normalization for name-only party identity.
			if key == "" {
				key = strings.Join(strings.Fields(strings.ReplaceAll(strings.ToLower(norm.NFKC.String(fact.CounterpartyName)), "ё", "е")), " ")
			}
			if key == "" || party != "" && party != key {
				return forecasting.ErrModelResponse
			}
			party = key
			refs[ref.OperationId] = true
			ids = append(ids, ref.OperationId)
			total++
		}
		if total > 20000 || !slices.IsSorted(ids) {
			return forecasting.ErrModelResponse
		}
		identity := strings.Join(append([]string{request.Snapshot.SnapshotId, "monthly_fit", forecasting.AnalysisVersion}, ids...), "\n")
		if h.PatternId != fmt.Sprintf("monthly_%x", sha256.Sum256([]byte(identity))) || !validMonthlyPattern(h, operations, request.Snapshot.CutoffDate) {
			return forecasting.ErrModelResponse
		}
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
		direction := domain.Outflow
		if h.Direction == pb.RecurringDirection_RECURRING_DIRECTION_INFLOW {
			direction = domain.Inflow
		}
		shift := map[pb.RecurringWeekendShift]string{pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NONE: "none", pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_PREVIOUS: "previous", pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NEXT: "next"}[h.WeekendShift]
		v := forecasting.RecurringHypothesis{ID: h.PatternId, RuleID: h.RuleId, RuleVersion: h.RuleVersion, CategoryCode: h.CategoryCode, Direction: direction, DayOfMonth: h.GetDayOfMonth(), MonthEnd: h.GetMonthEnd() != nil, WeekendShift: shift, AmountMinor: h.ExpectedAmountMinor, Confirmations: h.Confirmations, ExpectedPeriods: h.ExpectedPeriods, First: domain.Date(h.FirstEvidenceDate), Last: domain.Date(h.LastEvidenceDate), Status: "hypothesis"}
		for _, ref := range h.HistoricalOperations {
			id, _ := uuid.Parse(ref.OperationId)
			v.Operations = append(v.Operations, forecasting.OperationReference{ID: id, Version: int64(ref.Version)})
		}
		a.Hypotheses = append(a.Hypotheses, v)
	}
	return a
}

func validMonthlyPattern(h *pb.RecurringHypothesis, facts map[string]*pb.FinancialOperation, cutoffString string) bool {
	cutoff, err := time.Parse(time.DateOnly, cutoffString)
	if err != nil {
		return false
	}
	if h.GetMonthEnd() == nil && (h.GetDayOfMonth() < 1 || h.GetDayOfMonth() > 31) {
		return false
	}
	shift := h.WeekendShift
	if shift < pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NONE || shift > pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NEXT {
		return false
	}
	minimum := uint32(4)
	if h.Direction == pb.RecurringDirection_RECURRING_DIRECTION_OUTFLOW && slices.Contains([]string{"salary", "salary_advance", "salary_final", "tax", "rent", "equipment_rent", "bank_fee", "utilities", "subscription", "loan_payment", "overdraft_interest"}, h.CategoryCode) {
		minimum = 3
	}
	totals := map[string]int64{}
	for _, ref := range h.HistoricalOperations {
		op := facts[ref.OperationId]
		amount := op.InflowMinor + op.OutflowMinor
		if totals[op.Date] > int64(^uint64(0)>>1)-amount {
			return false
		}
		totals[op.Date] += amount
	}
	dates := make([]string, 0, len(totals))
	for d := range totals {
		dates = append(dates, d)
	}
	slices.Sort(dates)
	if dates[0] != h.FirstEvidenceDate || dates[len(dates)-1] != h.LastEvidenceDate {
		return false
	}
	first, _ := time.Parse(time.DateOnly, dates[0])
	last, _ := time.Parse(time.DateOnly, dates[len(dates)-1])
	if cutoff.Sub(last) > 45*24*time.Hour {
		return false
	}
	start := first.AddDate(0, 0, -4)
	end := cutoff.AddDate(0, 0, 4)
	used := map[string]bool{}
	amounts := []int64{}
	confirmed := uint32(0)
	expected := uint32(0)
	for month := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC); !month.After(end); month = month.AddDate(0, 1, 0) {
		lastDay := month.AddDate(0, 1, -1).Day()
		anchor := int(h.GetDayOfMonth())
		if h.GetMonthEnd() != nil {
			anchor = lastDay
		}
		due := month.AddDate(0, 0, min(anchor, lastDay)-1)
		for due.Weekday() == time.Saturday || due.Weekday() == time.Sunday {
			if shift == pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NONE {
				break
			}
			delta := 1
			if shift == pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_PREVIOUS {
				delta = -1
			}
			due = due.AddDate(0, 0, delta)
		}
		if due.Before(start) || due.After(end) {
			continue
		}
		if !due.After(cutoff) {
			expected++
		}
		best := ""
		distance := 5
		for _, date := range dates {
			if used[date] {
				continue
			}
			day, _ := time.Parse(time.DateOnly, date)
			d := int(day.Sub(due) / (24 * time.Hour))
			if d < 0 {
				d = -d
			}
			if d < distance {
				best = date
				distance = d
			}
		}
		if best != "" {
			used[best] = true
			amounts = append(amounts, totals[best])
			if !due.After(cutoff) {
				confirmed++
			}
		}
	}
	if confirmed < minimum || confirmed != h.Confirmations || expected != h.ExpectedPeriods || confirmed*4 < expected*3 || len(used) != len(dates) {
		return false
	}
	// The helper uses the median of the last three matched monthly amounts.
	if len(amounts) > 3 {
		amounts = amounts[len(amounts)-3:]
	}
	slices.Sort(amounts)
	median := amounts[len(amounts)/2]
	if len(amounts)%2 == 0 {
		median = amounts[0]/2 + amounts[1]/2 + (amounts[0]%2+amounts[1]%2)/2
	}
	return median == h.ExpectedAmountMinor
}
