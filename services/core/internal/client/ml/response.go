package ml

import (
	"slices"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func validateResponse(input forecasting.ModelInput, response *pb.PredictResponse) error {
	if response == nil || response.Model == nil || response.SnapshotId != input.SnapshotID.String() || response.CompanyId != input.Scope.CompanyID.String() ||
		response.AccountId != input.Scope.AccountID.String() || response.DataVersion != uint64(input.DataVersion) || response.Currency != money.RUB ||
		response.CutoffDate != string(input.Cutoff) || response.Model.ModelName != input.Model.Name || response.Model.ModelVersion != input.Model.Version ||
		response.Model.PreprocessingVersion != input.Model.PreprocessingVersion || len(response.Days) != input.HorizonDays || !response.HistoryOnlyIncludesRecurringFlows ||
		!slices.Contains(response.Warnings, "HISTORY_ONLY_NO_CALENDAR") || !slices.Contains(response.Warnings, "UNCALIBRATED_NO_PROBABILITY") {
		return forecasting.ErrModelResponse
	}
	if hasReconciliation(input.History) && !slices.Contains(response.Warnings, "OBSERVED_RECONCILIATION_WITHIN_ONE_MINOR_UNIT") {
		return forecasting.ErrModelResponse
	}
	cutoff, _ := time.Parse(time.DateOnly, string(input.Cutoff))
	if response.StartDate != cutoff.AddDate(0, 0, 1).Format(time.DateOnly) {
		return forecasting.ErrModelResponse
	}
	balance := input.OpeningBalance
	firstNegative := ""
	for i, day := range response.Days {
		if day == nil || day.Date != cutoff.AddDate(0, 0, i+1).Format(time.DateOnly) || day.InflowMinor < 0 || day.OutflowMinor < 0 {
			return forecasting.ErrModelResponse
		}
		next, err := balanceAfter(balance, day.InflowMinor, day.OutflowMinor)
		if err != nil || day.ReferenceBalanceMinor != next.Minor() {
			return forecasting.ErrModelResponse
		}
		balance = next
		if firstNegative == "" && balance.Minor() < 0 {
			firstNegative = day.Date
		}
	}
	if response.GetFirstNegativeBalanceDate() != firstNegative || (response.FirstNegativeBalanceDate != nil && firstNegative == "") {
		return forecasting.ErrModelResponse
	}
	return nil
}

func decodeResponse(input forecasting.ModelInput, response *pb.PredictResponse) forecasting.ModelResult {
	result := forecasting.ModelResult{SnapshotID: input.SnapshotID, Scope: input.Scope, DataVersion: input.DataVersion, Cutoff: input.Cutoff, Model: input.Model,
		Days: make([]forecasting.ModelForecastDay, 0, len(response.Days)), HistoryOnlyIncludesRecurringFlows: true, Warnings: slices.Clone(response.Warnings)}
	for _, day := range response.Days {
		inflow, _ := money.New(day.InflowMinor, money.RUB)
		outflow, _ := money.New(day.OutflowMinor, money.RUB)
		balance, _ := money.New(day.ReferenceBalanceMinor, money.RUB)
		result.Days = append(result.Days, forecasting.ModelForecastDay{Date: ledger.Date(day.Date), Inflow: inflow, Outflow: outflow, ReferenceBalance: balance})
	}
	if response.FirstNegativeBalanceDate != nil {
		date := ledger.Date(*response.FirstNegativeBalanceDate)
		result.FirstNegativeDate = &date
	}
	return result
}

func hasReconciliation(history []forecasting.ModelDay) bool {
	for i := 1; i < len(history); i++ {
		expected, err := balanceAfter(history[i-1].ClosingBalance, history[i].Inflow.Minor(), history[i].Outflow.Minor())
		if err != nil || expected != history[i].ClosingBalance {
			return true
		}
	}
	return false
}
