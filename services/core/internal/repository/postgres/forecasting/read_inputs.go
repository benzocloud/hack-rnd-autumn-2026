package forecasting

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/forecasting"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	paymentDomain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
	ledgerSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/ledger"
	paymentsSQL "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres/payments"
)

func (r *repository) ReadCategories(ctx context.Context, scope ledger.Scope) ([]ledger.CategoryDefinition, error) {
	return ledgerSQL.New(r.tx).ListCategories(ctx, scope.CompanyID, uuid.Nil, 1001)
}

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
	calendar := domain.Calendar{Reason: domain.HistoricalCalendarUnavailable, Captured: true, KnowledgeMode: "unavailable"}
	payments := paymentsSQL.New(r.tx)
	for _, status := range []paymentDomain.Status{paymentDomain.Planned, paymentDomain.PartiallyMatched, paymentDomain.Matched, paymentDomain.Cancelled} {
		items, err := payments.ListPayments(ctx, scope, "", "", status, "", uuid.Nil, 1001)
		if err != nil {
			return domain.Calendar{}, err
		}
		calendar.Payments = append(calendar.Payments, items...)
		if len(calendar.Payments) > 1000 {
			return domain.Calendar{}, storage.ErrUnavailable
		}
	}
	calendar.ExcludedPayments = int64(len(calendar.Payments))
	groups, err := payments.ListReconciliationGroups(ctx, scope, false, uuid.Nil, 1001)
	if err != nil {
		return domain.Calendar{}, err
	}
	if len(groups) > 1000 {
		return domain.Calendar{}, storage.ErrUnavailable
	}
	calendar.Groups = groups
	var latest *string
	err = r.tx.QueryRowxContext(ctx, queryLatestFactDate, scope.CompanyID, scope.AccountID).Scan(&latest)
	if err != nil {
		return domain.Calendar{}, storage.StorageError(err)
	}
	if latest != nil {
		calendar.LatestFactDate = ledger.Date(*latest)
	}
	return calendar, nil
}

func (r *repository) ReadBalanceHistory(ctx context.Context, scope ledger.Scope, cutoff ledger.Date) (result []domain.OpeningBalance, err error) {
	rows, err := r.tx.QueryxContext(ctx, queryReadBalanceHistory, scope.CompanyID, scope.AccountID, string(cutoff))
	if err != nil {
		return nil, storage.StorageError(err)
	}
	defer func() { err = errors.Join(err, storage.StorageError(rows.Close())) }()
	for rows.Next() {
		var date string
		var amount, version int64
		var source ledger.BalanceSource
		var recorded time.Time
		if err := rows.Scan(&date, &amount, &source.Kind, &source.Namespace.System, &source.Namespace.CompanyID, &recorded, &version); err != nil {
			return nil, storage.StorageError(err)
		}
		source.RecordedAt = recorded
		value, err := money.New(amount, money.RUB)
		if err != nil {
			return nil, storage.WithCause(storage.ErrUnavailable, err)
		}
		observation, err := ledger.NewBalanceObservation(scope, ledger.Date(date), value, source)
		if err != nil {
			return nil, storage.WithCause(storage.ErrUnavailable, err)
		}
		result = append(result, domain.OpeningBalance{Observation: observation, DataVersion: version})
	}
	return result, storage.StorageError(rows.Err())
}
