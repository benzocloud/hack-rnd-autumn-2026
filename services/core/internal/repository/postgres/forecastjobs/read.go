package forecastjobs

import (
	"context"
	"database/sql"
	"encoding/json"
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
	var kind sql.NullString
	var first, last, reference string
	var version int64
	var limitations []byte
	err := r.tx.QueryRowxContext(ctx, queryReadCoverage, scope.CompanyID, scope.AccountID, revision, string(from), string(through)).Scan(&kind, &first, &last, &reference, &version, &limitations)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.JobCoverage{}, false, nil
	}
	if err != nil {
		return domain.JobCoverage{}, false, storage.StorageError(err)
	}
	if !kind.Valid {
		return domain.JobCoverage{}, false, storage.ErrUnavailable
	}
	var decoded []string
	if json.Unmarshal(limitations, &decoded) != nil {
		return domain.JobCoverage{}, false, storage.ErrUnavailable
	}
	return domain.JobCoverage{Kind: kind.String, From: ledger.Date(first), Through: ledger.Date(last), Reference: reference, DataVersion: version, Limitations: decoded}, true, nil
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
