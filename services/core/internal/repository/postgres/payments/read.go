package payments

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
)

func optionalDate(d ledger.Date) any {
	if d == "" {
		return nil
	}
	return string(d)
}
func (r *repository) ListPayments(ctx context.Context, scope ledger.Scope, from, through ledger.Date, status domain.Status, after ledger.Date, id uuid.UUID, limit int) (items []domain.ExpectedPayment, err error) {
	rows, err := r.tx.QueryxContext(ctx, queryListPayments, scope.CompanyID, scope.AccountID, optionalDate(from), optionalDate(through), status, optionalDate(after), id, limit)
	if err != nil {
		return nil, storage.StorageError(err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			items = nil
			err = errors.Join(err, storage.StorageError(closeErr))
		}
	}()
	items = make([]domain.ExpectedPayment, 0)
	for rows.Next() {
		p, _, err := scanPayment(rows, scope)
		if err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, storage.StorageError(rows.Err())
}
func (r *repository) ListMatches(ctx context.Context, scope ledger.Scope, paymentID uuid.UUID, after time.Time, id uuid.UUID, limit int) (items []domain.PaymentMatch, err error) {
	var cursor any
	if !after.IsZero() {
		cursor = after
	}
	rows, err := r.tx.QueryxContext(ctx, queryListMatches, scope.CompanyID, scope.AccountID, paymentID, cursor, id, limit)
	if err != nil {
		return nil, storage.StorageError(err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			items = nil
			err = errors.Join(err, storage.StorageError(closeErr))
		}
	}()
	items = make([]domain.PaymentMatch, 0)
	for rows.Next() {
		var matchID, transactionID uuid.UUID
		var minor int64
		var currency string
		var at time.Time
		var removed *time.Time
		if err := rows.Scan(&matchID, &transactionID, &minor, &currency, &at, &removed); err != nil {
			return nil, storage.StorageError(err)
		}
		amount, err := money.New(minor, currency)
		if err != nil {
			return nil, storage.WithCause(storage.ErrUnavailable, err)
		}
		m, err := domain.RestorePaymentMatch(matchID, paymentID, transactionID, scope, amount, at)
		if err != nil {
			return nil, storage.WithCause(storage.ErrUnavailable, err)
		}
		if removed != nil {
			m, err = m.WithRemoval(*removed)
			if err != nil {
				return nil, storage.WithCause(storage.ErrUnavailable, err)
			}
		}
		items = append(items, m)
	}
	return items, storage.StorageError(rows.Err())
}
