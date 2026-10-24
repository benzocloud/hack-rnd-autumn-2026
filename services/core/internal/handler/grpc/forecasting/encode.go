package forecasting

import (
	"math"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

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
	input := &pb.ForecastInput{SnapshotId: d.ID.String(), DataVersion: d.DataVersion, CutoffDate: string(d.Cutoff), DataAsOf: string(d.Cutoff), StartDate: cutoff.AddDate(0, 0, 1).Format(time.DateOnly), EndDate: cutoff.AddDate(0, 0, 30).Format(time.DateOnly), HorizonDays: 30, Currency: "RUB", AccountId: d.Scope.AccountID.String(), OpeningBalance: kopecks(d.Opening.Observation.Amount().Minor()), BalanceAsOf: string(d.Cutoff), RiskDefinition: "below_zero_eod", ModelContext: &pb.ForecastModelContext{Id: view.ModelContextID.String(), ModelName: "mean_90", ModelVersion: "mean_90_components_v1", PreprocessingVersion: "calendar_replacement_v1", ConfigurationId: "mean_90_components_v1"}}
	out := &pb.Forecast{Id: view.ID.String(), CompanyId: view.Scope.CompanyID.String(), CreatedAt: timestamppb.New(view.CreatedAt), IsStale: view.CurrentDataVersion > d.DataVersion, CurrentDataVersion: view.CurrentDataVersion, Input: input, Readiness: &pb.ForecastReadiness{Status: "limited", HistoryDays: 90, ObservedDays: 1, Limitations: []string{"historical_calendar_unavailable", "historical_availability_unverified", "history_reconstructed_from_certified_facts_and_cutoff_eod", "not_calibrated"}}}
	switch view.Status {
	case "queued", "running":
		out.State = &pb.Forecast_Pending{Pending: &pb.ForecastPending{Status: statusName(view.Status)}}
	case "failed":
		out.State = &pb.Forecast_Failed{Failed: &pb.ForecastFailed{CompletedAt: timestamppb.New(*view.CompletedAt), Error: &pb.OperationError{Code: view.ErrorCode, Message: "Forecast calculation failed", RequestId: view.RequestID}}}
	case "succeeded":
		out.State = &pb.Forecast_Succeeded{Succeeded: &pb.ForecastSucceeded{CompletedAt: timestamppb.New(*view.CompletedAt), Result: encodeResult(view.Result)}}
	}
	return out
}

func encodeResult(saved *forecasting.SavedForecast) *pb.ForecastResult {
	metrics := &pb.ForecastMetrics{MaxShortfall: kopecks(0), MinimumBalance: kopecks(math.MaxInt64), ClosingBalance: kopecks(saved.Days[29].Balance), GapProbability: probability()}
	result := &pb.ForecastResult{Metrics: metrics, DailyPoints: []*pb.ForecastPoint{}, Components: []*pb.ForecastComponent{}, EffectiveOccurrences: []*pb.EffectiveOccurrence{}, Recommendations: []*pb.Recommendation{}, Assumptions: append(append([]string{}, saved.Assumptions...), saved.Limitations...), ScenarioCapabilities: &pb.ScenarioCapabilities{UnavailableReason: new("Residual forecast components are not certified scenario targets")}}
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
