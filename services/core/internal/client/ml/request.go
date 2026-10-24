package ml

import (
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/ml/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func validateInput(input forecasting.ModelInput) error {
	if input.SnapshotID == uuid.Nil || input.Scope.Validate() != nil || input.DataVersion < 1 || input.Cutoff.Validate() != nil || input.OpeningBalance.Validate() != nil ||
		input.HorizonDays < 1 || input.HorizonDays > 30 || len(input.History) != 90 || input.Model.Name == "" || input.Model.Version == "" || input.Model.PreprocessingVersion == "" {
		return forecasting.ErrModelInput
	}
	if len(input.KnownPayments) > 0 {
		return forecasting.ErrModelContext
	}
	cutoff, _ := time.Parse(time.DateOnly, string(input.Cutoff))
	if cutoff.AddDate(0, 0, -89).Year() < 1 || cutoff.AddDate(0, 0, input.HorizonDays).Year() > 9999 {
		return forecasting.ErrModelInput
	}
	for i, day := range input.History {
		if err := validateHistoryDay(day, cutoff.AddDate(0, 0, i-89)); err != nil {
			return err
		}
		if i > 0 {
			if !historyBalanceMatches(input.History[i-1].ClosingBalance.Minor(), day) {
				return forecasting.ErrModelInput
			}
		}
	}
	last := input.History[len(input.History)-1]
	if last.Source != forecasting.Observed || last.ClosingBalance != input.OpeningBalance {
		return forecasting.ErrModelInput
	}
	return nil
}

func validateHistoryDay(day forecasting.ModelDay, date time.Time) error {
	if string(day.Date) != date.Format(time.DateOnly) || day.Inflow.Nonnegative() != nil || day.Outflow.Nonnegative() != nil || day.ClosingBalance.Validate() != nil {
		return forecasting.ErrModelInput
	}
	if day.Source != forecasting.Observed && day.Source != forecasting.VerifiedNoOperations {
		return forecasting.ErrModelInput
	}
	if day.Source == forecasting.VerifiedNoOperations && (day.Inflow.Minor() != 0 || day.Outflow.Minor() != 0) {
		return forecasting.ErrModelInput
	}
	return nil
}

// Python validates historical reconciliation with arbitrary-precision integers.
// A one-kopeck difference remains valid even at an int64 boundary.
func historyBalanceMatches(previous int64, day forecasting.ModelDay) bool {
	delta := big.NewInt(previous)
	delta.Add(delta, big.NewInt(day.Inflow.Minor()))
	delta.Sub(delta, big.NewInt(day.Outflow.Minor()))
	delta.Sub(delta, big.NewInt(day.ClosingBalance.Minor()))
	if day.Source == forecasting.Observed {
		return delta.Abs(delta).Cmp(big.NewInt(1)) <= 0
	}
	return delta.Sign() == 0
}

// Nonnegative flows make their difference fit int64, even when applying inflow
// first would overflow and the final balance is representable.
func balanceAfter(previous money.Money, inflow, outflow int64) (money.Money, error) {
	net, err := money.New(inflow-outflow, money.RUB)
	if err != nil {
		return money.Money{}, err
	}
	return previous.Add(net)
}

func encodeInput(input forecasting.ModelInput) *pb.PredictRequest {
	opening := input.OpeningBalance.Minor()
	snapshot := &pb.ForecastSnapshot{SnapshotId: input.SnapshotID.String(), CompanyId: input.Scope.CompanyID.String(), AccountId: input.Scope.AccountID.String(),
		DataVersion: uint64(input.DataVersion), Currency: money.RUB, CutoffDate: string(input.Cutoff), InitialBalanceMinor: &opening, History: make([]*pb.HistoricalDay, 0, len(input.History))}
	for _, day := range input.History {
		inflow, outflow, balance := day.Inflow.Minor(), day.Outflow.Minor(), day.ClosingBalance.Minor()
		source := pb.DaySource_DAY_SOURCE_OBSERVED
		if day.Source == forecasting.VerifiedNoOperations {
			source = pb.DaySource_DAY_SOURCE_VERIFIED_NO_OPERATIONS
		}
		snapshot.History = append(snapshot.History, &pb.HistoricalDay{Date: string(day.Date), InflowMinor: &inflow, OutflowMinor: &outflow, ClosingBalanceMinor: &balance, Source: source})
	}
	return &pb.PredictRequest{Snapshot: snapshot, HorizonDays: uint32(input.HorizonDays), Model: &pb.ModelContext{ModelName: input.Model.Name, ModelVersion: input.Model.Version, PreprocessingVersion: input.Model.PreprocessingVersion}}
}
