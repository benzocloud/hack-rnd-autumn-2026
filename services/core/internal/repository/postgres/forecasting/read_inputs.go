package forecasting

import (
	"context"
	"errors"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
	ledgerSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/ledger"
)

func (r *repository) ReadDataVersion(ctx context.Context, scope ledger.Scope) (int64, error) {
	var revision int64
	err := r.tx.QueryRowxContext(ctx, queryReadRevision, scope.CompanyID, scope.AccountID).Scan(&revision)
	return revision, storage.StorageError(err)
}

func (r *repository) ReadOpeningBalance(ctx context.Context, scope ledger.Scope, cutoff ledger.Date) (domain.OpeningBalance, bool, error) {
	balance, revision, err := ledgerSQL.LoadBalance(ctx, r.tx, scope, cutoff)
	if errors.Is(err, storage.ErrNotFound) {
		return domain.OpeningBalance{}, false, nil
	}
	return domain.OpeningBalance{Observation: balance, DataVersion: revision}, err == nil, err
}

// These scoped projections deliberately do not use the write-side FOR UPDATE
// loaders: all reads must retain the app-owned REPEATABLE READ MVCC view.
func (r *repository) ReadFacts(ctx context.Context, scope ledger.Scope, cutoff ledger.Date) (facts []domain.Fact, err error) {
	rows, err := r.tx.QueryxContext(ctx, queryReadFacts, scope.CompanyID, scope.AccountID, string(cutoff))
	if err != nil {
		return nil, storage.StorageError(err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			facts = nil
			err = errors.Join(err, storage.StorageError(closeErr))
		}
	}()
	facts = make([]domain.Fact, 0)
	for rows.Next() {
		fact, err := scanFact(rows, scope)
		if err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	return facts, storage.StorageError(rows.Err())
}

func (r *repository) ReadCalendar(ctx context.Context, scope ledger.Scope) (domain.Calendar, error) {
	calendar := domain.Calendar{Reason: domain.HistoricalCalendarUnavailable}
	err := r.tx.QueryRowxContext(ctx, queryReadCalendar, scope.CompanyID, scope.AccountID).Scan(&calendar.ExcludedPayments)
	return calendar, storage.StorageError(err)
}
