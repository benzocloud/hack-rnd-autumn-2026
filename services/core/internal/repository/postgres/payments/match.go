package payments

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func (r *repository) FactAllocated(ctx context.Context, scope ledger.Scope, id uuid.UUID) (money.Money, error) {
	var total string
	if err := r.tx.GetContext(ctx, &total, queryAllocated, scope.CompanyID, scope.AccountID, id); err != nil {
		return money.Money{}, storage.StorageError(err)
	}
	// PostgreSQL SUM(bigint) is numeric; reject overflow rather than narrowing it.
	minor, err := strconv.ParseInt(total, 10, 64)
	if err != nil {
		return money.Money{}, domain.ErrAllocation
	}
	return money.New(minor, money.RUB)
}

func (r *repository) SaveMatch(ctx context.Context, payment domain.ExpectedPayment, allocation domain.PaymentMatch, expected int64) error {
	scope := allocation.Scope()
	tag, err := r.tx.ExecContext(ctx, querySavePayment, scope.CompanyID, scope.AccountID, payment.Data().ID,
		payment.Remaining().Minor(), payment.Status(), payment.Version(), expected)
	if err != nil {
		return storage.StorageError(err)
	}
	count, err := tag.RowsAffected()
	if err != nil {
		return storage.StorageError(err)
	}
	if count != 1 {
		return storage.ErrStaleVersion
	}
	_, err = r.tx.ExecContext(ctx, queryInsertMatch, scope.CompanyID, scope.AccountID, allocation.ID(), allocation.PaymentID(),
		allocation.TransactionID(), allocation.Amount().Minor(), allocation.Amount().Currency(), allocation.CreatedAt())
	return storage.StorageError(err)
}
