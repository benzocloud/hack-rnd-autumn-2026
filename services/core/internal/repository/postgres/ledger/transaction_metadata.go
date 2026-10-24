package ledger

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func (r *repository) TransactionCreatedAt(ctx context.Context, scope domain.Scope, id uuid.UUID) (time.Time, error) {
	var at time.Time
	if err := r.tx.GetContext(ctx, &at, queryTransactionCreatedAt, scope.CompanyID, scope.AccountID, id); err != nil {
		return time.Time{}, storage.StorageError(err)
	}
	return at.UTC(), nil
}

func (r *repository) CategoryExists(ctx context.Context, companyID, categoryID uuid.UUID) (bool, error) {
	var exists bool
	err := r.tx.GetContext(ctx, &exists, queryCategoryExists, companyID, categoryID)
	return exists, storage.StorageError(err)
}
