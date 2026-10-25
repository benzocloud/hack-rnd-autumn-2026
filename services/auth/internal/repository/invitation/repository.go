package invitation

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/domain/account"
)

type repository struct {
	db           sqlx.ExtContext
	queryTimeout time.Duration
}

func NewRepository(db sqlx.ExtContext, queryTimeout time.Duration) *repository {
	return &repository{db: db, queryTimeout: queryTimeout}
}

func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return domain.ErrConflict
	}
	return domain.ErrUnavailable
}
