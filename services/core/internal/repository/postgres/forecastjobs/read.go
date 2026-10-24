package forecastjobs

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func (r *repository) LockDataVersion(ctx context.Context, scope ledger.Scope) (int64, error) {
	var revision int64
	err := r.tx.GetContext(ctx, &revision, queryLockRevision, scope.CompanyID, scope.AccountID)
	return revision, storage.StorageError(err)
}

func (r *repository) ReadCoverage(ctx context.Context, scope ledger.Scope, revision int64, from, through ledger.Date) (domain.JobCoverage, bool, error) {
	var first, last, reference string
	err := r.tx.QueryRowxContext(ctx, queryReadCoverage, scope.CompanyID, scope.AccountID, revision, string(from), string(through)).Scan(&first, &last, &reference)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.JobCoverage{}, false, nil
	}
	return domain.JobCoverage{From: ledger.Date(first), Through: ledger.Date(last), Reference: reference}, err == nil, storage.StorageError(err)
}

func (r *repository) ReadJob(ctx context.Context, scope ledger.Scope, id uuid.UUID) (domain.Job, error) {
	var row jobRow
	err := r.tx.GetContext(ctx, &row, queryReadJob, scope.CompanyID, scope.AccountID, id)
	if err != nil {
		return domain.Job{}, storage.StorageError(err)
	}
	return row.job(), nil
}

// ListJobs returns summary metadata and frozen input, omitting Result. ReadJob
// is the full projection; a summary cannot be decoded as a complete succeeded job.
func (r *repository) ListJobs(ctx context.Context, scope ledger.Scope, before time.Time, beforeID uuid.UUID, limit int) ([]domain.Job, error) {
	var cutoff any
	if !before.IsZero() {
		cutoff = before
	}
	var rows []jobRow
	if err := r.tx.SelectContext(ctx, &rows, queryListJobs, scope.CompanyID, scope.AccountID, cutoff, beforeID, limit); err != nil {
		return nil, storage.StorageError(err)
	}
	result := make([]domain.Job, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.job())
	}
	return result, nil
}
