package forecasting

import (
	"math"
	"time"

	"github.com/google/uuid"

	"google.golang.org/protobuf/types/known/timestamppb"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func statusName(state string) pb.ForecastStatus {
	switch state {
	case "queued":
		return pb.ForecastStatus_FORECAST_STATUS_QUEUED
	case "running":
		return pb.ForecastStatus_FORECAST_STATUS_RUNNING
	case "succeeded":
		return pb.ForecastStatus_FORECAST_STATUS_SUCCEEDED
	case "failed":
		return pb.ForecastStatus_FORECAST_STATUS_FAILED
	default:
		return pb.ForecastStatus_FORECAST_STATUS_UNSPECIFIED
	}
}
func encodeSummary(view forecasting.ForecastView) *pb.ForecastSummary {
	cutoff, _ := time.Parse(time.DateOnly, string(view.Cutoff))
	return &pb.ForecastSummary{Id: view.ID.String(), Status: statusName(view.Status), CutoffDate: string(view.Cutoff), StartDate: cutoff.AddDate(0, 0, 1).Format(time.DateOnly), EndDate: cutoff.AddDate(0, 0, 30).Format(time.DateOnly), CreatedAt: timestamppb.New(view.CreatedAt), IsStale: view.CurrentDataVersion > view.DataVersion}
}
func kopecks(n int64) *pb.Money { return &pb.Money{AmountMinor: n, Currency: "RUB"} }
func probability() *pb.Probability {
	return &pb.Probability{Estimate: &pb.Probability_UnavailableReason{UnavailableReason: "not_calibrated"}}
}

func encodeForecast(view forecasting.ForecastView) *pb.Forecast {
	d := view.Snapshot.Data()
	cutoff, _ := time.Parse(time.DateOnly, string(d.Cutoff))
	coverage := view.Coverage
	kind := pb.HistoryCoverageKind_HISTORY_COVERAGE_KIND_UNSPECIFIED
	switch coverage.Kind {
	case "certified":
		kind = pb.HistoryCoverageKind_HISTORY_COVERAGE_KIND_CERTIFIED
	case "dataset_fixture_assumption":
		kind = pb.HistoryCoverageKind_HISTORY_COVERAGE_KIND_DATASET_FIXTURE_ASSUMPTION
	}
	historyCoverage := &pb.HistoryCoverage{Kind: kind, FromDate: string(coverage.From), ThroughDate: string(coverage.Through), Reference: coverage.Reference, DataVersion: coverage.DataVersion, Limitations: append([]string(nil), coverage.Limitations...)}
	input := &pb.ForecastInput{SnapshotId: d.ID.String(), DataVersion: d.DataVersion, CutoffDate: string(d.Cutoff), DataAsOf: string(d.Cutoff), StartDate: cutoff.AddDate(0, 0, 1).Format(time.DateOnly), EndDate: cutoff.AddDate(0, 0, 30).Format(time.DateOnly), HorizonDays: 30, Currency: "RUB", AccountId: d.Scope.AccountID.String(), OpeningBalance: kopecks(d.Opening.Observation.Amount().Minor()), BalanceAsOf: string(d.Cutoff), RiskDefinition: "below_zero_eod", ModelContext: &pb.ForecastModelContext{Id: view.ModelContextID.String(), ModelName: "mean_90", ModelVersion: "mean_90_components_v1", PreprocessingVersion: "calendar_replacement_v1", ConfigurationId: "mean_90_components_v1"}, HistoryCoverage: historyCoverage}
	input.CalendarMode = pb.ForecastCalendarMode_FORECAST_CALENDAR_MODE_AVAILABLE
	input.CalendarKnowledgeMode = pb.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE
	if d.Calendar.KnowledgeMode == "current" {
		input.CalendarKnowledgeMode = pb.ForecastCalendarKnowledgeMode_FORECAST_CALENDAR_KNOWLEDGE_MODE_CURRENT
		input.CalendarAsOfDate = string(d.Calendar.AsOf)
		input.NoPostCutoffMovements = d.Calendar.NoPostCutoffMovements
	}
	limitations := append(view.Snapshot.Limitations(), "history_reconstructed_from_covered_facts_and_cutoff_eod", "not_calibrated")
	limitations = append(limitations, coverage.Limitations...)
	out := &pb.Forecast{Id: view.ID.String(), CompanyId: view.Scope.CompanyID.String(), CreatedAt: timestamppb.New(view.CreatedAt), IsStale: view.CurrentDataVersion > d.DataVersion, CurrentDataVersion: view.CurrentDataVersion, Input: input, Readiness: &pb.ForecastReadiness{Status: "limited", HistoryDays: 90, ObservedDays: observedDays(d.Facts, coverage, cutoff), Limitations: limitations}}
	if a := view.Assessment; a != nil {
		input.HistoryCoverage = nil
		input.HistoryAssessment = &pb.HistoryAssessment{RuleVersion: a.RuleVersion, FromDate: string(a.From), ThroughDate: string(a.Through), DataVersion: a.DataVersion, OperationDays: uint32(a.OperationDays), Limitations: append([]string{}, a.Limitations...)}
		for _, b := range a.Balances {
			if b.Date >= a.From && b.Date <= a.Through {
				input.HistoryAssessment.BalanceDays++
			}
		}
		input.ModelContext.ModelName = view.Model.Name
		input.ModelContext.ModelVersion = view.Model.Version
		input.ModelContext.PreprocessingVersion = view.Model.PreprocessingVersion
		input.ModelContext.ConfigurationId = view.Model.Version
		out.Readiness.ObservedDays = uint32(a.OperationDays)
		out.Readiness.Limitations = append([]string{}, a.Limitations...)
	}

	switch view.Status {
	case "queued", "running":
		out.State = &pb.Forecast_Pending{Pending: &pb.ForecastPending{Status: statusName(view.Status)}}
	case "failed":
		out.State = &pb.Forecast_Failed{Failed: &pb.ForecastFailed{CompletedAt: timestamppb.New(*view.CompletedAt), Error: &pb.OperationError{Code: view.ErrorCode, Message: "Forecast calculation failed", RequestId: view.RequestID}}}
	case "succeeded":
		out.State = &pb.Forecast_Succeeded{Succeeded: &pb.ForecastSucceeded{CompletedAt: timestamppb.New(*view.CompletedAt), Result: encodeResult(view.Result)}}
	}
	if view.ScenarioID != nil {
		out.ScenarioId = new(view.ScenarioID.String())
	}
	if view.Status == "succeeded" {
		encodeKnownComponents(out.GetSucceeded().Result, view.Result, d.Calendar)
		encodeSales(out.GetSucceeded().Result, view)
		for _, expense := range view.ScenarioExpenses {
			out.GetSucceeded().Result.Components = append(out.GetSucceeded().Result.Components, &pb.ForecastComponent{Id: expense.ID.String(), OccurrenceId: expense.ID.String(), Kind: "scenario_expense", Direction: pb.Direction_DIRECTION_OUTFLOW, Date: string(expense.Date), Amount: kopecks(expense.AmountMinor), RemainingAmount: kopecks(expense.AmountMinor)})
		}
	}
	return out
}

func encodeKnownComponents(out *pb.ForecastResult, saved *forecasting.SavedForecast, calendar domain.Calendar) {
	for _, c := range saved.Components {
		if c.Kind != "known_remaining_payment" || len(c.SourceIDs) != 1 {
			continue
		}
		for _, payment := range calendar.Payments {
			d := payment.Data()
			if d.ID.String() != c.SourceIDs[0] {
				continue
			}
			direction := pb.Direction_DIRECTION_INFLOW
			if d.Direction == ledger.Outflow {
				direction = pb.Direction_DIRECTION_OUTFLOW
			}
			id := d.ID.String()
			source := &pb.RecordSource{Kind: pb.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL, SourceSystem: "manual_v1"}
			if d.Source.Kind == ledger.Imported {
				source.Kind = pb.RecordSourceKind_RECORD_SOURCE_KIND_FILE
				source.SourceSystem = d.Source.Namespace.System
				source.SourceCompanyId = new(d.Source.Namespace.CompanyID)
				source.ExternalPaymentId = new(d.Source.ExternalID)
			}
			component := &pb.ForecastComponent{Id: c.ID, OccurrenceId: c.ID, Kind: "known_payment", Direction: direction, Date: string(c.Date), Amount: kopecks(d.Amount.Minor()), RemainingAmount: kopecks(payment.Remaining().Minor()), PaymentId: &id, Source: source}
			if d.Category.ID != uuid.Nil {
				component.CategoryId = new(d.Category.ID.String())
			}
			if d.ReconciliationGroupID != uuid.Nil {
				component.RecurrenceGroupId = new(d.ReconciliationGroupID.String())
			}
			out.Components = append(out.Components, component)
			out.EffectiveOccurrences = append(out.EffectiveOccurrences, &pb.EffectiveOccurrence{EffectiveOccurrenceId: c.ID, ComponentId: c.ID, OccurrenceId: c.ID, PaymentId: &id, Direction: direction, Date: string(c.Date), RemainingAmount: kopecks(payment.Remaining().Minor()), InHorizon: true})
			out.Recommendations = append(out.Recommendations, &pb.Recommendation{Id: c.ID, Title: "Проверьте дату и остаток ожидаемого платежа", Explanation: "В прогнозе учтена только оставшаяся сумма ожидаемого платежа на указанную дату.", Evidence: []*pb.Evidence{{Type: "payment", ResourceId: id, Source: source}}})
		}
	}
}

func encodeResult(saved *forecasting.SavedForecast) *pb.ForecastResult {
	metrics := &pb.ForecastMetrics{MaxShortfall: kopecks(0), MinimumBalance: kopecks(math.MaxInt64), ClosingBalance: kopecks(saved.Days[29].Balance), GapProbability: probability()}
	result := &pb.ForecastResult{Analysis: encodeAnalysis(saved.Analysis), Metrics: metrics, DailyPoints: []*pb.ForecastPoint{}, Components: []*pb.ForecastComponent{}, EffectiveOccurrences: []*pb.EffectiveOccurrence{}, Recommendations: []*pb.Recommendation{}, Assumptions: append(append([]string{}, saved.Assumptions...), saved.Limitations...), ScenarioCapabilities: &pb.ScenarioCapabilities{OneOffExpense: true, UnavailableReason: new("Residual forecast components are not certified scenario targets")}}
	for _, day := range saved.Days {
		if day.Balance < metrics.MinimumBalance.AmountMinor {
			metrics.MinimumBalance = kopecks(day.Balance)
		}
		if day.Balance < 0 {
			// -MinInt64 cannot be represented by the nonnegative Money contract.
			shortfall := -day.Balance
			if metrics.FirstGap == nil {
				metrics.FirstGap = &pb.CashGap{Date: string(day.Date), Shortfall: kopecks(shortfall)}
			}
			if shortfall > metrics.MaxShortfall.AmountMinor {
				metrics.MaxShortfall = kopecks(shortfall)
			}
		}
		result.DailyPoints = append(result.DailyPoints, &pb.ForecastPoint{Date: string(day.Date), Inflow: kopecks(day.Inflow), Outflow: kopecks(day.Outflow), Balance: kopecks(day.Balance), NegativeBalanceProbability: probability()})
	}
	return result
}

// Observed days are distinct fact dates inside both the proof and the 90-day window.
func observedDays(facts []domain.Fact, coverage domain.JobCoverage, cutoff time.Time) uint32 {
	if coverage.Kind != "certified" && coverage.Kind != "dataset_fixture_assumption" {
		return 0
	}
	start := ledger.Date(cutoff.AddDate(0, 0, -89).Format(time.DateOnly))
	end := ledger.Date(cutoff.Format(time.DateOnly))
	seen := make(map[ledger.Date]struct{}, len(facts))
	for _, fact := range facts {
		date := fact.Transaction.Data().Date
		if date < coverage.From || date > coverage.Through || date < start || date > end {
			continue
		}
		seen[date] = struct{}{}
	}
	return uint32(len(seen))
}

func encodeSales(out *pb.ForecastResult, view forecasting.ForecastView) {
	for _, c := range view.Sales {
		item := &pb.ForecastComponent{Id: c.ID, OccurrenceId: c.ID, Kind: "ml_sales_receipt", Direction: pb.Direction_DIRECTION_INFLOW, Date: string(c.Date), Amount: kopecks(c.AmountMinor), RemainingAmount: kopecks(c.RemainingMinor), ScenarioEligible: view.ScenarioID == nil && c.RemainingMinor > 0}
		if c.CategoryID != uuid.Nil {
			item.CategoryId = new(c.CategoryID.String())
		}
		out.Components = append(out.Components, item)
	}
	if len(view.Sales) > 0 {
		out.ScenarioCapabilities.ReduceSales = view.ScenarioID == nil
		out.Assumptions = append(out.Assumptions, "classified_sales_mean_90_v1: модельная доля продаж по сохранённой классификации за 90 дней; займы, свои переводы и платежи календаря исключены")
	}
	out.ScenarioCapabilities.UnavailableReason = new("Для этого шаблона нет подходящих проверенных компонентов прогноза.")
}
