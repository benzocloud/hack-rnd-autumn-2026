package app

import (
	"errors"

	storage "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/repository/postgres"
	forecastService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
	ledgerService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/ledger"
	paymentsService "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/payments"
)

func ledgerError(err error) error {
	for _, pair := range []struct{ storage, application error }{
		{storage.ErrNotFound, ledgerService.ErrNotFound}, {storage.ErrStaleVersion, ledgerService.ErrStaleVersion},
		{storage.ErrConflict, ledgerService.ErrConflict}, {storage.ErrIdempotencyConflict, ledgerService.ErrIdempotencyConflict},
		{storage.ErrUnavailable, ledgerService.ErrUnavailable},
	} {
		if errors.Is(err, pair.storage) {
			return storage.WithCause(pair.application, err)
		}
	}
	return err
}

func forecastingError(err error) error {
	if errors.Is(err, storage.ErrNotFound) {
		return storage.WithCause(forecastService.ErrNotFound, err)
	}
	if errors.Is(err, storage.ErrUnavailable) || errors.Is(err, storage.ErrConflict) {
		return storage.WithCause(forecastService.ErrUnavailable, err)
	}
	return err
}

func paymentsError(err error) error {
	for _, pair := range []struct{ storage, application error }{
		{storage.ErrNotFound, paymentsService.ErrNotFound}, {storage.ErrStaleVersion, paymentsService.ErrStaleVersion},
		{storage.ErrConflict, paymentsService.ErrConflict}, {storage.ErrIdempotencyConflict, paymentsService.ErrIdempotencyConflict},
		{storage.ErrUnavailable, paymentsService.ErrUnavailable},
	} {
		if errors.Is(err, pair.storage) {
			return storage.WithCause(pair.application, err)
		}
	}
	return err
}
