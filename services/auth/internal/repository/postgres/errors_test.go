package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestStorageErrorsMapSQLXDriverFailuresWithoutSecrets(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{name: "success"},
		{name: "missing row", err: sql.ErrNoRows, want: ErrNotFound},
		{name: "cancelled", err: fmt.Errorf("TOP_SECRET: %w", context.Canceled), want: context.Canceled},
		{name: "deadline", err: fmt.Errorf("TOP_SECRET: %w", context.DeadlineExceeded), want: context.DeadlineExceeded},
		{name: "email conflict", err: &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key", Detail: "TOP_SECRET"}, want: ErrEmailExists},
		{name: "other conflict", err: &pgconn.PgError{Code: "23505", ConstraintName: "credentials_pkey", Detail: "TOP_SECRET"}, want: ErrUnavailable},
		{name: "check violation", err: &pgconn.PgError{Code: "23514", Detail: "TOP_SECRET"}, want: ErrInvalidRecord},
		{name: "not null", err: &pgconn.PgError{Code: "23502", Detail: "TOP_SECRET"}, want: ErrInvalidRecord},
		{name: "foreign key", err: &pgconn.PgError{Code: "23503", Detail: "TOP_SECRET"}, want: ErrInvalidRecord},
		{name: "invalid UUID", err: &pgconn.PgError{Code: "22P02", Detail: "TOP_SECRET"}, want: ErrInvalidRecord},
		{name: "connection failure", err: errors.New("postgres://TOP_SECRET"), want: ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := storageError(tc.err)
			if !errors.Is(got, tc.want) {
				t.Fatalf("wrong storage error: %v", got)
			}
			if got != nil && strings.Contains(got.Error(), "TOP_SECRET") {
				t.Fatal("storage error exposed private driver details")
			}
		})
	}
}
