package forecasting

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

type forecastCursor struct {
	Version          int
	Scope            ledger.Scope
	Actor            string
	IncludeScenarios bool
	Before           time.Time
	ID               uuid.UUID
}

func (s *jobs) Read(ctx context.Context, scope ledger.Scope, id uuid.UUID) (ForecastView, error) {
	if scope.Validate() != nil || id == uuid.Nil {
		return ForecastView{}, ErrInvalid
	}
	var view ForecastView
	err := s.withinTx(ctx, func(ctx context.Context, storage jobStore, snapshots jobSnapshots, _ jobCommands) error {
		job, err := storage.ReadJob(ctx, scope, id)
		if err != nil {
			return err
		}
		revision, err := storage.LockDataVersion(ctx, scope)
		if err != nil {
			return err
		}
		view, err = readForecastView(ctx, snapshots, job, revision)
		return err
	})
	if err != nil {
		return ForecastView{}, err
	}
	return view, nil
}

func readForecastView(ctx context.Context, snapshots jobSnapshots, job domain.Job, revision int64) (ForecastView, error) {
	input, err := decodeJobInput(job)
	if err != nil {
		return ForecastView{}, err
	}
	if job.ID == uuid.Nil || job.CreatedAt.IsZero() || job.CreatedAt.Year() < 1 || job.CreatedAt.Year() > 9999 || revision < job.DataVersion {
		return ForecastView{}, ErrUnavailable
	}
	snapshot, err := snapshots.ReadSnapshot(ctx, job.Scope, input.SnapshotID)
	if err != nil {
		return ForecastView{}, err
	}
	d := snapshot.Data()
	if d.Scope != job.Scope || d.ID != input.SnapshotID || d.DataVersion != job.DataVersion || d.Cutoff != input.Cutoff {
		return ForecastView{}, ErrUnavailable
	}
	if _, err := prepareJob(snapshot, input); err != nil {
		return ForecastView{}, ErrUnavailable
	}
	view := ForecastView{ID: job.ID, Scope: job.Scope, Status: job.Status, CreatedAt: job.CreatedAt, CompletedAt: job.CompletedAt, CurrentDataVersion: revision, DataVersion: job.DataVersion, Cutoff: input.Cutoff, Snapshot: snapshot, ModelContextID: input.ModelContextID, Coverage: input.Coverage, Assessment: input.Assessment, Model: input.Model, ErrorCode: job.ErrorCode, RequestID: job.RequestID}
	switch job.Status {
	case "queued", "running":
		if job.CompletedAt != nil || len(job.Result) != 0 || job.ErrorCode != "" {
			return ForecastView{}, ErrUnavailable
		}
	case "succeeded":
		if job.CompletedAt == nil || job.CompletedAt.Before(job.CreatedAt) || len(job.Result) == 0 || len(job.Result) > 2<<20 || job.ErrorCode != "" {
			return ForecastView{}, ErrUnavailable
		}
		var saved SavedForecast
		if json.Unmarshal(job.Result, &saved) != nil || validateSavedForecast(input, snapshot, &saved) != nil {
			return ForecastView{}, ErrUnavailable
		}
		view.Result = &saved
	case "failed":
		if job.CompletedAt == nil || job.CompletedAt.Before(job.CreatedAt) || job.ErrorCode == "" || job.RequestID == "" || len(job.Result) != 0 {
			return ForecastView{}, ErrUnavailable
		}
	default:
		return ForecastView{}, ErrUnavailable
	}
	return view, nil
}

func decodeForecastCursor(cmd ListForecastCommand) (forecastCursor, error) {
	c := forecastCursor{Version: 1, Scope: cmd.Scope, Actor: cmd.ActorID, IncludeScenarios: cmd.IncludeScenarios}
	if cmd.Cursor == "" {
		return c, nil
	}
	if len(cmd.Cursor) > 1024 {
		return c, ErrCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(cmd.Cursor)
	if err != nil {
		return c, ErrCursor
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&c) != nil || decoder.Decode(&struct{}{}) != io.EOF || c.Version != 1 || c.Scope != cmd.Scope || c.Actor != cmd.ActorID || c.IncludeScenarios != cmd.IncludeScenarios || c.Before.IsZero() || c.ID == uuid.Nil {
		return forecastCursor{}, ErrCursor
	}
	return c, nil
}

func (s *jobs) List(ctx context.Context, cmd ListForecastCommand) (ListForecastResult, error) {
	if cmd.Scope.Validate() != nil || strings.TrimSpace(cmd.ActorID) == "" || cmd.Limit < 0 || cmd.Limit > 100 {
		return ListForecastResult{}, ErrInvalid
	}
	if cmd.Limit == 0 {
		cmd.Limit = 50
	}
	cursor, err := decodeForecastCursor(cmd)
	if err != nil {
		return ListForecastResult{}, err
	}
	result := ListForecastResult{Items: []ForecastView{}}
	err = s.withinTx(ctx, func(ctx context.Context, storage jobStore, _ jobSnapshots, _ jobCommands) error {
		revision, err := storage.LockDataVersion(ctx, cmd.Scope)
		if err != nil {
			return err
		}
		rows, err := storage.ListJobs(ctx, cmd.Scope, cursor.Before, cursor.ID, cmd.Limit+1)
		if err != nil {
			return err
		}
		more := len(rows) > cmd.Limit
		if more {
			rows = rows[:cmd.Limit]
		}
		for _, row := range rows {
			input, err := decodeJobInput(row)
			if err != nil || row.Scope != cmd.Scope || revision < row.DataVersion || row.ID == uuid.Nil || row.CreatedAt.IsZero() {
				return ErrUnavailable
			}
			switch row.Status {
			case "queued", "running", "succeeded", "failed":
			default:
				return ErrUnavailable
			}
			result.Items = append(result.Items, ForecastView{ID: row.ID, Scope: row.Scope, Status: row.Status, CreatedAt: row.CreatedAt, CompletedAt: row.CompletedAt, CurrentDataVersion: revision, DataVersion: row.DataVersion, Cutoff: input.Cutoff})
		}
		if more {
			last := rows[len(rows)-1]
			cursor.Before, cursor.ID = last.CreatedAt, last.ID
			encoded, err := json.Marshal(cursor)
			if err != nil {
				return ErrUnavailable
			}
			result.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
		}
		return nil
	})
	if err != nil {
		return ListForecastResult{}, err
	}
	return result, nil
}
