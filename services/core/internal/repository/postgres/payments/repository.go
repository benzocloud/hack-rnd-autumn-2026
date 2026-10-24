package payments

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

type repository struct{ tx *sqlx.Tx }

// New borrows tx; its caller owns transaction completion.
func New(tx *sqlx.Tx) *repository { return &repository{tx: tx} }

func (r *repository) CarryHistoryCoverage(ctx context.Context, scope ledger.Scope, previous, next int64) error {
	if scope.Validate() != nil || previous < 1 || next != previous+1 {
		return storage.ErrStaleVersion
	}
	_, err := r.tx.ExecContext(ctx, queryCarryHistoryCoverage, scope.CompanyID, scope.AccountID, previous, next)
	return storage.StorageError(err)
}
