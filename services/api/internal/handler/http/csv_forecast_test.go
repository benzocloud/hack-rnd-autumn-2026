package httpapi

import (
	"testing"
	"time"

	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func TestCSVForecastRequiresExplicitLimitedAssessment(t *testing.T) {
	f := newFinancialFixture()
	succeeded(f)
	i := f.forecast.Input
	cutoff, _ := time.Parse(time.DateOnly, i.CutoffDate)
	i.HistoryCoverage = nil
	i.ModelContext.ModelName = "mean_90"
	i.ModelContext.ModelVersion = "mean_90_csv_v1"
	i.ModelContext.PreprocessingVersion = "csv_observed_operations_v1"
	i.ModelContext.ConfigurationId = "mean_90_csv_v1"
	i.ModelContext.RandomSeed = nil
	i.ModelContext.TrajectoryCount = nil
	i.HistoryAssessment = &pb.HistoryAssessment{RuleVersion: "csv_history_quality_v1", FromDate: cutoff.AddDate(0, 0, -89).Format(time.DateOnly), ThroughDate: i.CutoffDate, DataVersion: i.DataVersion, OperationDays: 2, BalanceDays: 2, Limitations: []string{"csv_history_completeness_unverified", "unobserved_days_unknown", "observed_operations_mean_per_calendar_day", "historical_calendar_unavailable", "historical_availability_unverified", "not_calibrated"}}
	f.forecast.Readiness.Status = "limited"
	f.forecast.Readiness.HistoryDays = 90
	f.forecast.Readiness.ObservedDays = 2
	f.forecast.Readiness.Limitations = append([]string{}, i.HistoryAssessment.Limitations...)
	if !validForecast(f.forecast, f.account, forecastID) {
		t.Fatal("valid CSV projection rejected")
	}
	i.HistoryAssessment.DataVersion++
	if validForecast(f.forecast, f.account, forecastID) {
		t.Fatal("stale quality assessment accepted")
	}
	i.HistoryAssessment.DataVersion--
	f.forecast.Readiness.Status = "ready"
	if validForecast(f.forecast, f.account, forecastID) {
		t.Fatal("CSV forecast advertised as fully ready")
	}
	f.forecast.Readiness.Status = "limited"
	i.HistoryAssessment = nil
	if validForecast(f.forecast, f.account, forecastID) {
		t.Fatal("CSV model without assessment accepted")
	}
}
