package ledger

import (
	"context"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func (r *repository) HasImportedEquivalent(ctx context.Context, d domain.TransactionData) (bool, error) {
	var exists bool
	err := r.tx.QueryRowxContext(ctx, queryImportedEquivalent, d.Scope.CompanyID, d.Scope.AccountID, string(d.Date), string(d.Direction), d.Amount.Minor(), d.CounterpartyINN, d.CounterpartyName, d.Purpose).Scan(&exists)
	return exists, storage.StorageError(err)
}
