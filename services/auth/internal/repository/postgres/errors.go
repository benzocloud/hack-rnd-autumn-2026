package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrUnavailable   = errors.New("auth database unavailable")
	ErrSchema        = errors.New("auth database schema does not match this binary; run migrations")
	ErrNotFound      = errors.New("auth user not found")
	ErrEmailExists   = errors.New("auth email already exists")
	ErrInvalidRecord = errors.New("invalid auth storage record")
)

// Do not wrap PgError.Detail or connection errors: they can contain email/DSN.
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
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
			return ErrEmailExists
		}
		switch pgErr.Code {
		case "23514", "23502", "23503", "22P02":
			return ErrInvalidRecord
		}
	}
	return ErrUnavailable
}
