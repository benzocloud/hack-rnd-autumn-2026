package forecasting

import (
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func encodeAnalysis(a *forecasting.Analysis) *pb.ForecastAnalysis {
	if a == nil {
		return nil
	}
	result := &pb.ForecastAnalysis{AnalysisVersion: a.Version, WindowFrom: string(a.From), WindowThrough: string(a.Through)}
	for _, op := range a.Operations {
		row := &pb.ClassifiedOperationAnalysis{TransactionId: op.ID.String(), Version: op.Version, CategoryCode: op.CategoryCode, CategorySource: op.CategorySource, CategoryRuleId: op.RuleID}
		if op.CategoryID != uuid.Nil {
			row.CategoryId = new(op.CategoryID.String())
		}
		result.ClassifiedOperations = append(result.ClassifiedOperations, row)
	}
	for _, h := range a.Hypotheses {
		direction := pb.Direction_DIRECTION_OUTFLOW
		if h.Direction == ledger.Inflow {
			direction = pb.Direction_DIRECTION_INFLOW
		}
		row := &pb.RecurringHypothesis{PatternId: h.ID, RuleId: h.RuleID, RuleVersion: h.RuleVersion, CategoryCode: h.CategoryCode, Direction: direction, ExpectedAmountMinor: h.AmountMinor, Confirmations: h.Confirmations, ExpectedPeriods: h.ExpectedPeriods, FirstEvidenceDate: string(h.First), LastEvidenceDate: string(h.Last), Status: pb.RecurringHypothesisStatus_RECURRING_HYPOTHESIS_STATUS_HYPOTHESIS}
		row.WeekendShift = map[string]pb.RecurringWeekendShift{"none": pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NONE, "previous": pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_PREVIOUS, "next": pb.RecurringWeekendShift_RECURRING_WEEKEND_SHIFT_NEXT}[h.WeekendShift]
		if h.MonthEnd {
			row.MonthlyAnchor = &pb.RecurringHypothesis_MonthEnd{MonthEnd: &emptypb.Empty{}}
		} else {
			row.MonthlyAnchor = &pb.RecurringHypothesis_DayOfMonth{DayOfMonth: h.DayOfMonth}
		}
		for _, ref := range h.Operations {
			row.HistoricalOperations = append(row.HistoricalOperations, &pb.HistoricalOperationRef{TransactionId: ref.ID.String(), Version: ref.Version})
		}
		result.RecurringHypotheses = append(result.RecurringHypotheses, row)
	}
	return result
}
