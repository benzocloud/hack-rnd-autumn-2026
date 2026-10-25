package payments

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func (r *repository) SavePayment(ctx context.Context, p domain.ExpectedPayment, expected int64) error {
	d := p.Data()
	tag, err := r.tx.ExecContext(ctx, queryUpdatePayment, d.Scope.CompanyID, d.Scope.AccountID, d.ID, d.Direction, d.Amount.Minor(), p.Remaining().Minor(), string(d.ScheduledDate), uuid.NullUUID{UUID: d.Category.ID, Valid: d.Category.ID != uuid.Nil}, d.CounterpartyName, d.Description, p.Status(), p.Version(), expected, uuid.NullUUID{UUID: d.ReconciliationGroupID, Valid: d.ReconciliationGroupID != uuid.Nil})
	if err != nil {
		return storage.StorageError(err)
	}
	n, err := tag.RowsAffected()
	if err != nil {
		return storage.StorageError(err)
	}
	if n != 1 {
		return storage.ErrStaleVersion
	}
	return nil
}
func (r *repository) FindMatch(ctx context.Context, scope ledger.Scope, paymentID, id uuid.UUID) (domain.PaymentMatch, error) {
	var matchID, transactionID uuid.UUID
	var minor int64
	var currency string
	var at time.Time
	err := r.tx.QueryRowxContext(ctx, queryFindMatch, scope.CompanyID, scope.AccountID, paymentID, id).Scan(&matchID, &transactionID, &minor, &currency, &at)
	if err != nil {
		return domain.PaymentMatch{}, storage.StorageError(err)
	}
	amount, err := money.New(minor, currency)
	if err != nil {
		return domain.PaymentMatch{}, storage.WithCause(storage.ErrUnavailable, err)
	}
	return domain.RestorePaymentMatch(matchID, paymentID, transactionID, scope, amount, at)
}
func (r *repository) RemoveMatch(ctx context.Context, scope ledger.Scope, id uuid.UUID) error {
	tag, err := r.tx.ExecContext(ctx, queryRemoveMatch, scope.CompanyID, scope.AccountID, id)
	if err != nil {
		return storage.StorageError(err)
	}
	n, err := tag.RowsAffected()
	if err != nil {
		return storage.StorageError(err)
	}
	if n != 1 {
		return storage.ErrNotFound
	}
	return nil
}
