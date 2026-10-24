package ledger

import (
	"context"
	"database/sql"
	"errors"

	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"

	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func LoadBalance(ctx context.Context, tx *sqlx.Tx, scope domain.Scope, date domain.Date) (domain.BalanceObservation, int64, error) {
	var amount, revision int64
	var currency string
	source := domain.BalanceSource{}
	err := tx.QueryRowxContext(ctx, queryLoadBalance, scope.CompanyID, scope.AccountID, string(date)).Scan(&amount, &currency,
		&source.Kind, &source.Namespace.System, &source.Namespace.CompanyID, &source.RecordedAt, &revision)
	if err != nil {
		return domain.BalanceObservation{}, 0, storage.StorageError(err)
	}
	value, err := money.New(amount, currency)
	if err != nil {
		return domain.BalanceObservation{}, 0, storage.StorageError(err)
	}
	record, err := domain.NewBalanceObservation(scope, date, value, source)
	return record, revision, storage.StorageError(err)
}

func (r *repository) InsertBalance(ctx context.Context, balance domain.BalanceObservation, note string, revision int64) error {
	scope, source := balance.Scope(), balance.Source()
	_, err := r.tx.ExecContext(ctx, queryInsertBalance, scope.CompanyID, scope.AccountID, string(balance.Date()), balance.Amount().Minor(),
		balance.Amount().Currency(), source.Kind, source.Namespace.System, source.Namespace.CompanyID, source.RecordedAt, note, revision)
	return storage.StorageError(err)
}

func (r *repository) FindBalance(ctx context.Context, scope domain.Scope, date domain.Date) (domain.BalanceObservation, int64, bool, error) {
	record, revision, err := LoadBalance(ctx, r.tx, scope, date)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.BalanceObservation{}, 0, false, nil
	}
	return record, revision, err == nil, storage.StorageError(err)
}

func (r *repository) FindLatestBalance(ctx context.Context, scope domain.Scope, date domain.Date) (domain.BalanceObservation, int64, bool, error) {
	var row balanceRow
	err := r.tx.GetContext(ctx, &row, queryLatestBalance, scope.CompanyID, scope.AccountID, string(date))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.BalanceObservation{}, 0, false, nil
	}
	if err != nil {
		return domain.BalanceObservation{}, 0, false, storage.StorageError(err)
	}
	observation, err := row.observation(scope)
	return observation, row.Revision, err == nil, storage.StorageError(err)
}
