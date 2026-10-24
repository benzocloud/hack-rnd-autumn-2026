package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/money"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/payments"
)

var (
	ErrNotFound            = errors.New("financial resource not found")
	ErrStaleVersion        = errors.New("stale financial data version")
	ErrConflict            = errors.New("financial storage conflict")
	ErrIdempotencyConflict = errors.New("idempotency key body conflict")
	ErrUnavailable         = errors.New("financial storage unavailable")
)

// SQL errors are translated at the storage boundary; driver messages may contain data.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return WithCause(context.Canceled, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return WithCause(context.DeadlineExceeded, err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return WithCause(ErrNotFound, err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23503", "23514":
			return WithCause(ErrConflict, err)
		default:
			return WithCause(ErrUnavailable, err)
		}
	}
	for _, expected := range []error{
		ErrNotFound, ErrStaleVersion, ErrConflict, ErrIdempotencyConflict, ErrUnavailable,
		ledger.ErrInvalid, ledger.ErrScope, ledger.ErrSourceConflict, ledger.ErrBinding, ledger.ErrExternalRecord, ledger.ErrBalanceConflict, ledger.ErrVersion,
		payments.ErrInvalid, payments.ErrState, payments.ErrAllocation, payments.ErrDirection,
		money.ErrCurrency, money.ErrAmount, money.ErrOverflow,
	} {
		if errors.Is(err, expected) {
			return WithCause(expected, err)
		}
	}
	return WithCause(ErrUnavailable, err)
}

// StorageError classifies SQL/domain failures without exposing driver messages.
// Call only at SQL/lifecycle boundaries; transaction callbacks preserve their errors.
func StorageError(err error) error { return mapError(err) }
