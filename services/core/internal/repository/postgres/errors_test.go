package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func TestStorageErrorBoundary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		err, want error
	}{
		{"not found", sql.ErrNoRows, ErrNotFound},
		{"foreign key", &pgconn.PgError{Code: "23503", Detail: "private financial payload"}, ErrConflict},
		{"database failure", &pgconn.PgError{Code: "42P01", Message: "private financial payload"}, ErrUnavailable},
		{"scan failure", errors.New("private financial payload"), ErrUnavailable},
		{"domain", ledger.ErrSourceConflict, ledger.ErrSourceConflict},
		{"deadline", context.DeadlineExceeded, context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapError(tc.err); !errors.Is(got, tc.want) {
				t.Fatalf("error=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestStorageErrorKeepsPrivateCause(t *testing.T) {
	t.Parallel()
	cause := &pgconn.PgError{Code: "42P01", Message: "private financial payload"}
	err := mapError(cause)
	var got *pgconn.PgError
	if !errors.As(err, &got) || got != cause || !errors.Is(err, ErrUnavailable) {
		t.Fatal("driver cause/classification lost")
	}
	if strings.Contains(err.Error(), cause.Message) {
		t.Fatal("driver message exposed")
	}
	scan := errors.New("private scan value")
	if !errors.Is(mapError(scan), scan) {
		t.Fatal("scan cause lost")
	}
}
