package postgres

import (
	"context"
	"errors"
)

var (
	ErrUnavailable = errors.New("auth database unavailable")
	ErrSchema      = errors.New("auth database schema does not match this binary; run migrations")
)

// Do not wrap connection errors: they can contain credentials or DSNs.
func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return ErrUnavailable
}
