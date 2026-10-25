package forecasting

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func validForecastKey(key string) bool {
	if len(key) < 8 || len(key) > 128 {
		return false
	}
	for _, c := range []byte(key) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == ':' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func validateForecastCreate(cmd CreateForecastCommand) error {
	if cmd.Scope.Validate() != nil || strings.TrimSpace(cmd.ActorID) == "" || !validForecastKey(cmd.IdempotencyKey) || cmd.Cutoff.Validate() != nil || cmd.Today.Validate() != nil || cmd.Cutoff > cmd.Today {
		return ErrInvalid
	}
	if cmd.HorizonDays != 30 {
		return ErrHorizon
	}
	cutoff, _ := time.Parse(time.DateOnly, string(cmd.Cutoff))
	if cutoff.AddDate(0, 0, -89).Year() < 1 || cutoff.AddDate(0, 0, 30).Year() > 9999 {
		return ErrInvalid
	}
	return nil
}

func (s *jobs) Create(ctx context.Context, cmd CreateForecastCommand) (ForecastAccepted, error) {
	if err := validateForecastCreate(cmd); err != nil {
		return ForecastAccepted{}, err
	}
	if s.withinTx == nil || s.predict == nil {
		return ForecastAccepted{}, ErrUnavailable
	}
	body, err := json.Marshal(struct {
		Scope   ledger.Scope
		Cutoff  ledger.Date
		Horizon int
	}{cmd.Scope, cmd.Cutoff, cmd.HorizonDays})
	if err != nil {
		return ForecastAccepted{}, ErrUnavailable
	}
	digest := sha256.Sum256(body)
	var accepted ForecastAccepted
	err = s.withinTx(ctx, func(ctx context.Context, storage jobStore, snapshots jobSnapshots, commands jobCommands) error {
		payload, replay, err := commands.Reserve(ctx, cmd.Scope, cmd.ActorID, forecastJobOperation, "", cmd.IdempotencyKey, digest[:])
		if err != nil {
			return err
		}
		if replay {
			if json.Unmarshal(payload, &accepted) != nil || accepted.ID == uuid.Nil || accepted.PollAfterMS != 1000 {
				return ErrUnavailable
			}
			job, err := storage.ReadJob(ctx, cmd.Scope, accepted.ID)
			if err != nil {
				return err
			}
			input, err := decodeJobInput(job)
			if err != nil || input.Cutoff != cmd.Cutoff {
				return ErrUnavailable
			}
			return nil
		}
		revision, err := storage.LockDataVersion(ctx, cmd.Scope)
		if err != nil {
			return err
		}
		opening, found, err := snapshots.ReadOpeningBalance(ctx, cmd.Scope, cmd.Cutoff)
		if err != nil {
			return err
		}
		if !found {
			return ErrMissingBalance
		}
		cutoff, _ := time.Parse(time.DateOnly, string(cmd.Cutoff))
		from := ledger.Date(cutoff.AddDate(0, 0, -89).Format(time.DateOnly))
		coverage, found, err := storage.ReadCoverage(ctx, cmd.Scope, revision, from, cmd.Cutoff)
		if err != nil {
			return err
		}
		hasCoverage := found
		facts, err := snapshots.ReadFacts(ctx, cmd.Scope, cmd.Cutoff)
		if err != nil {
			return err
		}
		calendar, err := snapshots.ReadCalendar(ctx, cmd.Scope)
		if err != nil {
			return err
		}
		if hasCoverage || len(calendar.Payments) > 0 || len(calendar.Groups) > 0 {
			calendar, err = activateCalendar(calendar, cmd.Cutoff, cmd.Today, facts)
		}
		if err != nil {
			return err
		}
		id, err := uuid.NewV7()
		if err != nil {
			return ErrUnavailable
		}
		snapshot, err := domain.NewFinancialSnapshot(domain.SnapshotData{ID: id, Scope: cmd.Scope, DataVersion: revision, Cutoff: cmd.Cutoff, CreatedAt: time.Now().UTC().Truncate(time.Microsecond), Opening: opening, Facts: facts, Calendar: calendar})
		if err != nil {
			return ErrHistory
		}
		input, err := frozenInput(snapshot, coverage)
		if err != nil {
			return err
		}
		if !hasCoverage {
			observations, err := snapshots.ReadBalanceHistory(ctx, cmd.Scope, cmd.Cutoff)
			if err != nil {
				return err
			}
			assessment, err := assessCSVHistory(snapshot, observations)
			if err != nil {
				return err
			}
			input.SchemaVersion = 2
			input.Coverage = domain.JobCoverage{}
			input.Assessment = assessment
			input.Categories, err = snapshots.ReadCategories(ctx, cmd.Scope)
			if err != nil {
				return err
			}
			input.Model = ModelContext{Name: "mean_90", Version: CSVAnalysisVersion, PreprocessingVersion: CSVAnalysisPreprocessing}
			if activeCalendar(calendar) {
				input.Model = ModelContext{Name: "mean_90", Version: CSVCalendarAnalysisVersion, PreprocessingVersion: CSVCalendarAnalysisPreprocessing}
			}
		}
		if _, err := prepareJob(snapshot, input); err != nil {
			return err
		}
		if err := snapshots.InsertSnapshot(ctx, snapshot); err != nil {
			return err
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			return ErrUnavailable
		}
		jobID, err := uuid.NewV7()
		if err != nil {
			return ErrUnavailable
		}
		job := domain.Job{ID: jobID, Scope: cmd.Scope, DataVersion: revision, Status: "queued", CreatedAt: snapshot.Data().CreatedAt, Input: encoded}
		if err := storage.InsertJob(ctx, job); err != nil {
			return err
		}
		accepted = ForecastAccepted{ID: jobID, PollAfterMS: 1000}
		receipt, err := json.Marshal(accepted)
		if err != nil {
			return ErrUnavailable
		}
		return commands.Complete(ctx, cmd.Scope, cmd.ActorID, forecastJobOperation, "", cmd.IdempotencyKey, receipt)
	})
	if err != nil {
		return ForecastAccepted{}, err
	}
	return accepted, nil
}
