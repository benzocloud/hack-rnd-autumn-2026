package ledger

import (
	"context"

	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
)

func LoadBalance(ctx context.Context, tx *sqlx.Tx, scope domain.Scope, date domain.Date) (domain.BalanceObservation, error) {
	var amount int64
	var currency string
	source := domain.BalanceSource{}
	err := tx.QueryRowxContext(ctx, queryLoadBalance, scope.CompanyID, scope.AccountID, string(date)).Scan(&amount, &currency,
		&source.Kind, &source.Namespace.System, &source.Namespace.CompanyID, &source.RecordedAt)
	if err != nil {
		return domain.BalanceObservation{}, err
	}
	value, err := money.New(amount, currency)
	if err != nil {
		return domain.BalanceObservation{}, err
	}
	return domain.NewBalanceObservation(scope, date, value, source)
}

func InsertBalance(ctx context.Context, tx *sqlx.Tx, balance domain.BalanceObservation, note string) error {
	scope, source := balance.Scope(), balance.Source()
	_, err := tx.ExecContext(ctx, queryInsertBalance, scope.CompanyID, scope.AccountID, string(balance.Date()), balance.Amount().Minor(),
		balance.Amount().Currency(), source.Kind, source.Namespace.System, source.Namespace.CompanyID, source.RecordedAt, note)
	return err
}
