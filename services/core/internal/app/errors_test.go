package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
	ledgerService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
	paymentsService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

func TestLedgerErrorKeepsPrivateCause(t *testing.T) {
	t.Parallel()
	cause := &pgconn.PgError{Code: "42P01", Message: "private financial payload"}
	err := ledgerError(errors.Join(storage.ErrUnavailable, cause))
	var got *pgconn.PgError
	if !errors.Is(err, ledgerService.ErrUnavailable) || !errors.As(err, &got) || got != cause {
		t.Fatal("application mapping lost classification or cause")
	}
	if strings.Contains(err.Error(), cause.Message) {
		t.Fatal("driver message exposed")
	}
}

func TestPaymentsErrorKeepsPrivateCause(t *testing.T) {
	t.Parallel()
	cause := &pgconn.PgError{Code: "42P01", Message: "private financial payload"}
	err := paymentsError(errors.Join(storage.ErrUnavailable, cause))
	var got *pgconn.PgError
	if !errors.Is(err, paymentsService.ErrUnavailable) || !errors.As(err, &got) || got != cause {
		t.Fatal("application mapping lost classification or cause")
	}
	if strings.Contains(err.Error(), cause.Message) {
		t.Fatal("driver message exposed")
	}
}

func TestFeatureMappingPreservesCallbackErrors(t *testing.T) {
	t.Parallel()
	for _, feature := range []struct {
		name     string
		mapError func(error) error
		expected []error
	}{
		{name: "ledger", mapError: ledgerError, expected: []error{ledgerService.ErrInvalid, ledgerService.ErrNotFound, ledgerService.ErrStaleVersion, ledgerService.ErrConflict, ledgerService.ErrIdempotencyConflict, ledgerService.ErrUnavailable}},
		{name: "payments", mapError: paymentsError, expected: []error{paymentsService.ErrInvalid, paymentsService.ErrNotFound, paymentsService.ErrStaleVersion, paymentsService.ErrConflict, paymentsService.ErrIdempotencyConflict, paymentsService.ErrUnavailable}},
	} {
		t.Run(feature.name, func(t *testing.T) {
			for _, want := range feature.expected {
				got := feature.mapError(want)
				if !errors.Is(got, want) || got.Error() != want.Error() {
					t.Fatalf("callback error reclassified: %v", got)
				}
			}
		})
	}
}
