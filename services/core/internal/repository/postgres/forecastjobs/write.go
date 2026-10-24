package forecastjobs

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func (r *repository) InsertJob(ctx context.Context, job domain.Job) error {
	_, err := r.tx.ExecContext(ctx, queryInsertJob, job.Scope.CompanyID, job.Scope.AccountID, job.ID, job.DataVersion, job.CreatedAt, job.Input)
	return storage.StorageError(err)
}

func (r *repository) ClaimJob(ctx context.Context, token uuid.UUID) (domain.Job, bool, error) {
	if token == uuid.Nil {
		return domain.Job{}, false, storage.ErrUnavailable
	}
	var row jobRow
	err := r.tx.GetContext(ctx, &row, queryClaimJob, token)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, false, nil
	}
	if err != nil {
		return domain.Job{}, false, storage.StorageError(err)
	}
	return row.job(), true, nil
}

func (r *repository) FinishJob(ctx context.Context, job domain.Job, token uuid.UUID) error {
	if token == uuid.Nil || (job.Status != "succeeded" && job.Status != "failed") {
		return storage.ErrUnavailable
	}
	var result any
	if len(job.Result) > 0 {
		result = job.Result
	}
	tag, err := r.tx.ExecContext(ctx, queryFinishJob, job.Scope.CompanyID, job.Scope.AccountID, job.ID, token, job.Status, result, job.ErrorCode, job.RequestID)
	if err != nil {
		return storage.StorageError(err)
	}
	affected, err := tag.RowsAffected()
	if err != nil {
		return storage.StorageError(err)
	}
	if affected != 1 {
		return storage.ErrConflict
	}
	return nil
}
