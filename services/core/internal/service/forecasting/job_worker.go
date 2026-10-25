package forecasting

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

// Run owns one bounded worker. No goroutine or model call survives its context;
// crashed/cancelled claims are recovered from their durable lease by a new run.
func (s *jobs) Run(ctx context.Context) error {
	if s.predict == nil {
		return ErrUnavailable
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		worked, err := s.ProcessOne(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *jobs) ProcessOne(ctx context.Context) (bool, error) {
	if s.predict == nil || s.withinTx == nil {
		return false, ErrUnavailable
	}
	token, err := uuid.NewV7()
	if err != nil {
		return false, ErrUnavailable
	}
	var job domain.Job
	var input FinancialModelInput
	var frozen jobInput
	var snapshot domain.FinancialSnapshot
	found := false
	err = s.withinTx(ctx, func(ctx context.Context, storage jobStore, snapshots jobSnapshots, _ jobCommands) error {
		var err error
		job, found, err = storage.ClaimJob(ctx, token)
		if err != nil || !found {
			return err
		}
		frozen, err = decodeJobInput(job)
		if err != nil {
			return err
		}
		snapshot, err = snapshots.ReadSnapshot(ctx, job.Scope, frozen.SnapshotID)
		if err != nil {
			return err
		}
		d := snapshot.Data()
		if d.Scope != job.Scope || d.DataVersion != job.DataVersion || d.Cutoff != frozen.Cutoff {
			return ErrUnavailable
		}
		input, err = prepareJob(snapshot, frozen)
		return err
	})
	if err != nil || !found {
		return false, err
	}
	// Deliberately outside the claim transaction and account locks.
	modelCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	result, modelErr := s.predict.PredictFinancial(modelCtx, input)
	cancel()
	if modelErr == nil && (result.Projection.SnapshotID != input.Snapshot.SnapshotID || result.Projection.Scope != input.Snapshot.Scope || result.Projection.DataVersion != input.Snapshot.DataVersion || result.Projection.Cutoff != input.Snapshot.Cutoff || result.Projection.Model != input.Snapshot.Model) {
		modelErr = ErrModelResponse
	}
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	job.Status = "failed"
	job.ErrorCode = "DEPENDENCY_UNAVAILABLE"
	job.RequestID = uuid.NewString()
	if modelErr == nil {
		job.Result, modelErr = encodeModelResult(snapshot, frozen, result)
	}
	if modelErr == nil {
		job.Status = "succeeded"
		job.ErrorCode = ""
		job.RequestID = ""
	}
	if modelErr != nil {
		job.Result = nil
		if errors.Is(modelErr, ErrModelInput) {
			job.ErrorCode = "INSUFFICIENT_HISTORY"
		}
		if errors.Is(modelErr, ErrModelContext) {
			job.ErrorCode = "UNSUPPORTED_MODEL_CONTEXT"
		}
	}
	err = s.withinTx(ctx, func(ctx context.Context, storage jobStore, _ jobSnapshots, _ jobCommands) error {
		return storage.FinishJob(ctx, job, token)
	})
	return true, err
}

func encodeModelResult(snapshot domain.FinancialSnapshot, frozen jobInput, result FinancialModelResult) ([]byte, error) {
	saved := saveForecast(result)
	if err := validateSavedForecast(frozen, snapshot, &saved); err != nil {
		return nil, ErrModelResponse
	}
	encoded, err := json.Marshal(saved)
	if err != nil || len(encoded) > 2<<20 {
		return nil, ErrModelResponse
	}
	return encoded, nil
}

func saveForecast(result FinancialModelResult) SavedForecast {
	saved := SavedForecast{SchemaVersion: 1, FirstNegativeDate: result.Projection.FirstNegativeDate, ProbabilityReason: result.ProbabilityUnavailableReason, Components: result.Components, Evidence: result.Evidence, Assumptions: result.Assumptions, Limitations: result.Limitations, Warnings: result.Projection.Warnings}
	for _, day := range result.Projection.Days {
		saved.Days = append(saved.Days, SavedForecastDay{Date: day.Date, Inflow: day.Inflow.Minor(), Outflow: day.Outflow.Minor(), Balance: day.ReferenceBalance.Minor()})
	}
	return saved
}

func validateSavedForecast(frozen jobInput, snapshot domain.FinancialSnapshot, result *SavedForecast) error {
	if frozen.Model.Version == CSVModelVersion && (result == nil || !slices.Equal(result.Limitations, csvLimitations()) || len(result.Components) != 0 || len(result.Evidence) != 0) {
		return ErrModelResponse
	}
	if frozen.Model.Version == CSVCalendarModelVersion && (result == nil || !activeCalendar(snapshot.Data().Calendar) || !slices.Equal(result.Limitations, csvCalendarLimitations())) {
		return ErrModelResponse
	}
	if result == nil || result.SchemaVersion != 1 || len(result.Days) != 30 || result.ProbabilityReason != "not_calibrated" || (!activeCalendar(snapshot.Data().Calendar) && !slices.Contains(result.Limitations, "historical_calendar_unavailable")) {
		return ErrModelResponse
	}
	cutoff, _ := time.Parse(time.DateOnly, string(frozen.Cutoff))
	balance := snapshot.Data().Opening.Observation.Amount()
	first := ""
	for n, day := range result.Days {
		if string(day.Date) != cutoff.AddDate(0, 0, n+1).Format(time.DateOnly) || day.Inflow < 0 || day.Outflow < 0 || day.Balance == math.MinInt64 {
			return ErrModelResponse
		}
		net, _ := money.New(day.Inflow-day.Outflow, money.RUB)
		next, err := balance.Add(net)
		if err != nil || next.Minor() != day.Balance {
			return ErrModelResponse
		}
		balance = next
		if first == "" && day.Balance < 0 {
			first = string(day.Date)
		}
	}
	if (first == "" && result.FirstNegativeDate != nil) || (first != "" && (result.FirstNegativeDate == nil || string(*result.FirstNegativeDate) != first)) {
		return ErrModelResponse
	}
	return nil
}
